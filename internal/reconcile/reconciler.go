package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"time"

	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"temporal-iqhost/internal/vm"
)

type Repository interface {
	Candidates(context.Context) ([]vm.Operation, error)
	Fence(context.Context, string) error
}
type Reconciler struct {
	Store  Repository
	Client client.Client
}

func (r *Reconciler) Once(ctx context.Context) error {
	operations, err := r.Store.Candidates(ctx)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		temporalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		info, err := r.Client.DescribeWorkflowExecution(temporalCtx, operation.WorkflowID, operation.RunID)
		cancel()
		if err != nil {
			var missing *serviceerror.NotFound
			if !errors.As(err, &missing) {
				slog.Warn("Temporal describe unavailable", "component", "reconciler", "event", "describe_failed", "operation_id", operation.ID)
				continue
			}
		} else if info.GetWorkflowExecutionInfo().GetStatus() == enums.WORKFLOW_EXECUTION_STATUS_RUNNING {
			continue
		}
		if err = r.Store.Fence(ctx, operation.ID); err != nil {
			return err
		}
		slog.Error("execution discrepancy fenced", "component", "reconciler", "event", "recovery_required", "operation_id", operation.ID, "vm_id", operation.VMID, "workflow_id", operation.WorkflowID, "run_id", operation.RunID)
	}
	return nil
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.Once(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("reconcile iteration failed", "component", "reconciler", "event", "poll_failed")
			}
		}
	}
}

const ReconcileInterval = 10 * time.Second
