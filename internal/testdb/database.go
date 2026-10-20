// Package testdb creates isolated PostgreSQL schemas for integration tests.
package testdb

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// New returns a DSN with a fixed search_path. Cleanup only removes its own schema.
func New(t *testing.T, variable string) string {
	t.Helper()
	dsn := os.Getenv(variable)
	if dsn == "" {
		t.Skip(variable + " must point to a dedicated test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	schema := "test_" + uuid.New().String()
	name := pgx.Identifier{schema}.Sanitize()
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), "DROP SCHEMA "+name+" CASCADE")
		require.NoError(t, err)
	})
	return dsnWithSchema(t, dsn, name)
}

func dsnWithSchema(t *testing.T, dsn, schema string) string {
	t.Helper()
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " search_path='" + schema + "'"
}
