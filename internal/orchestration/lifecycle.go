package orchestration

import (
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

// Lifecycle задаёт порядок шагов. БД и провайдер доступны только из Activities:
// при replay Temporal подставляет их записанные результаты без повторного эффекта.
func Lifecycle(ctx workflow.Context, operationID string) error {
	ctx = workflow.WithActivityOptions(ctx, forwardOptions())
	var start vm.StepResult
	if err := executeStep(ctx, vm.StepRequest{OperationID: operationID, Step: vm.StepBegin}, &start); err != nil {
		// Если начало операции не подтверждено, её состояние проверит reconciler.
		return err
	}
	workflow.GetLogger(ctx).Info("operation started", "component", "workflow", "event", "operation_started", "operation_id", operationID)

	operationErr := executeSteps(ctx, operationID, operationSteps(start))
	// Отмена пользовательской операции не должна отменять её очистку и фиксацию результата.
	cleanupCtx, _ := workflow.NewDisconnectedContext(ctx)
	cleanupCtx = workflow.WithActivityOptions(cleanupCtx, cleanupOptions())
	if operationErr == nil {
		return finishOperation(cleanupCtx, operationID, vm.Succeeded, "")
	}
	return rollbackOperation(cleanupCtx, operationID, operationErr)
}

func operationSteps(start vm.StepResult) []string {
	if start.Noop {
		return nil
	}
	switch start.Kind {
	case vm.CreateVM, vm.StartVM:
		return []string{vm.StepReserve, provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady}
	default:
		return cleanupSteps()
	}
}

// Очистка идемпотентна: весь список безопасно выполнить даже после частичного сбоя.
func cleanupSteps() []string {
	return []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease}
}

func executeStep(ctx workflow.Context, request vm.StepRequest, result any) error {
	return workflow.ExecuteActivity(ctx, stepActivityName(request.Step), request).Get(ctx, result)
}

func executeSteps(ctx workflow.Context, operationID string, steps []string) error {
	for _, step := range steps {
		if err := executeStep(ctx, vm.StepRequest{OperationID: operationID, Step: step}, nil); err != nil {
			return err
		}
	}
	return nil
}

func finishOperation(ctx workflow.Context, operationID string, status vm.OperationStatus, message string) error {
	return executeStep(ctx, vm.StepRequest{
		OperationID: operationID,
		Step:        vm.StepFinish,
		Status:      status,
		Error:       message,
	}, nil)
}

func rollbackOperation(ctx workflow.Context, operationID string, cause error) error {
	outcome := vm.OperationFailed
	if temporal.IsCanceledError(cause) {
		outcome = vm.OperationCanceled
	}
	cleanupErr := executeStep(ctx, vm.StepRequest{OperationID: operationID, Step: vm.StepBeginRollback}, nil)
	if cleanupErr == nil {
		cleanupErr = executeSteps(ctx, operationID, cleanupSteps())
	}
	if cleanupErr != nil {
		if err := finishOperation(ctx, operationID, vm.Blocked, cleanupErr.Error()); err != nil {
			return err
		}
		return temporal.NewNonRetryableApplicationError("cleanup requires recovery", "RollbackBlocked", cleanupErr)
	}
	if err := finishOperation(ctx, operationID, outcome, cause.Error()); err != nil {
		return err
	}
	return cause
}
