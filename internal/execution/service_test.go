package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

type recordingStore struct {
	calls  *[]string
	failAt string
}

func (store *recordingStore) LocalStep(context.Context, vm.StepRequest) (vm.StepResult, error) {
	*store.calls = append(*store.calls, "local")
	return vm.StepResult{}, nil
}

func (store *recordingStore) Prepare(_ context.Context, request vm.StepRequest) (provider.Request, error) {
	name := "prepare:" + request.Step
	*store.calls = append(*store.calls, name)
	if store.failAt == name {
		return provider.Request{}, errors.New("prepare failed")
	}
	return provider.Request{Key: request.Step}, nil
}

func (store *recordingStore) Commit(_ context.Context, request provider.Request, _ provider.State) error {
	name := "commit:" + request.Key
	*store.calls = append(*store.calls, name)
	if store.failAt == name {
		return errors.New("commit failed")
	}
	return nil
}

type recordingProvider struct {
	provider.Provider
	calls  *[]string
	failAt string
}

func (adapter *recordingProvider) Fence(context.Context, provider.Request) error {
	*adapter.calls = append(*adapter.calls, "fence")
	if adapter.failAt == "fence" {
		return errors.New("fence failed")
	}
	return nil
}

func (adapter *recordingProvider) Create(context.Context, provider.Request) (provider.State, error) {
	*adapter.calls = append(*adapter.calls, "create")
	if adapter.failAt == "create" {
		return provider.State{}, errors.New("response lost")
	}
	return provider.State{Exists: true}, nil
}

func TestProviderSequenceStopsAtFailure(t *testing.T) {
	sequence := []string{"prepare:fence", "fence", "commit:fence", "prepare:create", "create", "commit:create"}
	for index, failure := range append(append([]string{}, sequence...), "") {
		t.Run(failure, func(t *testing.T) {
			var calls []string
			service := Service{
				Store:    &recordingStore{calls: &calls, failAt: failure},
				Provider: &recordingProvider{calls: &calls, failAt: failure},
			}
			_, err := service.Step(context.Background(), vm.StepRequest{Step: provider.StepCreate})
			if failure == "" {
				require.NoError(t, err)
				require.Equal(t, sequence, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, sequence[:index+1], calls)
			}
		})
	}
}
