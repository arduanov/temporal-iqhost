package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"temporal-iqhost/internal/postgres/dbgen"
	"temporal-iqhost/internal/vm"
)

type Store struct {
	Pool *pgxpool.Pool
	Q    *dbgen.Queries
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "5s"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "15s"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database pool initialization failed")
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unavailable")
	}
	return &Store{Pool: pool, Q: dbgen.New(pool)}, nil
}

func invalid(message string) error {
	return &vm.RejectedError{Message: message}
}

func (s *Store) Seed(ctx context.Context) error {
	return s.withTransaction(ctx, func(queries *dbgen.Queries) error {
		for _, seedResources := range []func(context.Context) error{queries.SeedNodes, queries.SeedGPUs, queries.SeedStorage, queries.SeedIPPool, queries.SeedIPs} {
			if err := seedResources(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}
