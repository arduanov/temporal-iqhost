// Package execution выполняет шаги без зависимости от Temporal.
package execution

import (
	"context"

	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

type Store interface {
	LocalStep(context.Context, vm.StepRequest) (vm.StepResult, error)
	Prepare(context.Context, vm.StepRequest) (provider.Request, error)
	Commit(context.Context, provider.Request, provider.State) error
}
type Service struct {
	Store    Store
	Provider provider.Provider
}

func (s *Service) Step(ctx context.Context, request vm.StepRequest) (result vm.StepResult, err error) {
	switch request.Step {
	case vm.StepBegin, vm.StepBeginRollback:
		result, err = s.Store.LocalStep(ctx, request)
		if err != nil {
			return result, err
		}
		// Повтор подтверждает Fence и сверяет состояние с теми же сохранёнными ключами.
		err = s.executeProviderCommand(ctx, vm.StepRequest{OperationID: request.OperationID, Step: provider.StepInspect})
		return result, err
	case provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady, provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance:
		err = s.executeProviderCommand(ctx, request)
		return result, err
	default:
		return s.Store.LocalStep(ctx, request)
	}
}

// executeProviderCommand не держит транзакцию БД во время обращения к провайдеру.
func (s *Service) executeProviderCommand(ctx context.Context, request vm.StepRequest) error {
	if err := s.confirmFence(ctx, request.OperationID); err != nil {
		return err
	}
	savedRequest, err := s.Store.Prepare(ctx, request)
	if err != nil {
		return err
	}
	state, err := s.callProvider(ctx, request.Step, savedRequest)
	if err != nil {
		return err
	}
	if request.Step == provider.StepDeleteInstance && (state.Exists || state.PoweredOn || state.NetworkAttached) {
		return provider.Permanent("delete did not confirm absence")
	}
	return s.Store.Commit(ctx, savedRequest, state)
}

// Fence подтверждает, что команды прежних поколений больше не смогут менять VM.
// У барьера отдельный сохранённый запрос, не зависящий от выделенных ресурсов.
func (s *Service) confirmFence(ctx context.Context, operationID string) error {
	fence, err := s.Store.Prepare(ctx, vm.StepRequest{OperationID: operationID, Step: provider.StepFence})
	if err != nil {
		return err
	}
	if err = s.Provider.Fence(ctx, fence); err != nil {
		return err
	}
	return s.Store.Commit(ctx, fence, provider.State{})
}

func (s *Service) callProvider(ctx context.Context, step string, request provider.Request) (provider.State, error) {
	switch step {
	case provider.StepCreate:
		return s.Provider.Create(ctx, request)
	case provider.StepAttachNetwork:
		return s.Provider.AttachNetwork(ctx, request)
	case provider.StepStart:
		return s.Provider.Start(ctx, request)
	case provider.StepWaitReady:
		return s.Provider.WaitReady(ctx, request)
	case provider.StepStop:
		return s.Provider.Stop(ctx, request)
	case provider.StepDetachNetwork:
		return s.Provider.DetachNetwork(ctx, request)
	case provider.StepDeleteInstance:
		return s.Provider.Delete(ctx, request)
	case provider.StepInspect:
		return s.Provider.Inspect(ctx, request)
	default:
		return provider.State{}, provider.Permanent("unknown provider command")
	}
}
