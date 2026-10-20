package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

// Accept сохраняет операцию и outbox атомарно: принятая команда не потеряется
// при сбое между записью в БД и запуском Temporal.
func (s *Store) Accept(ctx context.Context, requestID string, command vm.Command) (vm.Operation, error) {
	var validationErr error
	command, validationErr = validateCommand(requestID, command)
	if validationErr != nil {
		return vm.Operation{}, validationErr
	}
	raw, _ := json.Marshal(command)
	var result vm.Operation
	err := s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		if err := createVMIfNeeded(ctx, queries, command); err != nil {
			return err
		}
		machine, err := queries.LockVM(ctx, command.VMID)
		if err != nil {
			return err
		}
		old, err := queries.GetOperation(ctx, parseUUID(requestID))
		if err == nil {
			var previous vm.Command
			if json.Unmarshal(old.Request, &previous) != nil || previous != command {
				return invalid("request ID conflicts with another command")
			}
			result = operationFromRow(old)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		target, source, err := operationTarget(ctx, queries, machine, command)
		if err != nil {
			return err
		}
		operation, err := queries.InsertOperation(ctx, dbgen.InsertOperationParams{
			ID:                parseUUID(requestID),
			VmID:              command.VMID,
			Kind:              string(command.Kind),
			Request:           raw,
			WorkflowID:        operationWorkflowID(requestID, command),
			UserID:            command.UserID,
			RecoveryTarget:    target,
			SourceOperationID: source,
		})
		if err != nil {
			return err
		}
		if err = queries.OwnVM(ctx, dbgen.OwnVMParams{ID: command.VMID, CurrentOperationID: operation.ID}); err != nil {
			return err
		}
		if err = queries.Enqueue(ctx, operation.ID); err != nil {
			return err
		}
		result = operationFromRow(operation)
		return nil
	})
	return result, err
}

func operationTarget(ctx context.Context, queries *dbgen.Queries, machine dbgen.Vm, command vm.Command) (string, pgtype.UUID, error) {
	var err error
	var prior dbgen.VmOperation
	if machine.CurrentOperationID.Valid {
		prior, err = queries.LockOperation(ctx, machine.CurrentOperationID)
		if err != nil {
			return "", pgtype.UUID{}, err
		}
		if isActiveOperation(prior) {
			return "", pgtype.UUID{}, invalid("VM already has an active operation")
		}
	}
	if machine.RecoveryRequired && command.Kind != vm.Cleanup {
		return "", pgtype.UUID{}, invalid("VM requires cleanup")
	}
	target := "stopped"
	source := pgtype.UUID{}
	switch command.Kind {
	case vm.CreateVM:
		if machine.State != "not-created" || machine.Cpu != command.CPU || machine.MemoryMib != command.MemoryMiB || machine.Gpu != command.GPU || machine.DiskGib != command.DiskGiB {
			return "", pgtype.UUID{}, invalid("create conflicts with VM state or configuration")
		}
		target = "not-created"
	case vm.StartVM, vm.StopVM:
		if machine.State != "stopped" && machine.State != "running" {
			return "", pgtype.UUID{}, invalid("VM must be stopped or running")
		}
	case vm.DeleteVM:
		if machine.State != "stopped" && machine.State != "running" && machine.State != "deleted" {
			return "", pgtype.UUID{}, invalid("VM cannot be deleted from this state")
		}
		target = "deleted"
	case vm.Cleanup:
		if !machine.RecoveryRequired {
			return "", pgtype.UUID{}, invalid("VM does not require cleanup")
		}
		target = prior.RecoveryTarget
		source = prior.ID
	default:
		return "", pgtype.UUID{}, invalid("unknown operation")
	}
	return target, source, nil
}

func validateCommand(requestID string, command vm.Command) (vm.Command, error) {
	if !parseUUID(requestID).Valid || command.VMID == "" {
		return command, invalid("UUID request ID and VM ID required")
	}
	command.UserID = strings.TrimSpace(command.UserID)
	if command.UserID == "" {
		command.UserID = "unknown"
	}
	if !utf8.ValidString(command.UserID) || len(command.UserID) > 256 {
		return command, invalid("user ID must be valid UTF-8 and at most 256 bytes")
	}
	if len(operationWorkflowID(requestID, command)) > 1000 {
		return command, invalid("VM ID is too long for workflow ID")
	}
	if command.Kind == vm.CreateVM && (command.CPU < 1 || command.MemoryMiB < 1 || command.DiskGiB < 1) {
		return command, invalid("positive resource sizes required")
	}
	if command.Kind != vm.CreateVM {
		command.CPU = 0
		command.MemoryMiB = 0
		command.DiskGiB = 0
		command.GPU = false
	}
	return command, nil
}

func createVMIfNeeded(ctx context.Context, queries *dbgen.Queries, command vm.Command) error {
	if command.Kind == vm.CreateVM {
		if err := queries.InsertVM(ctx, dbgen.InsertVMParams{
			ID:        command.VMID,
			Cpu:       command.CPU,
			MemoryMib: command.MemoryMiB,
			Gpu:       command.GPU,
			DiskGib:   command.DiskGiB,
		}); err != nil {
			return err
		}
	}
	return nil
}

func operationWorkflowID(requestID string, command vm.Command) string {
	return "vm-operation/" + string(command.Kind) + "/" + url.PathEscape(command.VMID) + "/" + requestID
}
