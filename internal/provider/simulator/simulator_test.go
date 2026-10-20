package simulator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/provider"
)

func request(step string) provider.Request {
	return provider.Request{
		VMID:        "independent-vm",
		OperationID: "opaque-operation",
		Phase:       "forward",
		Step:        step,
		Key:         "opaque-operation/forward/" + step,
		Generation:  1,
		Config: provider.Config{
			CPU:       2,
			MemoryMiB: 1024,
			DiskGiB:   10,
		},
		Resources: provider.Resources{
			NodeID: 123,
			DiskID: 456,
			IP:     "192.0.2.10",
		},
	}
}

func TestLifecycleReplay(t *testing.T) {
	t.Parallel()
	simulator := NewWithDelays(Delays{})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	created, err := simulator.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.True(t, created.Exists)
	require.Equal(t, int64(123), created.NodeID)
	_, err = simulator.AttachNetwork(ctx, request(provider.StepAttachNetwork))
	require.NoError(t, err)
	_, err = simulator.Start(ctx, request(provider.StepStart))
	require.NoError(t, err)
	ready, err := simulator.WaitReady(ctx, request(provider.StepWaitReady))
	require.NoError(t, err)
	require.True(t, ready.PoweredOn)
	replay, err := simulator.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.Equal(t, created, replay)
	inspected, err := simulator.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.Equal(t, ready, inspected)
	for _, step := range []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance} {
		state, err := simulator.run(ctx, request(step), step)
		require.NoError(t, err)
		again, err := simulator.run(ctx, request(step), step)
		require.NoError(t, err)
		require.Equal(t, state, again)
		if step == provider.StepDeleteInstance {
			require.Equal(t, provider.State{}, state)
		}
	}
}

func TestConflictFenceAndCancellation(t *testing.T) {
	t.Parallel()
	simulator := NewWithDelays(Delays{})
	ctx := context.Background()
	var permanent *provider.PermanentError
	_, err := simulator.Create(ctx, request(provider.StepCreate))
	require.ErrorAs(t, err, &permanent)
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	command := request(provider.StepCreate)
	_, err = simulator.Create(ctx, command)
	require.NoError(t, err)
	changed := command
	changed.Config.CPU++
	_, err = simulator.Create(ctx, changed)
	require.ErrorAs(t, err, &permanent)
	fence := request(provider.StepFence)
	fence.Generation = 2
	fence.Phase = "rollback"
	fence.Key = fence.OperationID + "/rollback/" + provider.StepFence
	require.NoError(t, simulator.Fence(ctx, fence))
	require.NoError(t, simulator.Fence(ctx, fence))
	_, err = simulator.Create(ctx, command)
	require.ErrorAs(t, err, &permanent)
	require.ErrorAs(t, simulator.Fence(ctx, request(provider.StepFence)), &permanent)
	unfenced := command
	unfenced.Generation = 3
	_, err = simulator.Create(ctx, unfenced)
	require.ErrorAs(t, err, &permanent)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = simulator.Inspect(canceled, request(provider.StepInspect))
	require.ErrorIs(t, err, context.Canceled)
}

func TestMissingInstanceCleanup(t *testing.T) {
	t.Parallel()
	simulator := NewWithDelays(Delays{})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	for _, step := range []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, provider.StepInspect} {
		state, err := simulator.run(ctx, request(step), step)
		require.NoError(t, err)
		require.Equal(t, provider.State{}, state)
	}
}

func TestConcurrentRetries(t *testing.T) {
	t.Parallel()
	simulator := NewWithDelays(Delays{Create: 20 * time.Millisecond})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, err := simulator.Create(ctx, request(provider.StepCreate))
			require.NoError(t, err)
			require.True(t, state.Exists)
		}()
	}
	wg.Wait()
	require.Len(t, simulator.results, 2)
}

func TestNewSimulatorIsEmpty(t *testing.T) {
	original := NewWithDelays(Delays{})
	ctx := context.Background()
	require.NoError(t, original.Fence(ctx, request(provider.StepFence)))
	_, err := original.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	fresh := NewWithDelays(Delays{})
	require.Empty(t, fresh.states)
	require.Empty(t, fresh.generations)
	require.Empty(t, fresh.results)
	_, err = fresh.Create(ctx, request(provider.StepCreate))
	require.Error(t, err)
	require.NoError(t, fresh.Fence(ctx, request(provider.StepFence)))
	state, err := fresh.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.Equal(t, provider.State{}, state)
}

