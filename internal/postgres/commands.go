package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

func lockOperationOwner(ctx context.Context, queries *dbgen.Queries, operationID string) (dbgen.Vm, dbgen.VmOperation, error) {
	hint, err := queries.GetOperation(ctx, parseUUID(operationID))
	if err != nil {
		return dbgen.Vm{}, hint, err
	}
	machine, err := queries.LockVM(ctx, hint.VmID)
	if err != nil {
		return machine, hint, err
	}
	operation, err := queries.LockOperation(ctx, hint.ID)
	if err != nil {
		return machine, operation, err
	}
	if machine.CurrentOperationID != operation.ID || !isActiveOperation(operation) {
		return machine, operation, invalid("operation no longer owns VM")
	}
	return machine, operation, nil
}

func isForwardCommand(step string) bool {
	return step == provider.StepCreate || step == provider.StepAttachNetwork || step == provider.StepStart || step == provider.StepWaitReady
}

// Prepare holds only application locks. Once committed, the request never changes,
// including on a worker restart after a provider effect but before confirmation.
func (s *Store) Prepare(ctx context.Context, request vm.StepRequest) (provider.Request, error) {
	var providerRequest provider.Request
	transactionErr := s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		machine, operation, err := lockOperationOwner(ctx, queries, request.OperationID)
		if err != nil {
			return err
		}
		if operation.Phase == "rollback" && isForwardCommand(request.Step) {
			return invalid("forward command fenced")
		}
		key := request.OperationID + "/" + operation.Phase + "/" + request.Step
		old, err := queries.GetCommand(ctx, key)
		if err == nil {
			return json.Unmarshal(old.Request, &providerRequest)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		providerRequest = provider.Request{
			VMID:        machine.ID,
			OperationID: request.OperationID,
			Phase:       operation.Phase,
			Step:        request.Step,
			Generation:  machine.Generation,
			Key:         key,
		}
		if request.Step != provider.StepFence {
			if err = populateRequestResources(ctx, queries, machine, &providerRequest); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(providerRequest)
		if err != nil {
			return err
		}
		return queries.InsertCommand(ctx, dbgen.InsertCommandParams{
			CommandKey:  key,
			OperationID: operation.ID,
			VmID:        machine.ID,
			Generation:  machine.Generation,
			Request:     raw,
		})
	})
	return providerRequest, transactionErr
}

func (s *Store) Commit(ctx context.Context, providerRequest provider.Request, state provider.State) error {
	return s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		machine, operation, err := lockOperationOwner(ctx, queries, providerRequest.OperationID)
		if err != nil {
			return err
		}
		if machine.ID != providerRequest.VMID || machine.Generation != providerRequest.Generation || operation.Phase != providerRequest.Phase {
			return invalid("late provider result fenced")
		}
		command, err := queries.GetCommand(ctx, providerRequest.Key)
		if err != nil {
			return err
		}
		var savedRequest provider.Request
		if err = json.Unmarshal(command.Request, &savedRequest); err != nil {
			return err
		}
		if savedRequest != providerRequest {
			return invalid("command content conflict")
		}
		// A duplicate old result must never overwrite a later confirmed state.
		if command.Result != nil {
			return nil
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if err = queries.CompleteCommand(ctx, dbgen.CompleteCommandParams{CommandKey: providerRequest.Key, Result: raw}); err != nil {
			return err
		}
		if providerRequest.Step != provider.StepFence {
			if err = queries.PutInstance(ctx, dbgen.PutInstanceParams{
				VmID:             machine.ID,
				NodeID:           pgtype.Int8{Int64: state.NodeID, Valid: state.NodeID != 0},
				Exists:           state.Exists,
				NetworkAttached:  state.NetworkAttached,
				PoweredOn:        state.PoweredOn,
				OwnerOperationID: operation.ID,
				Generation:       machine.Generation,
			}); err != nil {
				return err
			}
			stateName, stateErr := confirmedVMState(ctx, queries, machine)
			if stateErr != nil {
				return stateErr
			}
			if stateErr = queries.SetVMState(ctx, dbgen.SetVMStateParams{
				ID:               machine.ID,
				State:            stateName,
				RecoveryRequired: machine.RecoveryRequired,
			}); stateErr != nil {
				return stateErr
			}
		}
		key := providerRequest.Phase + "/" + providerRequest.Step
		if err = queries.SaveStep(ctx, dbgen.SaveStepParams{OperationID: operation.ID, Step: key}); err != nil {
			return err
		}
		return queries.Progress(ctx, dbgen.ProgressParams{ID: operation.ID, Step: providerRequest.Step})
	})
}

// Отсутствующий резерв остаётся нулевым: cleanup допустим после частичного выполнения.
func populateRequestResources(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, providerRequest *provider.Request) error {
	providerRequest.Config = provider.Config{
		CPU:       machine.Cpu,
		MemoryMiB: machine.MemoryMib,
		DiskGiB:   machine.DiskGib,
		GPU:       machine.Gpu,
	}
	compute, resourceErr := queries.GetCompute(ctx, machine.ID)
	if resourceErr != nil && !errors.Is(resourceErr, pgx.ErrNoRows) {
		return resourceErr
	}
	providerRequest.Resources.NodeID = compute.NodeID
	providerRequest.Resources.GPUID = compute.GpuID.Int64
	disk, resourceErr := queries.GetDisk(ctx, machine.ID)
	if resourceErr != nil && !errors.Is(resourceErr, pgx.ErrNoRows) {
		return resourceErr
	}
	providerRequest.Resources.DiskID = disk.ID
	providerRequest.Resources.PoolID = disk.PoolID
	ip, resourceErr := queries.GetIP(ctx, pgtype.Text{String: machine.ID, Valid: true})
	if resourceErr != nil && !errors.Is(resourceErr, pgx.ErrNoRows) {
		return resourceErr
	}
	providerRequest.Resources.IP = ip
	return nil
}
