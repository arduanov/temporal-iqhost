package admin

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

type Machine struct {
	ID               string `json:"id"`
	CPU              int32  `json:"cpu"`
	MemoryMiB        int32  `json:"memory_mib"`
	DiskGiB          int32  `json:"disk_gib"`
	GPU              bool   `json:"gpu"`
	State            string `json:"state"`
	RecoveryRequired bool   `json:"recovery_required"`
	OperationID      string `json:"operation_id"`
	OperationStatus  string `json:"operation_status"`
	RecoveryTarget   string `json:"recovery_target"`
	Node             string `json:"node"`
	IP               string `json:"ip"`
	GPUID            int64  `json:"gpu_id"`
	DiskID           int64  `json:"disk_id"`
}

func machineDTO(row dbgen.AdminVMsRow) Machine {
	return Machine{
		ID:        row.ID,
		CPU:       row.Cpu,
		MemoryMiB: row.MemoryMib,
		DiskGiB:   row.DiskGib,
		GPU:       row.Gpu,

		State:            row.State,
		RecoveryRequired: row.RecoveryRequired,
		OperationID:      row.OperationID,

		OperationStatus: row.OperationStatus,
		RecoveryTarget:  row.RecoveryTarget,
		Node:            row.Node,
		IP:              row.Ip,
		GPUID:           row.GpuID,
		DiskID:          row.DiskID,
	}
}

type Operation struct {
	vm.Operation
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func operationDTO(row dbgen.VmOperation) Operation {
	source := ""
	if row.SourceOperationID.Valid {
		source = uuid.UUID(row.SourceOperationID.Bytes).String()
	}
	return Operation{Operation: vm.Operation{
		ID:     uuid.UUID(row.ID.Bytes).String(),
		VMID:   row.VmID,
		UserID: row.UserID,
		Kind:   vm.OperationKind(row.Kind),

		Status:     vm.OperationStatus(row.Status),
		Phase:      vm.Phase(row.Phase),
		Step:       row.Step,
		WorkflowID: row.WorkflowID,

		RunID:             row.RunID,
		Error:             row.Error,
		RecoveryTarget:    vm.VMState(row.RecoveryTarget),
		SourceOperationID: source,
	}, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}

type Delivery struct {
	Attempts      int32      `json:"attempts"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	DeliveredAt   *time.Time `json:"delivered_at"`
}
type Step struct {
	Step        string    `json:"step"`
	CompletedAt time.Time `json:"completed_at"`
}
type OperationDetail struct {
	Operation
	Delivery Delivery `json:"delivery"`
	Steps    []Step   `json:"steps"`
}

func operationDetailDTO(snapshot postgres.OperationSnapshot) OperationDetail {
	result := OperationDetail{Operation: operationDTO(snapshot.Operation), Steps: []Step{}, Delivery: Delivery{
		Attempts: snapshot.Delivery.Attempts, NextAttemptAt: snapshot.Delivery.NextAttemptAt.Time,
	}}
	if snapshot.Delivery.DeliveredAt.Valid {
		result.Delivery.DeliveredAt = &snapshot.Delivery.DeliveredAt.Time
	}
	for _, row := range snapshot.Steps {
		result.Steps = append(result.Steps, Step{Step: row.Step, CompletedAt: row.CompletedAt.Time})
	}
	return result
}

type Node struct {
	ID                int64    `json:"id"`
	Name              string   `json:"name"`
	Available         bool     `json:"available"`
	CPU               int32    `json:"cpu"`
	MemoryMiB         int32    `json:"memory_mib"`
	ReservedCPU       int64    `json:"reserved_cpu"`
	ReservedMemoryMiB int64    `json:"reserved_memory_mib"`
	FreeCPU           int64    `json:"free_cpu"`
	FreeMemoryMiB     int64    `json:"free_memory_mib"`
	VMIDs             []string `json:"vm_ids"`
}
type GPU struct {
	ID        int64  `json:"id"`
	Model     string `json:"model"`
	Node      string `json:"node"`
	Available bool   `json:"available"`
	VMID      string `json:"vm_id"`
}
type Disk struct {
	ID      int64  `json:"id"`
	VMID    string `json:"vm_id"`
	SizeGiB int32  `json:"size_gib"`
}
type Storage struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	CapacityGiB int32  `json:"capacity_gib"`
	ReservedGiB int64  `json:"reserved_gib"`
	FreeGiB     int64  `json:"free_gib"`
	Disks       []Disk `json:"disks"`
}
type Address struct {
	Address string `json:"address"`
	VMID    string `json:"vm_id"`
}
type IPPool struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Subnet    string    `json:"subnet"`
	Total     int64     `json:"total"`
	Reserved  int64     `json:"reserved"`
	Free      int64     `json:"free"`
	Addresses []Address `json:"addresses"`
}
type Resources struct {
	Nodes   []Node    `json:"nodes"`
	GPUs    []GPU     `json:"gpus"`
	Storage []Storage `json:"storage"`
	IPPools []IPPool  `json:"ip_pools"`
}

func resourcesDTO(snapshot postgres.ResourceSnapshot) (Resources, error) {
	result := Resources{
		Nodes:   []Node{},
		GPUs:    []GPU{},
		Storage: []Storage{},
		IPPools: []IPPool{},
	}
	for _, row := range snapshot.Nodes {
		node, err := nodeDTO(row)
		if err != nil {
			return result, err
		}
		result.Nodes = append(result.Nodes, node)
	}
	for _, row := range snapshot.GPUs {
		result.GPUs = append(result.GPUs, GPU{
			ID:        row.ID,
			Model:     row.Model,
			Node:      row.Node,
			Available: row.Available && row.NodeAvailable && row.VmID == "",
			VMID:      row.VmID,
		})
	}
	for _, row := range snapshot.Storage {
		pool := Storage{
			ID:          row.ID,
			Name:        row.Name,
			Available:   row.Available,
			CapacityGiB: row.CapacityGib,
			ReservedGiB: row.ReservedGib,
			FreeGiB:     int64(row.CapacityGib) - row.ReservedGib,
		}
		if err := json.Unmarshal([]byte(row.Disks), &pool.Disks); err != nil {
			return result, err
		}
		result.Storage = append(result.Storage, pool)
	}
	for _, row := range snapshot.IPPools {
		pool := IPPool{
			ID:       row.ID,
			Name:     row.Name,
			Subnet:   row.Subnet,
			Total:    row.Total,
			Reserved: row.Reserved,
			Free:     row.Total - row.Reserved,
		}
		if err := json.Unmarshal([]byte(row.Addresses), &pool.Addresses); err != nil {
			return result, err
		}
		result.IPPools = append(result.IPPools, pool)
	}
	return result, nil
}

func nodeDTO(row dbgen.AdminNodesRow) (Node, error) {
	node := Node{
		ID:        row.ID,
		Name:      row.Name,
		Available: row.Available,
		CPU:       row.Cpu,
		MemoryMiB: row.MemoryMib,

		ReservedCPU:       row.ReservedCpu,
		ReservedMemoryMiB: row.ReservedMemoryMib,

		FreeCPU:       int64(row.Cpu) - row.ReservedCpu,
		FreeMemoryMiB: int64(row.MemoryMib) - row.ReservedMemoryMib,
	}
	if err := json.Unmarshal([]byte(row.VmIds), &node.VMIDs); err != nil {
		return node, err
	}
	return node, nil
}
