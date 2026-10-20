package main

import (
	"context"
	"log/slog"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"temporal-iqhost/internal/execution"
	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/orchestration"
	"temporal-iqhost/internal/orchestration/activities"
	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/provider/simulator"
)

func runWorker(ctx context.Context, store *postgres.Store, simulatorURL, address, failAt string) error {
	simulatedProvider := simulator.NewClient(simulatorURL)
	activityService := &activities.Service{
		Execution: &execution.Service{
			Store:    store,
			Provider: simulatedProvider,
		},
		FailAt: failAt,
	}
	temporalClient, err := client.DialContext(ctx, client.Options{
		HostPort: address,
		Logger:   logging.Temporal{Log: slog.Default().With("component", "workflow", "event", "temporal_sdk")},
	})
	if err != nil {
		return err
	}
	defer temporalClient.Close()
	temporalWorker := worker.New(temporalClient, orchestration.TaskQueue, worker.Options{WorkerStopTimeout: 30 * time.Second})
	temporalWorker.RegisterWorkflow(orchestration.Lifecycle)
	orchestration.RegisterStepActivities(temporalWorker, activityService.Step)
	if err = temporalWorker.Start(); err != nil {
		return err
	}
	<-ctx.Done()
	temporalWorker.Stop()
	return nil
}
