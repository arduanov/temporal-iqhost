package simulator

import (
	"context"

	"temporal-iqhost/internal/provider"
)

func (s *Simulator) Fence(ctx context.Context, request provider.Request) error {
	_, commandErr := s.run(ctx, request, provider.StepFence)
	return commandErr
}

func (s *Simulator) Create(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepCreate)
}

func (s *Simulator) AttachNetwork(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepAttachNetwork)
}

func (s *Simulator) Start(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepStart)
}

func (s *Simulator) WaitReady(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepWaitReady)
}

func (s *Simulator) Stop(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepStop)
}

func (s *Simulator) DetachNetwork(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepDetachNetwork)
}

func (s *Simulator) Delete(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepDeleteInstance)
}

func (s *Simulator) Inspect(ctx context.Context, request provider.Request) (provider.State, error) {
	return s.run(ctx, request, provider.StepInspect)
}

func applyCommand(request provider.Request, step string, state *provider.State) error {
	switch step {
	case provider.StepFence:
		*state = provider.State{}
	case provider.StepCreate:
		if request.Resources.NodeID == 0 || request.Resources.DiskID == 0 || request.Resources.IP == "" {
			return provider.Permanent("resources missing")
		}
		state.Exists = true
		state.NodeID = request.Resources.NodeID
	case provider.StepAttachNetwork:
		if !state.Exists {
			return provider.Permanent("instance missing")
		}
		state.NetworkAttached = true
	case provider.StepStart:
		if !state.Exists || !state.NetworkAttached {
			return provider.Permanent("instance not connected")
		}
		state.PoweredOn = true
	case provider.StepWaitReady:
		if !state.Exists || !state.PoweredOn || !state.NetworkAttached {
			return provider.Permanent("instance not ready")
		}
	case provider.StepStop:
		state.PoweredOn = false
	case provider.StepDetachNetwork:
		if state.PoweredOn {
			return provider.Permanent("instance still powered on")
		}
		state.NetworkAttached = false
	case provider.StepDeleteInstance:
		if state.PoweredOn || state.NetworkAttached {
			return provider.Permanent("instance still active")
		}
		*state = provider.State{}
	case provider.StepInspect:
	default:
		return provider.Permanent("unknown command")
	}
	return nil
}
