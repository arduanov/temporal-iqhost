package activities_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/provider/simulator"
	"temporal-iqhost/internal/vm"
)

type intercepted struct {
	provider.Provider
	create func(context.Context, provider.Request) (provider.State, error)
	fence  func(context.Context, provider.Request) error
	delete func(context.Context, provider.Request) (provider.State, error)
}

func (p *intercepted) Create(c context.Context, r provider.Request) (provider.State, error) {
	if p.create != nil {
		return p.create(c, r)
	}
	return p.Provider.Create(c, r)
}

func (p *intercepted) Fence(c context.Context, r provider.Request) error {
	if p.fence != nil {
		return p.fence(c, r)
	}
	return p.Provider.Fence(c, r)
}

func (p *intercepted) Delete(c context.Context, r provider.Request) (provider.State, error) {
	if p.delete != nil {
		return p.delete(c, r)
	}
	return p.Provider.Delete(c, r)
}

func TestProviderLostResponseAndRestart(t *testing.T) {
	s := newEnvironment(t)
	ctx := context.Background()
	o := accept(t, s, "lost", vm.CreateVM)
	step(t, s, o, vm.StepBegin)
	step(t, s, o, vm.StepReserve)
	a := service(s)
	sim := a.Execution.Provider
	a.Execution.Provider = &intercepted{Provider: sim, create: func(c context.Context, r provider.Request) (provider.State, error) {
		_, e := sim.Create(c, r)
		require.NoError(t, e)
		return provider.State{}, errors.New("response lost")
	}}
	_, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
	require.Error(t, e)
	i, e := s.Q.Instance(ctx, o.VMID)
	require.NoError(t, e)
	require.False(t, i.Exists)
	// A fresh service/store represents a restart before application commit.
	restarted, e := postgres.Open(ctx, s.appDSN)
	require.NoError(t, e)
	defer restarted.Pool.Close()
	restartedSim := simulator.NewClient(s.simURL)
	restartedEnv := &testEnvironment{Store: restarted, simulator: restartedSim}
	step(t, restartedEnv, o, provider.StepCreate)
	i, e = s.Q.Instance(ctx, o.VMID)
	require.NoError(t, e)
	require.True(t, i.Exists)
	// Duplicate command responses cannot revert the later started state.
	step(t, s, o, provider.StepAttachNetwork)
	step(t, s, o, provider.StepStart)
	step(t, s, o, provider.StepCreate)
	i, e = s.Q.Instance(ctx, o.VMID)
	require.NoError(t, e)
	require.True(t, i.PoweredOn)
}

func TestCleanupAfterUnconfirmedEffect(t *testing.T) {
	s := newEnvironment(t)
	ctx := context.Background()
	o := accept(t, s, "effect-error", vm.CreateVM)
	step(t, s, o, vm.StepBegin)
	step(t, s, o, vm.StepReserve)
	a := service(s)
	sim := a.Execution.Provider
	a.Execution.Provider = &intercepted{Provider: sim, create: func(c context.Context, r provider.Request) (provider.State, error) {
		_, e := sim.Create(c, r)
		require.NoError(t, e)
		return provider.State{}, provider.Permanent("bad response")
	}}
	_, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
	var app *temporal.ApplicationError
	require.ErrorAs(t, e, &app)
	require.True(t, app.NonRetryable())
	_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepRelease})
	require.Error(t, e)
	step(t, s, o, vm.StepBeginRollback)
	for _, name := range []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease} {
		step(t, s, o, name)
		step(t, s, o, name)
	}
	finish(t, s, o, vm.OperationFailed)
	require.Nil(t, view(t, s, o.VMID)["compute"])
}

