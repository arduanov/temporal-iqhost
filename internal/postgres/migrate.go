package postgres

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(ctx context.Context, dsn string, down bool) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Error("close migration database", "error", closeErr)
		}
	}()
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker), goose.WithLogger(goose.NopLogger()))
	if err != nil {
		return err
	}
	slog.Info("migration started", "component", "migrate", "event", "migration_started", "down", down)
	if down {
		_, err = p.Down(ctx)
	} else {
		_, err = p.Up(ctx)
	}
	if err == nil {
		slog.Info("migration completed", "component", "migrate", "event", "migration_completed")
	}
	return err
}
