package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/execution"
	"temporal-iqhost/internal/orchestration/activities"
	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/provider/simulator"
	"temporal-iqhost/internal/testdb"
	"temporal-iqhost/internal/vm"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := testdb.New(t, "TEST_DATABASE_URL")
	ctx := context.Background()
	require.NoError(t, Migrate(ctx, dsn, false))
	s, e := Open(ctx, dsn)
	require.NoError(t, e)
	t.Cleanup(s.Pool.Close)
	server := httptest.NewServer(simulator.NewHandler(simulator.NewWithDelays(simulator.Delays{})))
	t.Cleanup(server.Close)
	sim := simulator.NewClient(server.URL)
	testProviders.Store(s, sim)
	testDSNs.Store(s, dsn)
	t.Cleanup(func() { testProviders.Delete(s); testDSNs.Delete(s) })
	require.NoError(t, s.Seed(ctx))
	return s
}

func accept(t *testing.T, s *Store, id string, kind vm.OperationKind) vm.Operation {
	t.Helper()
	c := vm.Command{VMID: id, Kind: kind}
	if kind == vm.CreateVM {
		c.CPU = 4
		c.MemoryMiB = 8192
		c.DiskGiB = 20
		c.GPU = true
	}
	o, e := s.Accept(context.Background(), uuid.NewString(), c)
	require.NoError(t, e)
	return o
}

func step(t *testing.T, s *Store, o vm.Operation, name string) {
	t.Helper()
	_, e := service(s).Step(context.Background(), vm.StepRequest{OperationID: o.ID, Step: name})
	require.NoError(t, e)
}

func finish(t *testing.T, s *Store, o vm.Operation, status vm.OperationStatus) {
	t.Helper()
	_, e := service(s).Step(context.Background(), vm.StepRequest{OperationID: o.ID, Step: vm.StepFinish, Status: status})
	require.NoError(t, e)
}

func execute(t *testing.T, s *Store, o vm.Operation) {
	t.Helper()
	step(t, s, o, vm.StepBegin)
	if o.Kind == vm.CreateVM || o.Kind == vm.StartVM {
		for _, n := range []string{vm.StepReserve, provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady} {
			step(t, s, o, n)
		}
	} else {
		for _, n := range []string{provider.StepStop, provider.StepDetachNetwork, provider.StepDeleteInstance, vm.StepRelease} {
			step(t, s, o, n)
		}
	}
	finish(t, s, o, vm.Succeeded)
}

func view(t *testing.T, s *Store, id string) map[string]interface{} {
	t.Helper()
	raw, e := s.Get(context.Background(), id)
	require.NoError(t, e)
	var v map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

func TestLifecycleAndIdempotency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	rid := uuid.NewString()
	c := vm.Command{VMID: "life", Kind: vm.CreateVM, CPU: 4, MemoryMiB: 8192, DiskGiB: 20, GPU: true}
	o, e := s.Accept(ctx, rid, c)
	require.NoError(t, e)
	again, e := s.Accept(ctx, rid, c)
	require.NoError(t, e)
	require.Equal(t, o, again)
	c.CPU++
	_, e = s.Accept(ctx, rid, c)
	require.Error(t, e)
	_, e = s.Accept(ctx, uuid.NewString(), vm.Command{VMID: "life", Kind: vm.StopVM})
	require.Error(t, e)
	step(t, s, o, vm.StepBegin)
	step(t, s, o, vm.StepReserve)
	step(t, s, o, vm.StepReserve) // commit response lost
	for _, n := range []string{provider.StepCreate, provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady} {
		step(t, s, o, n)
	}
	finish(t, s, o, vm.Succeeded)
	finish(t, s, o, vm.Succeeded)
	initial := view(t, s, "life")
	require.Equal(t, "running", initial["vm"].(map[string]interface{})["state"])
	stopped := accept(t, s, "life", vm.StopVM)
	execute(t, s, stopped)
	v := view(t, s, "life")
	require.Nil(t, v["compute"])
	require.Equal(t, initial["disk"], v["disk"])
	require.Equal(t, initial["ip"], v["ip"])
	// Force placement on the other node without altering existing reservations.
	_, e = s.Pool.Exec(ctx, "UPDATE compute_nodes SET available=false WHERE id=1")
	require.NoError(t, e)
	started := accept(t, s, "life", vm.StartVM)
	execute(t, s, started)
	v = view(t, s, "life")
	require.Equal(t, float64(2), v["compute"].(map[string]interface{})["node_id"])
	noop := accept(t, s, "life", vm.StartVM)
	r, e := service(s).Step(ctx, vm.StepRequest{OperationID: noop.ID, Step: vm.StepBegin})
	require.NoError(t, e)
	require.True(t, r.Noop)
	finish(t, s, noop, vm.Succeeded)
	deleted := accept(t, s, "life", vm.DeleteVM)
	execute(t, s, deleted)
	v = view(t, s, "life")
	require.Nil(t, v["compute"])
	require.Nil(t, v["disk"])
	require.Nil(t, v["ip"])
	noop = accept(t, s, "life", vm.DeleteVM)
	r, e = service(s).Step(ctx, vm.StepRequest{OperationID: noop.ID, Step: vm.StepBegin})
	require.NoError(t, e)
	require.True(t, r.Noop)
	finish(t, s, noop, vm.Succeeded)
	c.CPU--
	_, e = s.Accept(ctx, uuid.NewString(), c)
	require.Error(t, e)
	_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
	require.Error(t, e)
}

