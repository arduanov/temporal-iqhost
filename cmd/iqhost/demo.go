package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"temporal-iqhost/internal/vm"
)

type demoRepository interface {
	Accept(context.Context, string, vm.Command) (vm.Operation, error)
	Operation(context.Context, string) (vm.Operation, error)
}

// Демо использует обычный приём команд: ресурсы и история появляются благодаря
// workflow, а не прямой записи выдуманных результатов в таблицы.
func prepareDemo(ctx context.Context, store demoRepository) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	commands := []vm.Command{
		{
			VMID:      "demo-gpu",
			Kind:      vm.CreateVM,
			CPU:       4,
			MemoryMiB: 8192,
			DiskGiB:   40,
			GPU:       true,
		},
		{
			VMID:      "demo-stopped",
			Kind:      vm.CreateVM,
			CPU:       2,
			MemoryMiB: 2048,
			DiskGiB:   20,
		},
		{VMID: "demo-stopped", Kind: vm.StopVM},
		{
			VMID:      "demo-deleted",
			Kind:      vm.CreateVM,
			CPU:       2,
			MemoryMiB: 2048,
			DiskGiB:   10,
		},
		{VMID: "demo-deleted", Kind: vm.DeleteVM},
		{
			VMID:      "demo-no-capacity",
			Kind:      vm.CreateVM,
			CPU:       100,
			MemoryMiB: 2048,
			DiskGiB:   10,
		},
	}
	for _, command := range commands {
		command.UserID = "demo-customer"
		// ID не меняется при перезапуске: уже выполненные команды не повторяют эффект,
		// а прерванная подготовка продолжает ждать ранее принятую операцию.
		requestID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("iqhost-demo/v1/"+command.VMID+"/"+string(command.Kind))).String()
		operation, err := store.Accept(ctx, requestID, command)
		if err != nil {
			return fmt.Errorf("demo %s %s: %w", command.Kind, command.VMID, err)
		}
		operation, err = waitForOperation(ctx, store, operation)
		if err != nil {
			return err
		}
		expected := vm.Succeeded
		if command.VMID == "demo-no-capacity" {
			expected = vm.OperationFailed
		}
		if operation.Status != expected {
			return fmt.Errorf("demo %s %s: expected %s, got %s", command.Kind, command.VMID, expected, operation.Status)
		}
		slog.Info("demo scenario completed", "vm_id", command.VMID, "operation_kind", command.Kind, "status", operation.Status)
	}
	slog.Info("demo ready", "admin_url", "http://localhost:8082", "temporal_url", "http://localhost:8080")
	return nil
}
