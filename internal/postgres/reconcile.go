package postgres

import (
	"context"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

func (s *Store) Candidates(ctx context.Context) ([]vm.Operation, error) {
	rows, err := s.Q.ReconcileCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]vm.Operation, 0, len(rows))
	for _, operation := range rows {
		out = append(out, operationFromRow(operation))
	}
	return out, nil
}

func (s *Store) Fence(ctx context.Context, operationID string) error {
	return s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		hint, err := queries.GetOperation(ctx, parseUUID(operationID))
		if err != nil {
			return err
		}
		machine, err := queries.LockVM(ctx, hint.VmID)
		if err != nil {
			return err
		}
		operation, err := queries.LockOperation(ctx, hint.ID)
		if err != nil {
			return err
		}
		if !isActiveOperation(operation) || machine.CurrentOperationID != operation.ID {
			return nil
		}
		if operation.Phase != "rollback" {
			if err = queries.BumpGeneration(ctx, machine.ID); err != nil {
				return err
			}
		}
		if err = queries.Rollback(ctx, operation.ID); err != nil {
			return err
		}
		if err = queries.FinishOperation(ctx, dbgen.FinishOperationParams{
			ID:     operation.ID,
			Status: "rollback-blocked",
			Error:  "Temporal execution closed or history missing before database completion",
		}); err != nil {
			return err
		}
		state, err := confirmedVMState(ctx, queries, machine)
		if err != nil {
			return err
		}
		return queries.SetVMState(ctx, dbgen.SetVMStateParams{
			ID:               machine.ID,
			State:            state,
			RecoveryRequired: true,
		})
	})
}
