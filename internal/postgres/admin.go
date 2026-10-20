package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"temporal-iqhost/internal/postgres/dbgen"
)

type ResourceSnapshot struct {
	Nodes   []dbgen.AdminNodesRow
	GPUs    []dbgen.AdminGPUsRow
	Storage []dbgen.AdminStorageRow
	IPPools []dbgen.AdminIPPoolsRow
}

type OperationSnapshot struct {
	Operation dbgen.VmOperation
	Delivery  dbgen.AdminDeliveryRow
	Steps     []dbgen.AdminStepsRow
}

// Все части ответа видят один снимок БД. Чтение не берёт блокировки размещения
// и заканчивается до передачи ответа браузеру.
func (s *Store) readSnapshot(ctx context.Context, read func(*dbgen.Queries) error) error {
	transaction, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if err = read(dbgen.New(transaction)); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

func (s *Store) AdminResources(ctx context.Context) (ResourceSnapshot, error) {
	var snapshot ResourceSnapshot
	err := s.readSnapshot(ctx, func(queries *dbgen.Queries) error {
		var err error
		snapshot.Nodes, err = queries.AdminNodes(ctx)
		if err != nil {
			return err
		}
		snapshot.GPUs, err = queries.AdminGPUs(ctx)
		if err != nil {
			return err
		}
		snapshot.Storage, err = queries.AdminStorage(ctx)
		if err != nil {
			return err
		}
		snapshot.IPPools, err = queries.AdminIPPools(ctx)
		return err
	})
	return snapshot, err
}

func (s *Store) AdminOperation(ctx context.Context, id string) (OperationSnapshot, error) {
	var snapshot OperationSnapshot
	err := s.readSnapshot(ctx, func(queries *dbgen.Queries) error {
		var err error
		snapshot.Operation, err = queries.GetOperation(ctx, parseUUID(id))
		if err != nil {
			return err
		}
		snapshot.Delivery, err = queries.AdminDelivery(ctx, parseUUID(id))
		if err != nil {
			return err
		}
		snapshot.Steps, err = queries.AdminSteps(ctx, parseUUID(id))
		return err
	})
	return snapshot, err
}
