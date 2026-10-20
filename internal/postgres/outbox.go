package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

func (s *Store) Claim(ctx context.Context) (*vm.Delivery, error) {
	delivery, err := s.Q.Claim(ctx, parseUUID(uuid.NewString()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	operation, err := s.Q.GetOperation(ctx, delivery.OperationID)
	if err != nil {
		return nil, err
	}
	return &vm.Delivery{
		ID:        delivery.ID,
		Operation: operationFromRow(operation),
		Token:     formatUUID(delivery.LeaseToken),
		Attempts:  delivery.Attempts,
	}, nil
}

func (s *Store) Delivered(ctx context.Context, delivery vm.Delivery, runID string) error {
	return s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		operationID, err := queries.Delivered(ctx, dbgen.DeliveredParams{ID: delivery.ID, LeaseToken: parseUUID(delivery.Token)})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return queries.SetRunID(ctx, dbgen.SetRunIDParams{ID: operationID, RunID: runID})
	})
}

func (s *Store) RetryDelivery(ctx context.Context, delivery vm.Delivery, retryAt time.Time) error {
	return s.Q.RetryDelivery(ctx, dbgen.RetryDeliveryParams{
		ID:            delivery.ID,
		LeaseToken:    parseUUID(delivery.Token),
		NextAttemptAt: pgtype.Timestamptz{Time: retryAt, Valid: true},
		LastError:     "Temporal delivery failed",
	})
}