func TestConcurrentCapacityAndAtomicReservation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var ops []vm.Operation
	for i := 0; i < 8; i++ {
		ops = append(ops, accept(t, s, fmt.Sprint("parallel-", i), vm.CreateVM))
	}
	var wg sync.WaitGroup
	results := make(chan error, len(ops))
	for _, o := range ops {
		wg.Add(1)
		go func(o vm.Operation) {
			defer wg.Done()
			_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepReserve})
			results <- e
		}(o)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	require.Equal(t, 2, success)
	var count int
	require.NoError(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM compute_allocations WHERE released_at IS NULL").Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM vm_disks WHERE deleted_at IS NULL").Scan(&count))
	require.Equal(t, 2, count)
	// Storage failure rolls compute back.
	s = testStore(t)
	_, e := s.Pool.Exec(ctx, "UPDATE storage_pools SET capacity_gib=1")
	require.NoError(t, e)
	o := accept(t, s, "disk-full", vm.CreateVM)
	_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepReserve})
	require.Error(t, e)
	require.Nil(t, view(t, s, "disk-full")["compute"])
	_, e = s.Pool.Exec(ctx, "UPDATE storage_pools SET capacity_gib=200; DELETE FROM ip_addresses")
	require.NoError(t, e)
	_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepReserve})
	require.Error(t, e)
	require.Nil(t, view(t, s, "disk-full")["compute"])
	require.Nil(t, view(t, s, "disk-full")["disk"])
}

func TestRollbackFenceAndRecoveryTargets(t *testing.T) {
	for _, kind := range []vm.OperationKind{vm.CreateVM, vm.StartVM, vm.StopVM, vm.DeleteVM} {
		t.Run(string(kind), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			o := accept(t, s, "recover", vm.CreateVM)
			if kind != vm.CreateVM {
				execute(t, s, o)
				if kind == vm.StartVM {
					execute(t, s, accept(t, s, "recover", vm.StopVM))
				}
				o = accept(t, s, "recover", kind)
			}
			step(t, s, o, vm.StepBegin)
			if kind == vm.CreateVM || kind == vm.StartVM {
				step(t, s, o, vm.StepReserve)
				step(t, s, o, provider.StepCreate)
				step(t, s, o, provider.StepAttachNetwork)
				step(t, s, o, provider.StepStart)
			}
			_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepRelease})
			require.Error(t, e)
			step(t, s, o, vm.StepBeginRollback)
			_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepCreate})
			require.Error(t, e)
			finish(t, s, o, vm.Blocked)
			v := view(t, s, "recover")
			require.NotNil(t, v["compute"])
			_, e = s.Accept(ctx, uuid.NewString(), vm.Command{VMID: "recover", Kind: vm.StopVM})
			require.Error(t, e)
			cleanup := accept(t, s, "recover", vm.Cleanup)
			require.Equal(t, o.ID, cleanup.SourceOperationID)
			_, e = service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: provider.StepStop})
			require.Error(t, e)
			execute(t, s, cleanup)
			v = view(t, s, "recover")
			require.Equal(t, string(o.RecoveryTarget), v["vm"].(map[string]interface{})["state"])
			require.Nil(t, v["compute"])
			if kind == vm.StartVM || kind == vm.StopVM {
				require.NotNil(t, v["disk"])
				require.NotNil(t, v["ip"])
			} else {
				require.Nil(t, v["disk"])
				require.Nil(t, v["ip"])
			}
			if kind == vm.CreateVM {
				execute(t, s, accept(t, s, "recover", vm.CreateVM))
			}
		})
	}
}

