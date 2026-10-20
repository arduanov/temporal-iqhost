package simulator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/provider"
)

func TestPersistentSimulatorRestoresStateResultsAndFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	adapter, err := NewPersistent(path)
	require.NoError(t, err)
	adapter.delays = Delays{}
	ctx := context.Background()
	require.NoError(t, adapter.Fence(ctx, request(provider.StepFence)))
	created, err := adapter.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	_, err = adapter.AttachNetwork(ctx, request(provider.StepAttachNetwork))
	require.NoError(t, err)
	_, err = adapter.Start(ctx, request(provider.StepStart))
	require.NoError(t, err)

	restored, err := NewPersistent(path)
	require.NoError(t, err)
	restored.delays = Delays{}
	replay, err := restored.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.Equal(t, created, replay)
	state, err := restored.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.True(t, state.PoweredOn)
	changed := request(provider.StepCreate)
	changed.Config.CPU++
	_, err = restored.Create(ctx, changed)
	require.ErrorContains(t, err, "content conflict")
	fence := request(provider.StepFence)
	fence.Generation++
	fence.OperationID = "next-operation"
	fence.Key = fence.OperationID + "/forward/" + provider.StepFence
	require.NoError(t, restored.Fence(ctx, fence))
	restored, err = NewPersistent(path)
	require.NoError(t, err)
	_, err = restored.Create(ctx, request(provider.StepCreate))
	require.ErrorContains(t, err, "stale")
}

func TestPersistentSimulatorRejectsCorruptSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte("broken"), 0o600))
	_, err := NewPersistent(path)
	require.Error(t, err)
}

func TestSnapshotFailureDoesNotApplyCommand(t *testing.T) {
	directory := t.TempDir()
	adapter, err := NewPersistent(filepath.Join(directory, "state.json"))
	require.NoError(t, err)
	adapter.delays = Delays{}
	require.NoError(t, os.RemoveAll(directory))
	require.Error(t, adapter.Fence(context.Background(), request(provider.StepFence)))
	require.Empty(t, adapter.generations)
	require.Empty(t, adapter.results)
}
