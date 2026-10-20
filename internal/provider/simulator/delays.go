package simulator

import (
	"context"
	"time"

	"temporal-iqhost/internal/provider"
)

// Delays задаёт длительности команд; нулевой профиль отключает ожидание в тестах.
type Delays struct {
	Create, AttachNetwork, Start, WaitReady time.Duration
	Stop, DetachNetwork, Delete             time.Duration
}

func (delays Delays) forStep(step string) time.Duration {
	switch step {
	case provider.StepCreate:
		return delays.Create
	case provider.StepAttachNetwork:
		return delays.AttachNetwork
	case provider.StepStart:
		return delays.Start
	case provider.StepWaitReady:
		return delays.WaitReady
	case provider.StepStop:
		return delays.Stop
	case provider.StepDetachNetwork:
		return delays.DetachNetwork
	case provider.StepDeleteInstance:
		return delays.Delete
	default:
		return 0
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