func TestOutboxLeaseAndReconcilerRace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	o := accept(t, s, "lease", vm.CreateVM)
	var wg sync.WaitGroup
	deliveries := make(chan *vm.Delivery, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); d, e := s.Claim(ctx); deliveries <- d; errs <- e }()
	}
	wg.Wait()
	close(deliveries)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	var first *vm.Delivery
	for d := range deliveries {
		if d != nil {
			require.Nil(t, first)
			first = d
		}
	}
	require.NotNil(t, first)
	_, e := s.Pool.Exec(ctx, "UPDATE outbox SET locked_until=now()-interval '1 second'")
	require.NoError(t, e)
	second, e := s.Claim(ctx)
	require.NoError(t, e)
	require.NotNil(t, second)
	require.NotEqual(t, first.Token, second.Token)
	require.NoError(t, s.Delivered(ctx, *first, "stale-run"))
	rows, e := s.Candidates(ctx)
	require.NoError(t, e)
	require.Empty(t, rows)
	require.NoError(t, s.Delivered(ctx, *second, "valid-run"))
	rows, e = s.Candidates(ctx)
	require.NoError(t, e)
	require.Len(t, rows, 1)
	step(t, s, o, vm.StepReserve)
	require.NoError(t, s.Fence(ctx, o.ID))
	require.NoError(t, s.Fence(ctx, o.ID))
	v := view(t, s, "lease")
	require.NotNil(t, v["compute"])
	require.True(t, v["vm"].(map[string]interface{})["recovery_required"].(bool))
	execute(t, s, accept(t, s, "lease", vm.Cleanup))
	o = accept(t, s, "lease", vm.CreateVM)
	execute(t, s, o)
	require.NoError(t, s.Fence(ctx, o.ID))
	latest, e := s.Operation(ctx, o.ID)
	require.NoError(t, e)
	require.Equal(t, vm.Succeeded, latest.Status)
	d, e := s.Claim(ctx)
	require.NoError(t, e)
	require.Nil(t, d)
}

func TestLockWaitDoesNotMeanNoCapacity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	o := accept(t, s, "locked", vm.CreateVM)
	tx, e := s.Pool.Begin(ctx)
	require.NoError(t, e)
	// Rollback is a fallback; the transaction may already be committed or rolled back.
	defer func() { _ = tx.Rollback(ctx) }()
	_, e = dbgen.New(tx).LockNodes(ctx)
	require.NoError(t, e)
	done := make(chan error, 1)
	go func() {
		_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepReserve})
		done <- e
	}()
	select {
	case e := <-done:
		t.Fatalf("did not wait: %v", e)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)
}

func TestMigrationsAndSeed(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	dsn := storeDSN(s)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Migrate(ctx, dsn, false) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	o := accept(t, s, "seed", vm.CreateVM)
	execute(t, s, o)
	before := view(t, s, "seed")
	require.NoError(t, s.Seed(ctx))
	require.Equal(t, before, view(t, s, "seed"))
	require.NoError(t, Migrate(ctx, dsn, true))
	require.NoError(t, Migrate(ctx, dsn, false))
	require.NoError(t, s.Seed(ctx))
	var version int64
	require.NoError(t, s.Pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&version))
	require.Equal(t, int64(3), version)
}

func TestConcurrentAdmission(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	c := vm.Command{VMID: "same", Kind: vm.CreateVM, CPU: 1, MemoryMiB: 1024, DiskGiB: 1}
	rid := uuid.NewString()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	ids := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); o, e := s.Accept(ctx, rid, c); errs <- e; ids <- o.ID }()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		require.NoError(t, e)
	}
	for id := range ids {
		require.Equal(t, rid, id)
	}
	var count int
	require.NoError(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&count))
	require.Equal(t, 1, count)
}

