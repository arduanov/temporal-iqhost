package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDatabaseErrorsDoNotExposeValues(t *testing.T) {
	secret := "postgres://user:private-password@example/db"
	for _, e := range []error{errors.New(secret), &pgconn.PgError{Code: "23505", Message: secret, Detail: secret}} {
		require.NotContains(t, safeError(e), secret)
		require.NotContains(t, safeError(e), "private-password")
	}
}
