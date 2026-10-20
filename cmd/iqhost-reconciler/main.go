package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/client"

	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/reconcile"
)

func main() {
	slog.SetDefault(logging.New(os.Stdout, os.Getenv("LOG_LEVEL")))
	if err := run(); err != nil {
		// Ошибки транспорта могут содержать DSN, поэтому выводим только безопасное сообщение.
		slog.Error("process failed", "component", "reconciler", "event", "process_failed")
		os.Exit(1)
	}
}

func run() (runErr error) {
	address := flag.String("address", env("TEMPORAL_ADDRESS", "localhost:7233"), "Temporal address")

	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Сигнал во время подключения — штатная остановка, как и сигнал в рабочем цикле.
	defer func() {
		if ctx.Err() != nil {
			runErr = nil
		}
	}()
	store, err := postgres.Open(ctx, env("DATABASE_URL", "postgres://iqhost:iqhost-local@localhost:5432/iqhost?sslmode=disable"))
	if err != nil {
		return err
	}
	defer store.Pool.Close()
	temporalClient, err := client.DialContext(ctx, client.Options{
		HostPort: *address,
		Logger:   logging.Temporal{Log: slog.Default().With("component", "workflow", "event", "temporal_sdk")},
	})
	if err != nil {
		return err
	}
	defer temporalClient.Close()
	service := &reconcile.Reconciler{Store: store, Client: temporalClient}
	service.Run(ctx)
	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
