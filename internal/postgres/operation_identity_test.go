package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/vm"
)

func TestOperationIdentity(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	requestID := uuid.NewString()
	command := vm.Command{VMID: "alice/vm", UserID: "alice", Kind: vm.CreateVM, CPU: 1, MemoryMiB: 1024, DiskGiB: 10}
	operation, err := store.Accept(ctx, requestID, command)
	require.NoError(t, err)
	require.Equal(t, "alice", operation.UserID)
	require.Equal(t, "vm-operation/create/alice%2Fvm/"+requestID, operation.WorkflowID)
	saved, err := store.Operation(ctx, requestID)
	require.NoError(t, err)
	require.Equal(t, operation, saved)
	again, err := store.Accept(ctx, requestID, command)
	require.NoError(t, err)
	require.Equal(t, operation, again)
	command.UserID = "bob"
	_, err = store.Accept(ctx, requestID, command)
	require.ErrorContains(t, err, "conflicts")
}

func TestIdentityValidation(t *testing.T) {
	command := vm.Command{VMID: "vm-001", Kind: vm.StopVM}
	normalized, err := validateCommand(uuid.NewString(), command)
	require.NoError(t, err)
	require.Equal(t, "unknown", normalized.UserID)
	command.UserID = strings.Repeat("a", 257)
	_, err = validateCommand(uuid.NewString(), command)
	require.Error(t, err)
	command.UserID = "alice"
	command.VMID = strings.Repeat("v", 1000)
	_, err = validateCommand(uuid.NewString(), command)
	require.ErrorContains(t, err, "too long")
}
