package orchestration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/vm"
)

func TestLifecycleWorkflow(t *testing.T) {
	for _, kind := range []vm.OperationKind{vm.CreateVM, vm.StartVM, vm.StopVM, vm.DeleteVM, vm.Cleanup} {
		steps := []string{vm.StepReserve, provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady}
		if kind != vm.CreateVM && kind != vm.StartVM {
			steps = []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease}
		}
		for _, fail := range append([]string{""}, steps...) {
			t.Run(fmt.Sprintf("%s/%s", kind, fail), func(t *testing.T) {
				var suite testsuite.WorkflowTestSuite
				env := suite.NewTestWorkflowEnvironment()
				var calls []vm.StepRequest
				failed := false
				RegisterStepActivities(env, func(_ context.Context, r vm.StepRequest) (vm.StepResult, error) {
					calls = append(calls, r)
					if r.Step == fail && !failed {
						failed = true
						return vm.StepResult{}, temporal.NewNonRetryableApplicationError("injected", "TestFailure", nil)
					}
					return vm.StepResult{Kind: kind, Target: vm.Stopped}, nil
				})
				env.ExecuteWorkflow(Lifecycle, "op")
				require.True(t, env.IsWorkflowCompleted())
				if fail == "" {
					require.NoError(t, env.GetWorkflowError())
					require.Equal(t, vm.Succeeded, calls[len(calls)-1].Status)
				} else {
					require.Error(t, env.GetWorkflowError())
					require.Equal(t, vm.OperationFailed, calls[len(calls)-1].Status)
					var tail []string
					for _, c := range calls[len(calls)-6:] {
						tail = append(tail, c.Step)
					}
					require.Equal(t, []string{vm.StepBeginRollback, provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease, vm.StepFinish}, tail)
				}
			})
		}
	}
}

func TestLifecycleBlockedAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			var final vm.OperationStatus
			RegisterStepActivities(env, func(ctx context.Context, r vm.StepRequest) (vm.StepResult, error) {
				if r.Step == provider.StepWaitReady {
					if cancel {
						<-ctx.Done()
						return vm.StepResult{}, ctx.Err()
					}
					return vm.StepResult{}, temporal.NewNonRetryableApplicationError("ready failed", "Test", nil)
				}
				if r.Step == provider.StepStop && !cancel {
					return vm.StepResult{}, temporal.NewNonRetryableApplicationError("stop failed", "Test", nil)
				}
				if r.Step == vm.StepFinish {
					final = r.Status
				}
				return vm.StepResult{Kind: vm.CreateVM, Target: vm.NotCreated}, nil
			})
			if cancel {
				env.RegisterDelayedCallback(env.CancelWorkflow, time.Second)
			}
			env.ExecuteWorkflow(Lifecycle, "op")
			require.Error(t, env.GetWorkflowError())
			if cancel {
				require.Equal(t, vm.OperationCanceled, final)
			} else {
				require.Equal(t, vm.Blocked, final)
			}
		})
	}
}

func TestLifecycleNoop(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	var calls []string
	RegisterStepActivities(env, func(_ context.Context, r vm.StepRequest) (vm.StepResult, error) {
		calls = append(calls, r.Step)
		return vm.StepResult{Kind: vm.StartVM, Noop: true}, nil
	})
	env.RegisterWorkflow(Lifecycle)
	env.ExecuteWorkflow("Lifecycle", "op")
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{vm.StepBegin, vm.StepFinish}, calls)
}

func TestEveryCompensationFailureBlocks(t *testing.T) {
	for _, fail := range []string{vm.StepBeginRollback, provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease} {
		t.Run(fail, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			var calls []vm.StepRequest
			RegisterStepActivities(env, func(_ context.Context, r vm.StepRequest) (vm.StepResult, error) {
				calls = append(calls, r)
				if r.Step == provider.StepWaitReady || r.Step == fail {
					return vm.StepResult{}, temporal.NewNonRetryableApplicationError("injected", "Test", nil)
				}
				return vm.StepResult{Kind: vm.CreateVM, Target: vm.NotCreated}, nil
			})
			env.ExecuteWorkflow(Lifecycle, "op")
			require.Error(t, env.GetWorkflowError())
			require.Equal(t, vm.Blocked, calls[len(calls)-1].Status)
			require.Equal(t, fail, calls[len(calls)-2].Step)
		})
	}
}

func TestLifecycleActivityNames(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	var names []string
	RegisterStepActivities(env, func(ctx context.Context, r vm.StepRequest) (vm.StepResult, error) {
		names = append(names, activity.GetInfo(ctx).ActivityType.Name)
		return vm.StepResult{Kind: vm.CreateVM}, nil
	})
	env.ExecuteWorkflow(Lifecycle, "op")
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"vm.begin-operation", "vm.reserve-resources", "vm.create-instance",
		"vm.attach-network", "vm.start-instance", "vm.wait-ready", "vm.finish-operation",
	}, names)
}
