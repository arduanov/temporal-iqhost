package postgres

import (
	"context"
	"errors"
	"time"

	"temporal-iqhost/internal/postgres/dbgen"
)

func (s *Store) withTransaction(ctx context.Context, execute func(*dbgen.Queries) error) error {
	for attempt := 0; ; attempt++ {
		transaction, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		err = execute(s.Q.WithTx(transaction))
		if err == nil {
			err = transaction.Commit(ctx)
		} else {
			_ = transaction.Rollback(ctx)
		}
		_ = transaction.Rollback(ctx)
		if err == nil || attempt >= 4 || !retryableTransactionError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
}

// PostgreSQL разрешает повторить всю транзакцию после этих конфликтов.
func retryableTransactionError(err error) bool {
	var databaseError interface{ SQLState() string }
	if !errors.As(err, &databaseError) {
		return false
	}
	switch databaseError.SQLState() {
	case "40001": // serialization_failure: конкурентное изменение данных
		return true
	case "40P01": // deadlock_detected: взаимная блокировка
		return true
	case "55P03": // lock_not_available: истёк срок ожидания блокировки
		return true
	default:
		return false
	}
}
