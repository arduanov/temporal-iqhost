package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/vm"
)

type demoStore struct {
	commands map[string]vm.Command
	status   vm.OperationStatus
}

func (store *demoStore) Accept(_ context.Context, requestID string, command vm.Command) (vm.Operation, error) {
	store.commands[requestID] = command
	status := store.status
	if status == "" {
		status = vm.Succeeded
		if command.CPU == 100 {
			status = vm.OperationFailed
		}
	}
	return vm.Operation{ID: requestID, VMID: command.VMID, Status: status}, nil
}

func (store *demoStore) Operation(context.Context, string) (vm.Operation, error) {
	return vm.Operation{}, context.Canceled
}

func TestDemoUsesStableCommandsAndExpectedFailure(t *testing.T) {
	store := &demoStore{commands: make(map[string]vm.Command)}
	require.NoError(t, prepareDemo(context.Background(), store))
	require.Len(t, store.commands, 6)
	previous := make(map[string]vm.Command)
	for requestID, command := range store.commands {
		previous[requestID] = command
	}
	require.NoError(t, prepareDemo(context.Background(), store))
	require.Equal(t, previous, store.commands)
}

func TestDemoRejectsUnexpectedResult(t *testing.T) {
	store := &demoStore{commands: make(map[string]vm.Command), status: vm.Blocked}
	require.ErrorContains(t, prepareDemo(context.Background(), store), "rollback-blocked")
	require.Len(t, store.commands, 1)
}

func TestDemoStopsWaitingOnCancellation(t *testing.T) {
	store := &demoStore{commands: make(map[string]vm.Command), status: vm.Pending}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, prepareDemo(ctx, store), context.Canceled)
}
