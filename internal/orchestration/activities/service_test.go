package activities

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"temporal-iqhost/internal/execution"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

type failingStore struct {
	execution.Store
	err error
}

func (store failingStore) LocalStep(context.Context, vm.StepRequest) (vm.StepResult, error) {
	return vm.StepResult{}, store.err
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		errorType string
	}{
		{"rejected", fmt.Errorf("wrapped: %w", &vm.RejectedError{Message: "stale owner"}), "Rejected"},
		{"provider", provider.Permanent("invalid request"), "ProviderRejected"},
		{"canceled", fmt.Errorf("wrapped: %w", context.Canceled), ""},
		{"deadline", context.DeadlineExceeded, ""},
		{"transient", errors.New("temporary failure"), ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service := Service{Execution: &execution.Service{Store: failingStore{err: test.err}}}
			_, err := service.Step(context.Background(), vm.StepRequest{Step: vm.StepReserve})
			if test.errorType == "" {
				require.ErrorIs(t, err, test.err)
				return
			}
			var applicationError *temporal.ApplicationError
			require.ErrorAs(t, err, &applicationError)
			require.Equal(t, test.errorType, applicationError.Type())
			require.True(t, applicationError.NonRetryable())
		})
	}
}