func TestProviderConcurrentRetriesConflictAndLateGeneration(t *testing.T) {
	s := newEnvironment(t)
	ctx := context.Background()
	o := accept(t, s, "concurrent", vm.CreateVM)
	step(t, s, o, vm.StepBegin)
	step(t, s, o, vm.StepReserve)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	req, e := s.Prepare(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
	require.NoError(t, e)
	sim := s.simulator
	changed := req
	changed.Config.CPU++
	_, e = sim.Create(ctx, changed)
	var permanent *provider.PermanentError
	require.ErrorAs(t, e, &permanent)
	step(t, s, o, vm.StepBeginRollback)
	v, e := s.Q.GetVM(ctx, o.VMID)
	require.NoError(t, e)
	generation := v.Generation
	step(t, s, o, vm.StepBeginRollback)
	v, e = s.Q.GetVM(ctx, o.VMID)
	require.NoError(t, e)
	require.Equal(t, generation, v.Generation)
	_, e = sim.Create(ctx, req)
	require.ErrorAs(t, e, &permanent)
	require.Error(t, s.Commit(ctx, req, provider.State{Exists: true}))
}

func TestProviderWaitDoesNotHoldApplicationLocks(t *testing.T) {
	for _, afterEffect := range []bool{false, true} {
		t.Run(fmt.Sprint(afterEffect), func(t *testing.T) {
			s := newEnvironment(t)
			ctx := context.Background()
			o := accept(t, s, "slow", vm.CreateVM)
			step(t, s, o, vm.StepBegin)
			step(t, s, o, vm.StepReserve)
			entered := make(chan struct{})
			resume := make(chan struct{})
			done := make(chan error, 1)
			a := service(s)
			sim := a.Execution.Provider
			a.Execution.Provider = &intercepted{Provider: sim, create: func(c context.Context, r provider.Request) (provider.State, error) {
				if afterEffect {
					state, err := sim.Create(c, r)
					close(entered)
					<-resume
					return state, err
				}
				close(entered)
				<-resume
				return sim.Create(c, r)
			}}
			go func() { _, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate}); done <- e }()
			<-entered
			// Reconciler can acquire VM/operation locks while the external request waits.
			c, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			e := s.Fence(c, o.ID)
			if e != nil {
				close(resume)
				if err := <-done; err != nil {
					t.Logf("create step after fence failure: %v", err)
				}
				t.Fatal(e)
			}
			cleanup := accept(t, s, o.VMID, vm.Cleanup)
			execute(t, s, cleanup)
			close(resume)
			require.Error(t, <-done)
			require.Nil(t, view(t, s, o.VMID)["compute"])
		})
	}
}

func TestFenceAndUnknownDeletePreserveReservations(t *testing.T) {
	for _, failure := range []string{provider.StepFence, "delete"} {
		t.Run(failure, func(t *testing.T) {
			s := newEnvironment(t)
			ctx := context.Background()
			o := accept(t, s, "unsafe", vm.CreateVM)
			execute(t, s, o)
			o = accept(t, s, o.VMID, vm.DeleteVM)
			a := service(s)
			sim := a.Execution.Provider
			if failure == provider.StepFence {
				a.Execution.Provider = &intercepted{Provider: sim, fence: func(context.Context, provider.Request) error { return provider.Permanent("cannot fence") }}
				_, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepStop})
				require.Error(t, e)
			} else {
				step(t, s, o, provider.StepStop)
				step(t, s, o, provider.StepDetachNetwork)
				a.Execution.Provider = &intercepted{Provider: sim, delete: func(c context.Context, r provider.Request) (provider.State, error) {
					_, e := sim.Delete(c, r)
					require.NoError(t, e)
					return provider.State{}, errors.New("unknown result")
				}}
				_, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepDeleteInstance})
				require.Error(t, e)
			}
			_, e := a.Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepRelease})
			require.Error(t, e)
			require.NotNil(t, view(t, s, o.VMID)["compute"])
		})
	}
}

func TestCancellationPreservedAtProviderBoundary(t *testing.T) {
	s := newEnvironment(t)
	o := accept(t, s, "cancel", vm.CreateVM)
	step(t, s, o, vm.StepBegin)
	step(t, s, o, vm.StepReserve)
	a := service(s)
	a.Execution.Provider = &intercepted{Provider: a.Execution.Provider, create: func(context.Context, provider.Request) (provider.State, error) {
		return provider.State{}, context.Canceled
	}}
	_, e := a.Step(context.Background(), vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
	require.ErrorIs(t, e, context.Canceled)
	require.NotNil(t, view(t, s, o.VMID)["compute"])
}
