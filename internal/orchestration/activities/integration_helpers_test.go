package activities_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/execution"
	"temporal-iqhost/internal/orchestration/activities"
	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/provider"
	"temporal-iqhost/internal/provider/simulator"
	"temporal-iqhost/internal/testdb"
	"temporal-iqhost/internal/vm"
)

// Каждый тест владеет отдельной схемой приложения и HTTP-сервером симулятора.
type testEnvironment struct {
	*postgres.Store
	simulator      *simulator.Client
	appDSN, simURL string
}

func newEnvironment(t *testing.T) *testEnvironment {
	t.Helper()
	appDSN := testdb.New(t, "TEST_DATABASE_URL")
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(ctx, appDSN, false))
	store, err := postgres.Open(ctx, appDSN)
	require.NoError(t, err)
	t.Cleanup(store.Pool.Close)
	require.NoError(t, store.Seed(ctx))
	server := httptest.NewServer(simulator.NewHandler(simulator.NewWithDelays(simulator.Delays{})))
	t.Cleanup(server.Close)
	return &testEnvironment{Store: store, simulator: simulator.NewClient(server.URL), appDSN: appDSN, simURL: server.URL}
}

func service(s *testEnvironment) *activities.Service {
	return &activities.Service{Execution: &execution.Service{Store: s.Store, Provider: s.simulator}}
}

func accept(t *testing.T, s *testEnvironment, id string, kind vm.OperationKind) vm.Operation {
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

func step(t *testing.T, s *testEnvironment, o vm.Operation, name string) {
	t.Helper()
	_, e := service(s).Step(context.Background(), vm.StepRequest{OperationID: o.ID, Step: name})
	require.NoError(t, e)
}

func finish(t *testing.T, s *testEnvironment, o vm.Operation, status vm.OperationStatus) {
	t.Helper()
	_, e := service(s).Step(context.Background(), vm.StepRequest{OperationID: o.ID, Step: vm.StepFinish, Status: status})
	require.NoError(t, e)
}

func execute(t *testing.T, s *testEnvironment, o vm.Operation) {
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

func view(t *testing.T, s *testEnvironment, id string) map[string]interface{} {
	t.Helper()
	raw, e := s.Get(context.Background(), id)
	require.NoError(t, e)
	var v map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}
