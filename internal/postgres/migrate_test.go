package postgres

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/testdb"
)

func TestEmptyDatabaseConcurrentMigration(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.New(t, "TEST_DATABASE_URL")
	require.NoError(t, Migrate(ctx, dsn, false))
	s, err := Open(ctx, dsn)
	require.NoError(t, err)
	defer s.Pool.Close()
	var independent bool
	require.NoError(t, s.Pool.QueryRow(ctx, "SELECT to_regclass('simulated_instances') IS NULL AND to_regclass('simulator_commands') IS NULL AND to_regclass('simulator_generations') IS NULL").Scan(&independent))
	require.True(t, independent)
	require.NoError(t, Migrate(ctx, dsn, true))
	require.NoError(t, Migrate(ctx, dsn, true))
	require.NoError(t, Migrate(ctx, dsn, true))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- Migrate(ctx, dsn, false) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	require.NoError(t, s.Seed(ctx))
	v, e := s.List(ctx)
	require.NoError(t, e)
	require.Empty(t, v)
}

func TestMigrationFailureIsTransactional(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.New(t, "TEST_DATABASE_URL")
	db, e := sql.Open("pgx", dsn)
	require.NoError(t, e)
	defer func() { require.NoError(t, db.Close()) }()
	files := fstest.MapFS{"00001_failure.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE should_rollback(id int);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE should_rollback;\n")}}
	p, e := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithLogger(goose.NopLogger()))
	require.NoError(t, e)
	_, e = p.Up(ctx)
	require.Error(t, e)
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass('should_rollback') IS NOT NULL").Scan(&exists))
	require.False(t, exists)
}
