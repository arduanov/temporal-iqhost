package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

func confirmedVMState(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm) (string, error) {
	instance, err := queries.Instance(ctx, machine.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if instance.Exists && instance.PoweredOn {
		return "running", nil
	}
	if !instance.Exists && (machine.State == "not-created" || machine.State == "deleted") {
		return machine.State, nil
	}
	return "stopped", nil
}

func finishLocalOperation(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, operation dbgen.VmOperation, request vm.StepRequest) (string, error) {
	state, err := confirmedVMState(ctx, queries, machine)
	if err != nil {
		return state, err
	}
	blocked := request.Status == vm.Blocked
	if !blocked {
		if request.Status == vm.Succeeded && (operation.Kind == "create" || operation.Kind == "start") {
			state = "running"
		} else {
			state = operation.RecoveryTarget
		}
	}
	if !blocked {
		if err = validateOutcome(ctx, queries, machine.ID, state); err != nil {
			return state, err
		}
	}
	if err = queries.FinishOperation(ctx, dbgen.FinishOperationParams{
		ID:     operation.ID,
		Status: string(request.Status),
		Error:  request.Error,
	}); err != nil {
		return state, err
	}
	if err = queries.SetVMState(ctx, dbgen.SetVMStateParams{
		ID:               machine.ID,
		State:            state,
		RecoveryRequired: blocked,
	}); err != nil {
		return state, err
	}
	return state, nil
}

// Успешное завершение допустимо только при согласованном состоянии VM и резервов.
func validateOutcome(ctx context.Context, queries *dbgen.Queries, vmID, state string) error {
	resources, resourceErr := queries.ResourcesPresent(ctx, vmID)
	if resourceErr != nil {
		return resourceErr
	}
	instance, resourceErr := queries.Instance(ctx, vmID)
	if resourceErr != nil && !errors.Is(resourceErr, pgx.ErrNoRows) {
		return resourceErr
	}
	switch state {
	case "running":
		if !instance.Exists || !instance.PoweredOn || !instance.NetworkAttached || !resources.Compute || !resources.Disk || !resources.Ip {
			return invalid("running outcome not confirmed")
		}
	case "stopped":
		if instance.Exists || instance.PoweredOn || instance.NetworkAttached || resources.Compute || !resources.Disk || !resources.Ip {
			return invalid("stopped outcome not confirmed")
		}
	default:
		if instance.Exists || instance.PoweredOn || instance.NetworkAttached || resources.Compute || resources.Disk || resources.Ip {
			return invalid("resource removal not confirmed")
		}
	}
	return nil
}
