package outbox

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"temporal-iqhost/internal/orchestration"
	"temporal-iqhost/internal/visibility"
	"temporal-iqhost/internal/vm"
)

type Repository interface {
	Claim(context.Context) (*vm.Delivery, error)
	Operation(context.Context, string) (vm.Operation, error)
	Delivered(context.Context, vm.Delivery, string) error
	RetryDelivery(context.Context, vm.Delivery, time.Time) error
}
type Dispatcher struct {
	Store  Repository
	Client client.Client
}

func (d *Dispatcher) Once(ctx context.Context) error {
	delivery, err := d.Store.Claim(ctx)
	if err != nil || delivery == nil {
		return err
	}
	operation, err := d.Store.Operation(ctx, delivery.Operation.ID)
	if err != nil {
		return err
	}
	if operation.Terminal() {
		return nil
	}
	temporalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	run, err := d.Client.ExecuteWorkflow(temporalCtx, client.StartWorkflowOptions{
		ID:                                       operation.WorkflowID,
		TaskQueue:                                orchestration.TaskQueue,
		WorkflowIDReusePolicy:                    enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
		Memo: map[string]interface{}{
			"operation_id":   operation.ID,
			"operation_kind": string(operation.Kind),
			"vm_id":          operation.VMID,
			"user_id":        operation.UserID,
		},
		TypedSearchAttributes: temporal.NewSearchAttributes(
			visibility.OperationKind.ValueSet(string(operation.Kind)),
			visibility.VMID.ValueSet(operation.VMID),
			visibility.UserID.ValueSet(operation.UserID),
		),
	}, orchestration.Lifecycle, operation.ID)
	runID := ""
	if err == nil {
		runID = run.GetRunID()
	} else {
		var existingRunID string
		existingRunID, err = d.existingWorkflowRun(temporalCtx, operation, err)
		if err == nil {
			runID = existingRunID
		}
	}
	if err != nil {
		backoff := deliveryRetryDelay(delivery.Attempts)
		slog.Warn("delivery retry scheduled", "component", "dispatcher", "event", "delivery_retry", "operation_id", operation.ID, "attempt", delivery.Attempts+1)
		return d.Store.RetryDelivery(ctx, *delivery, time.Now().Add(backoff))
	}
	if err = d.Store.Delivered(ctx, *delivery, runID); err != nil {
		return err
	}
	slog.Info("command delivered", "component", "dispatcher", "event", "delivered", "user_id", operation.UserID, "operation_kind", operation.Kind, "operation_id", operation.ID, "vm_id", operation.VMID, "workflow_id", operation.WorkflowID, "run_id", runID)
	return nil
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := d.Once(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("dispatcher iteration failed", "component", "dispatcher", "event", "poll_failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// AlreadyStarted означает успешную доставку только для той же операции и типа workflow.
func (d *Dispatcher) existingWorkflowRun(ctx context.Context, operation vm.Operation, startErr error) (string, error) {
	var exists *serviceerror.WorkflowExecutionAlreadyStarted
	if !errors.As(startErr, &exists) {
		return "", startErr
	}
	info, err := d.Client.DescribeWorkflowExecution(ctx, operation.WorkflowID, exists.RunId)
	if err != nil {
		return "", startErr
	}
	workflowInfo := info.GetWorkflowExecutionInfo()
	payload := workflowInfo.GetMemo().GetFields()["operation_id"]
	var existingID string
	if workflowInfo.GetType().GetName() != "Lifecycle" || payload == nil || clientData(payload, &existingID) != nil || existingID != operation.ID {
		return "", startErr
	}
	return workflowInfo.GetExecution().GetRunId(), nil
}

// Случайная добавка распределяет повторы разных workers во времени.
func deliveryRetryDelay(attempts int32) time.Duration {
	shift := min(attempts, 6)
	backoff := min(time.Second*time.Duration(1<<shift), time.Minute)
	backoff = backoff/2 + time.Duration(rand.Int64N(int64(backoff/2)+1))
	return backoff
}
