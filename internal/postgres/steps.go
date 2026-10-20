package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

// LocalStep выполняет прикладной шаг и сохраняет его результат в одной транзакции.
func (s *Store) LocalStep(ctx context.Context, request vm.StepRequest) (vm.StepResult, error) {
	started := time.Now()
	logger := logging.FromContext(ctx).With("component", "activity", "operation_id", request.OperationID, "step", request.Step)
	logger.Info("step started", "event", "step_started")
	var execution localStepExecution
	err := s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		return execution.run(ctx, queries, request)
	})
	attrs := []any{"event", "step_completed", "duration_ms", time.Since(started).Milliseconds()}
	if execution.vmID != "" {
		attrs = append(attrs, "vm_id", execution.vmID)
	}
	if execution.state != "" && err == nil {
		attrs = append(attrs, "state", execution.state)
	}
	if err != nil {
		logger.Warn("step failed", append(attrs, "error", safeError(err))...)
	} else {
		logger.Info("step committed", attrs...)
		if request.Step == vm.StepFinish {
			if request.Status == vm.Succeeded {
				logger.Info("operation completed", "event", "operation_completed", "vm_id", execution.vmID, "status", request.Status)
			} else {
				logger.Error("operation requires attention", "event", "operation_completed", "vm_id", execution.vmID, "status", request.Status, "error", request.Error)
			}
		}
	}
	if err != nil {
		var app *vm.RejectedError
		if !errors.As(err, &app) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			err = errors.New(safeError(err))
		}
	}
	return execution.result, err
}

// Never log driver messages containing SQL values or connection strings.
func safeError(err error) string {
	var databaseError interface{ SQLState() string }
	if errors.As(err, &databaseError) {
		return "database error " + databaseError.SQLState()
	}
	var app interface{ Type() string }
	if errors.As(err, &app) {
		return app.Type()
	}
	return "operation failed"
}

func applyLocalStep(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, operation dbgen.VmOperation, request vm.StepRequest) (string, error) {
	var err error
	switch request.Step {
	case vm.StepBegin:
	case vm.StepBeginRollback:
		if operation.Phase != "rollback" {
			if err = queries.BumpGeneration(ctx, machine.ID); err != nil {
				return "", err
			}
		}
		if err = queries.Rollback(ctx, operation.ID); err != nil {
			return "", err
		}
	case vm.StepReserve:
		if err = allocateResources(ctx, queries, machine, operation); err != nil {
			return "", err
		}
	case vm.StepRelease:
		if err = releaseResources(ctx, queries, machine, operation, request); err != nil {
			return "", err
		}
	case vm.StepFinish:
		return finishLocalOperation(ctx, queries, machine, operation, request)
	default:
		return "", invalid("unknown step")
	}
	return "", nil
}

func saveLocalStep(ctx context.Context, queries *dbgen.Queries, operation dbgen.VmOperation, request vm.StepRequest, key string, result vm.StepResult) error {
	var err error
	if request.Step != vm.StepFinish {
		if err = queries.Progress(ctx, dbgen.ProgressParams{ID: operation.ID, Step: request.Step}); err != nil {
			return err
		}
	}
	if err = queries.SaveStep(ctx, dbgen.SaveStepParams{OperationID: operation.ID, Step: key}); err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return queries.SaveStepResult(ctx, dbgen.SaveStepResultParams{
		OperationID: operation.ID,
		Step:        key,
		Result:      raw,
	})
}

type localStepExecution struct {
	vmID, state string
	result      vm.StepResult
}

func (execution *localStepExecution) run(ctx context.Context, queries *dbgen.Queries, request vm.StepRequest) error {
	hint, err := queries.GetOperation(ctx, parseUUID(request.OperationID))
	if err != nil {
		return err
	}
	machine, err := queries.LockVM(ctx, hint.VmID)
	if err != nil {
		return err
	}
	execution.vmID = machine.ID
	operation, err := queries.LockOperation(ctx, hint.ID)
	if err != nil {
		return err
	}
	if machine.CurrentOperationID != operation.ID {
		return invalid("operation no longer owns VM")
	}
	execution.result = vm.StepResult{
		Kind:   vm.OperationKind(operation.Kind),
		Target: vm.VMState(operation.RecoveryTarget),
		Noop:   alreadyInRequestedState(machine, operation),
	}
	if !isActiveOperation(operation) {
		if request.Step == vm.StepFinish && operation.Status == string(request.Status) {
			return nil
		}
		return invalid("operation is terminal")
	}
	if operation.Phase == "rollback" && isForwardLocalStep(request.Step) {
		return invalid("forward step fenced by rollback")
	}
	key := operation.Phase + "/" + request.Step
	done, err := queries.StepDone(ctx, dbgen.StepDoneParams{OperationID: operation.ID, Step: key})
	if err != nil {
		return err
	}
	// Владение проверено выше: сохранённый результат не даёт старой операции новых прав.
	if done {
		raw, resultErr := queries.StepResult(ctx, dbgen.StepResultParams{OperationID: operation.ID, Step: key})
		if resultErr != nil {
			return resultErr
		}
		return json.Unmarshal(raw, &execution.result)
	}
	execution.state, err = applyLocalStep(ctx, queries, machine, operation, request)
	if err != nil {
		return err
	}
	return saveLocalStep(ctx, queries, operation, request, key, execution.result)
}

func alreadyInRequestedState(machine dbgen.Vm, operation dbgen.VmOperation) bool {
	switch operation.Kind {
	case "start":
		return machine.State == "running"
	case "stop":
		return machine.State == "stopped"
	case "delete":
		return machine.State == "deleted"
	default:
		return false
	}
}

func isForwardLocalStep(step string) bool {
	return step == vm.StepBegin || step == vm.StepReserve || isForwardCommand(step)
}
