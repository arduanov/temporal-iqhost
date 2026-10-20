package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/client"

	"temporal-iqhost/internal/logging"
	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/visibility"
)

func main() {
	output := os.Stderr
	if len(os.Args) > 1 && (os.Args[1] == "migrate" || os.Args[1] == "seed") {
		output = os.Stdout
	}
	slog.SetDefault(logging.New(output, os.Getenv("LOG_LEVEL")))
	if err := run(); err != nil {
		slog.Error("command failed", "component", "cli", "event", "command_failed", "error", publicError(err))
		os.Exit(1)
	}
}

// Database/transport errors may contain credentials; only expose our application errors.
func publicError(err error) string {
	type app interface{ Type() string }
	if applicationError, ok := err.(app); ok {
		return applicationError.Type() + ": " + err.Error()
	}
	return "command could not be completed"
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: iqhost create|start|stop|delete|cleanup|get|list|operation|migrate|seed|demo|temporal-setup")
	}
	opts, err := parseOptions()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if opts.command == "temporal-setup" {
		return setupTemporal(ctx, opts.address)
	}
	dsn := env("DATABASE_URL", "postgres://iqhost:iqhost-local@localhost:5432/iqhost?sslmode=disable")
	if opts.command == "migrate" {
		return postgres.Migrate(ctx, dsn, opts.down)
	}
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Pool.Close()
	return executeCommand(ctx, store, opts)
}

func setupTemporal(ctx context.Context, address string) error {
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	temporalClient, err := client.DialContext(setupCtx, client.Options{HostPort: address})
	if err != nil {
		return fmt.Errorf("connect to Temporal: %w", err)
	}
	defer temporalClient.Close()
	if err := visibility.Setup(setupCtx, temporalClient.OperatorService()); err != nil {
		slog.Error("Temporal search attributes setup failed", "error", err)
		return err
	}
	return nil
}
