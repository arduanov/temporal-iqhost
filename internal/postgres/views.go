package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

func (s *Store) Operation(ctx context.Context, operationID string) (vm.Operation, error) {
	operation, err := s.Q.GetOperation(ctx, parseUUID(operationID))
	return operationFromRow(operation), err
}

func (s *Store) Get(ctx context.Context, vmID string) (json.RawMessage, error) {
	machine, err := s.Q.VMView(ctx, vmID)
	return json.RawMessage(machine), err
}

func (s *Store) List(ctx context.Context) ([]json.RawMessage, error) {
	ids, err := s.Q.ListVMs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(ids))
	for _, vmID := range ids {
		machine, err := s.Get(ctx, vmID)
		if err != nil {
			return nil, err
		}
		out = append(out, machine)
	}
	return out, nil
}

func operationFromRow(operation dbgen.VmOperation) vm.Operation {
	return vm.Operation{
		ID:                formatUUID(operation.ID),
		UserID:            operation.UserID,
		VMID:              operation.VmID,
		Kind:              vm.OperationKind(operation.Kind),
		Status:            vm.OperationStatus(operation.Status),
		Phase:             vm.Phase(operation.Phase),
		Step:              operation.Step,
		WorkflowID:        operation.WorkflowID,
		RunID:             operation.RunID,
		Error:             operation.Error,
		RecoveryTarget:    vm.VMState(operation.RecoveryTarget),
		SourceOperationID: formatUUID(operation.SourceOperationID),
	}
}

func isActiveOperation(operation dbgen.VmOperation) bool {
	return operation.Status == "pending" || operation.Status == "running"
}

func parseUUID(s string) pgtype.UUID {
	u, err := uuid.Parse(s)
	return pgtype.UUID{Bytes: u, Valid: err == nil}
}

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}