func TestConcurrentCapacityDimensions(t *testing.T) {
	for _, dimension := range []string{"cpu", "ram", "storage", "ip"} {
		t.Run(dimension, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			c := vm.Command{Kind: vm.CreateVM, CPU: 1, MemoryMiB: 1, DiskGiB: 1}
			expected := 2
			switch dimension {
			case "cpu":
				c.CPU = 8
			case "ram":
				c.MemoryMiB = 16384
			case "storage":
				c.DiskGiB = 100
			case "ip":
				expected = 10
			}
			var ops []vm.Operation
			for i := 0; i < 12; i++ {
				c.VMID = fmt.Sprint(dimension, i)
				o, e := s.Accept(ctx, uuid.NewString(), c)
				require.NoError(t, e)
				ops = append(ops, o)
			}
			var wg sync.WaitGroup
			results := make(chan error, len(ops))
			for _, o := range ops {
				wg.Add(1)
				go func(o vm.Operation) {
					defer wg.Done()
					_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepReserve})
					results <- e
				}(o)
			}
			wg.Wait()
			close(results)
			count := 0
			for e := range results {
				if e == nil {
					count++
				}
			}
			require.Equal(t, expected, count)
			var compute, disks, ips int
			require.NoError(t, s.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM compute_allocations WHERE released_at IS NULL),(SELECT count(*) FROM vm_disks WHERE deleted_at IS NULL),(SELECT count(*) FROM ip_addresses WHERE vm_id IS NOT NULL)").Scan(&compute, &disks, &ips))
			require.Equal(t, expected, compute)
			require.Equal(t, expected, disks)
			require.Equal(t, expected, ips)
		})
	}
}

func TestConcurrentFinalizationAndReconcilers(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	o := accept(t, s, "final-race", vm.CreateVM)
	for _, name := range []string{vm.StepBegin, vm.StepReserve, provider.StepCreate, provider.StepAttachNetwork, provider.StepStart, provider.StepWaitReady} {
		step(t, s, o, name)
	}
	var wg sync.WaitGroup
	fenceErrors := make(chan error, 2)
	finishErrors := make(chan error, 1)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); fenceErrors <- s.Fence(ctx, o.ID) }()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, e := service(s).Step(ctx, vm.StepRequest{OperationID: o.ID, Step: vm.StepFinish, Status: vm.Succeeded})
		finishErrors <- e
	}()
	wg.Wait()
	close(fenceErrors)
	for e := range fenceErrors {
		require.NoError(t, e)
	}
	latest, e := s.Operation(ctx, o.ID)
	require.NoError(t, e)
	v := view(t, s, "final-race")
	require.NotNil(t, v["compute"])
	if latest.Status == vm.Succeeded {
		require.NoError(t, <-finishErrors)
		require.False(t, v["vm"].(map[string]interface{})["recovery_required"].(bool))
	} else {
		require.Equal(t, vm.Blocked, latest.Status)
		require.Error(t, <-finishErrors)
		require.True(t, v["vm"].(map[string]interface{})["recovery_required"].(bool))
	}
}

func service(s *Store) *activities.Service {
	return &activities.Service{Execution: &execution.Service{Store: s, Provider: simulatorProvider(s)}}
}

var (
	testProviders sync.Map
	testDSNs      sync.Map
)

func simulatorProvider(s *Store) *simulator.Client {
	v, _ := testProviders.Load(s)
	return v.(*simulator.Client)
}
func storeDSN(s *Store) string { v, _ := testDSNs.Load(s); return v.(string) }

func TestCommandValidation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, c := range []vm.Command{{Kind: vm.CreateVM}, {VMID: "bad", Kind: vm.CreateVM, CPU: 1, MemoryMiB: 1}, {VMID: "bad", Kind: vm.CreateVM, CPU: -1, MemoryMiB: 1, DiskGiB: 1}} {
		_, e := s.Accept(ctx, uuid.NewString(), c)
		require.Error(t, e)
	}
}
