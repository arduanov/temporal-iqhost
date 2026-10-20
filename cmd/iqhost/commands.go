package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/google/uuid"

	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/vm"
)

func executeCommand(ctx context.Context, store *postgres.Store, opts commandOptions) error {
	emit := func(value interface{}) error { return json.NewEncoder(os.Stdout).Encode(value) }
	switch opts.command {
	case "seed":
		return store.Seed(ctx)
	case "demo":
		return prepareDemo(ctx, store)
	case "get":
		value, err := store.Get(ctx, opts.id)
		if err != nil {
			return err
		}
		return emit(value)
	case "list":
		value, err := store.List(ctx)
		if err != nil {
			return err
		}
		return emit(value)
	case "operation":
		value, err := store.Operation(ctx, opts.id)
		if err != nil {
			return err
		}
		return emit(value)
	case "create", "start", "stop", "delete", "cleanup":
		return submitCommand(ctx, store, opts)
	default:
		return fmt.Errorf("unknown command")
	}
}

func submitCommand(ctx context.Context, store *postgres.Store, opts commandOptions) error {
	emit := func(value interface{}) error { return json.NewEncoder(os.Stdout).Encode(value) }
	if opts.requestID == "" {
		opts.requestID = uuid.NewString()
		slog.Info("request ID generated before submission", "component", "cli", "event", "request_id", "operation_id", opts.requestID)
	}
	command := vm.Command{VMID: opts.id, Kind: vm.OperationKind(opts.command), UserID: opts.userID}
	if opts.command == "create" {
		if opts.cpu < 1 || opts.cpu > math.MaxInt32 || opts.memory < 1 || opts.memory > math.MaxInt32 || opts.disk < 1 || opts.disk > math.MaxInt32 {
			return fmt.Errorf("resource size out of range")
		}
		command.CPU = int32(opts.cpu)
		command.MemoryMiB = int32(opts.memory)
		command.DiskGiB = int32(opts.disk)
		command.GPU = opts.gpu
	}
	operation, err := store.Accept(ctx, opts.requestID, command)
	if err != nil {
		return err
	}
	slog.Info("command accepted", "component", "cli", "event", "command_accepted", "user_id", operation.UserID, "operation_kind", operation.Kind, "vm_id", operation.VMID, "operation_id", operation.ID, "workflow_id", operation.WorkflowID)
	if opts.wait {
		operation, err = waitForOperation(ctx, store, operation)
		if err != nil {
			return err
		}
	}
	if err = emit(operation); err != nil {
		return err
	}
	if opts.wait && operation.Status != vm.Succeeded {
		return fmt.Errorf("operation did not succeed")
	}
	return nil
}

// Ожидаем подтверждённый результат из БД, включая ошибку или необходимость cleanup.
func waitForOperation(ctx context.Context, store interface {
	Operation(context.Context, string) (vm.Operation, error)
}, operation vm.Operation,
) (vm.Operation, error) {
	for !operation.Terminal() {
		select {
		case <-ctx.Done():
			return operation, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		var err error
		operation, err = store.Operation(ctx, operation.ID)
		if err != nil {
			return operation, err
		}
	}
	return operation, nil
}
