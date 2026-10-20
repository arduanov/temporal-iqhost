package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

func TestAdminResourceSnapshotsAndPagination(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	first := accept(t, store, "admin-a", vm.CreateVM)
	execute(t, store, first)
	second := accept(t, store, "admin-b", vm.CreateVM)
	execute(t, store, second)
	snapshot, err := store.AdminResources(ctx)
	require.NoError(t, err)
	var cpu, memory, disk, ips int64
	for _, node := range snapshot.Nodes {
		cpu += node.ReservedCpu
		memory += node.ReservedMemoryMib
	}
	for _, pool := range snapshot.Storage {
		disk += pool.ReservedGib
	}
	for _, pool := range snapshot.IPPools {
		ips += pool.Reserved
	}
	require.EqualValues(t, 8, cpu)
	require.EqualValues(t, 16384, memory)
	require.EqualValues(t, 40, disk)
	require.EqualValues(t, 2, ips)
	stop := accept(t, store, "admin-a", vm.StopVM)
	execute(t, store, stop)
	remove := accept(t, store, "admin-b", vm.DeleteVM)
	execute(t, store, remove)
	snapshot, err = store.AdminResources(ctx)
	require.NoError(t, err)
	cpu, disk, ips = 0, 0, 0
	for _, node := range snapshot.Nodes {
		cpu += node.ReservedCpu
	}
	for _, pool := range snapshot.Storage {
		disk += pool.ReservedGib
	}
	for _, pool := range snapshot.IPPools {
		ips += pool.Reserved
	}
	require.Zero(t, cpu)
	require.EqualValues(t, 20, disk)
	require.EqualValues(t, 1, ips)
	for _, gpu := range snapshot.GPUs {
		require.Empty(t, gpu.VmID)
	}
	checkAdminPagination(t, store)
	detail, err := store.AdminOperation(ctx, first.ID)
	require.NoError(t, err)
	require.NotEmpty(t, detail.Steps)
	for index := 1; index < len(detail.Steps); index++ {
		require.False(t, detail.Steps[index].CompletedAt.Time.Before(detail.Steps[index-1].CompletedAt.Time))
	}
}

func checkAdminPagination(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	rows, err := store.Q.AdminVMs(ctx, dbgen.AdminVMsParams{PageLimit: 1, PageOffset: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "admin-b", rows[0].ID)
	rows, err = store.Q.AdminVMs(ctx, dbgen.AdminVMsParams{PageLimit: 50, Search: "admin-", State: "stopped"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "admin-a", rows[0].ID)
	// Одинаковое время не нарушает стабильный порядок: второй ключ сортировки — UUID.
	_, err = store.Pool.Exec(ctx, "UPDATE vm_operations SET created_at='2026-01-01'::timestamptz")
	require.NoError(t, err)
	operations, err := store.Q.AdminOperations(ctx, dbgen.AdminOperationsParams{PageLimit: 50})
	require.NoError(t, err)
	require.Len(t, operations, 4)
	for index := range operations {
		page, pageErr := store.Q.AdminOperations(ctx, dbgen.AdminOperationsParams{PageLimit: 1, PageOffset: int32(index)})
		require.NoError(t, pageErr)
		require.Len(t, page, 1)
		require.Equal(t, operations[index].ID, page[0].ID)
	}
}
