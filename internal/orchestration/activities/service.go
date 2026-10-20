// Package activities связывает выполнение шагов с контекстом и ошибками Temporal.
package activities

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"temporal-iqhost/internal/execution"
	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

type Service struct {
	Execution *execution.Service
	FailAt    string
}

func (s *Service) Step(ctx context.Context, request vm.StepRequest) (result vm.StepResult, err error) {
	started := time.Now()
	logger := slog.Default().With("component", "activity", "operation_id", request.OperationID, "step", request.Step)
	if activity.IsActivity(ctx) {
		info := activity.GetInfo(ctx)
		logger = logger.With("workflow_id", info.WorkflowExecution.ID, "run_id", info.WorkflowExecution.RunID, "attempt", info.Attempt)
	}
	ctx = logging.WithLogger(ctx, logger)
	defer func() {
		attrs := []any{"event", "activity_completed", "duration_ms", time.Since(started).Milliseconds()}
		if err != nil {
			logger.Warn("activity failed", attrs...)
		} else {
			logger.Info("activity completed", attrs...)
		}
	}()
	defer func() {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		var rejected *vm.RejectedError
		if errors.As(err, &rejected) {
			err = temporal.NewNonRetryableApplicationError(rejected.Message, "Rejected", nil)
			return
		}
		var permanent *provider.PermanentError
		if errors.As(err, &permanent) {
			err = temporal.NewNonRetryableApplicationError(permanent.Message, "ProviderRejected", err)
		}
	}()
	if s.FailAt == "retry:"+request.Step {
		return result, errors.New("injected transient failure")
	}
	if s.FailAt == request.Step {
		return result, temporal.NewNonRetryableApplicationError("injected failure at "+request.Step, "Rejected", nil)
	}
	return s.Execution.Step(ctx, request)
}