func TestDelayAndFastReplay(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: 80 * time.Millisecond})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	started := time.Now()
	created, err := simulator.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(started), 80*time.Millisecond)
	// Повтор должен успеть до короткого deadline, несмотря на задержку Create.
	replayCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	replay, err := simulator.Create(replayCtx, request(provider.StepCreate))
	require.NoError(t, err)
	require.Equal(t, created, replay)
}

func TestCancellationDuringDelay(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: time.Second})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	waiting, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err := simulator.Create(waiting, request(provider.StepCreate))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, simulator.states)
	require.Len(t, simulator.results, 1)
}

// observedContext сообщает, что команда дошла до ожидания таймера.
type observedContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *observedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestFenceDuringDelayAndOtherVM(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: 150 * time.Millisecond})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	waiting := &observedContext{Context: ctx, waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := simulator.Create(waiting, request(provider.StepCreate))
		result <- err
	}()
	<-waiting.waiting
	immediate, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	other := request(provider.StepFence)
	other.VMID = "other"
	other.OperationID = "other-operation"
	other.Key = other.OperationID + "/forward/" + other.Step
	require.NoError(t, simulator.Fence(immediate, other))
	other.Step = provider.StepInspect
	other.Key = other.OperationID + "/forward/" + other.Step
	_, err := simulator.Inspect(immediate, other)
	require.NoError(t, err)
	fence := request(provider.StepFence)
	fence.Generation = 2
	fence.Phase = "rollback"
	fence.Key = fence.OperationID + "/rollback/" + fence.Step
	require.NoError(t, simulator.Fence(immediate, fence))
	var permanent *provider.PermanentError
	require.ErrorAs(t, <-result, &permanent)
	require.Empty(t, simulator.states)
	require.NotContains(t, simulator.results, request(provider.StepCreate).Key)
}

func TestConcurrentDelayedConflicts(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: 30 * time.Millisecond})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	results := make(chan error, 2)
	for _, node := range []int64{1, 2} {
		go func() {
			command := request(provider.StepCreate)
			command.Resources.NodeID = node
			_, err := simulator.Create(ctx, command)
			results <- err
		}()
	}
	first, second := <-results, <-results
	if first == nil {
		require.Error(t, second)
	} else {
		require.NoError(t, second)
	}
	require.Len(t, simulator.results, 2)
}

func TestStateRecheckedAfterDelay(t *testing.T) {
	simulator := NewWithDelays(Delays{Delete: 50 * time.Millisecond})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	_, err := simulator.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	waiting := &observedContext{Context: ctx, waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, commandErr := simulator.Delete(waiting, request(provider.StepDeleteInstance))
		result <- commandErr
	}()
	<-waiting.waiting
	// Удаление было допустимо до ожидания, но сеть подключилась до фиксации.
	_, err = simulator.AttachNetwork(ctx, request(provider.StepAttachNetwork))
	require.NoError(t, err)
	var permanent *provider.PermanentError
	require.ErrorAs(t, <-result, &permanent)
	state, err := simulator.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.True(t, state.Exists)
	require.True(t, state.NetworkAttached)
}

func TestInvalidCommandDoesNotWait(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: time.Second, Start: time.Second})
	ctx := context.Background()
	require.NoError(t, simulator.Fence(ctx, request(provider.StepFence)))
	waiting, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	var permanent *provider.PermanentError
	_, err := simulator.Start(waiting, request(provider.StepStart))
	require.ErrorAs(t, err, &permanent)
	invalid := request(provider.StepCreate)
	invalid.Resources.DiskID = 0
	_, err = simulator.Create(waiting, invalid)
	require.ErrorAs(t, err, &permanent)
	invalid = request(provider.StepCreate)
	invalid.Key = "wrong-key"
	_, err = simulator.Create(waiting, invalid)
	require.ErrorAs(t, err, &permanent)
}
