package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

// Все операции берут блокировки в одном порядке: ноды → GPU → storage → IP.
// Проверяем свободную ёмкость после блокировки, чтобы избежать двойного выделения.
func lockInventory(ctx context.Context, queries *dbgen.Queries) ([]dbgen.ComputeNode, []dbgen.GpuDevice, []dbgen.StoragePool, []dbgen.IpAddress, error) {
	nodes, err := queries.LockNodes(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	gpus, err := queries.LockGPUs(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	pools, err := queries.LockStorage(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ips, err := queries.LockIPs(ctx)
	return nodes, gpus, pools, ips, err
}

func allocateResources(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, operation dbgen.VmOperation) error {
	nodes, gpus, pools, ips, err := lockInventory(ctx, queries)
	if err != nil {
		return err
	}
	selectedNodeID, gpu, err := selectCompute(ctx, queries, machine, nodes, gpus)
	if err != nil {
		return err
	}
	if err = queries.AllocateCompute(ctx, dbgen.AllocateComputeParams{
		VmID:        machine.ID,
		NodeID:      selectedNodeID,
		Cpu:         machine.Cpu,
		MemoryMib:   machine.MemoryMib,
		GpuID:       gpu,
		OperationID: operation.ID,
	}); err != nil {
		return err
	}
	if operation.Kind != "create" {
		return nil
	}
	return allocateDiskAndIP(ctx, queries, machine, pools, ips)
}

func allocateDiskAndIP(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, pools []dbgen.StoragePool, ips []dbgen.IpAddress) error {
	selectedPoolID, err := selectStoragePool(ctx, queries, machine.DiskGib, pools)
	if err != nil {
		return err
	}
	if err = queries.AllocateDisk(ctx, dbgen.AllocateDiskParams{
		VmID:    machine.ID,
		PoolID:  selectedPoolID,
		SizeGib: machine.DiskGib,
	}); err != nil {
		return err
	}
	return allocateIP(ctx, queries, machine.ID, ips)
}

func allocateIP(ctx context.Context, queries *dbgen.Queries, vmID string, ips []dbgen.IpAddress) error {
	for _, ip := range ips {
		if !ip.VmID.Valid {
			return queries.AssignIP(ctx, dbgen.AssignIPParams{ID: ip.ID, VmID: pgtype.Text{String: vmID, Valid: true}})
		}
	}
	return invalid("no IPv4 addresses available")
}

func releaseResources(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, operation dbgen.VmOperation, request vm.StepRequest) error {
	deletion, err := queries.GetCommand(ctx, request.OperationID+"/"+operation.Phase+"/delete-instance")
	if err != nil {
		return err
	}
	if deletion.Generation != machine.Generation || deletion.Result == nil {
		return invalid("deletion not confirmed in current generation")
	}
	instance, err := queries.Instance(ctx, machine.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if instance.Generation != machine.Generation || instance.Exists || instance.PoweredOn || instance.NetworkAttached {
		return invalid("instance absence not confirmed")
	}
	if _, _, _, _, err = lockInventory(ctx, queries); err != nil {
		return err
	}
	if err = queries.ReleaseCompute(ctx, machine.ID); err != nil {
		return err
	}
	if operation.RecoveryTarget != "stopped" {
		if err = queries.ReleaseDisk(ctx, machine.ID); err != nil {
			return err
		}
		if err = queries.ReleaseIP(ctx, pgtype.Text{String: machine.ID, Valid: true}); err != nil {
			return err
		}
	}
	return nil
}

// Выбираем первую подходящую ноду в порядке, заданном SQL-запросом.
func selectCompute(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, nodes []dbgen.ComputeNode, gpus []dbgen.GpuDevice) (int64, pgtype.Int8, error) {
	selectedNodeID := int64(0)
	gpu := pgtype.Int8{}
	for _, node := range nodes {
		if !node.Available {
			continue
		}
		used, usageErr := queries.ComputeUsage(ctx, node.ID)
		if usageErr != nil {
			return 0, pgtype.Int8{}, usageErr
		}
		if int64(node.Cpu)-used.Cpu < int64(machine.Cpu) || int64(node.MemoryMib)-used.MemoryMib < int64(machine.MemoryMib) {
			continue
		}
		if machine.Gpu {
			var gpuErr error
			gpu, gpuErr = selectGPU(ctx, queries, node.ID, gpus)
			if gpuErr != nil {
				return 0, pgtype.Int8{}, gpuErr
			}
			if !gpu.Valid {
				continue
			}
		}
		selectedNodeID = node.ID
		break
	}
	if selectedNodeID == 0 {
		return 0, pgtype.Int8{}, invalid("insufficient compute capacity")
	}
	return selectedNodeID, gpu, nil
}

func selectGPU(ctx context.Context, queries *dbgen.Queries, nodeID int64, gpus []dbgen.GpuDevice) (pgtype.Int8, error) {
	for _, device := range gpus {
		if device.NodeID != nodeID || !device.Available {
			continue
		}
		gpuID := pgtype.Int8{Int64: device.ID, Valid: true}
		busy, err := queries.GPUUsed(ctx, gpuID)
		if err != nil {
			return pgtype.Int8{}, err
		}
		if !busy {
			return gpuID, nil
		}
	}
	return pgtype.Int8{}, nil
}

func selectStoragePool(ctx context.Context, queries *dbgen.Queries, diskGiB int32, pools []dbgen.StoragePool) (int64, error) {
	selectedPoolID := int64(0)
	for _, pool := range pools {
		if !pool.Available {
			continue
		}
		used, usageErr := queries.DiskUsage(ctx, pool.ID)
		if usageErr != nil {
			return 0, usageErr
		}
		if int64(pool.CapacityGib)-used >= int64(diskGiB) {
			selectedPoolID = pool.ID
			break
		}
	}
	if selectedPoolID == 0 {
		return 0, invalid("insufficient storage capacity")
	}
	return selectedPoolID, nil
}
