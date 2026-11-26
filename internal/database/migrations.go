package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"log/slog"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFs embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
	cfg := config.Get()

	// get migration files and check if accessible
	migrationFiles, err := fs.Sub(migrationsFs, "migrations")
	if err != nil {
		return errors.New("access embedded migrations failed")
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationFiles)
	if err != nil {
		return errors.New("initialize migrations failed")
	}

	migrateCtx, cancel := context.WithTimeout(ctx, cfg.DBMigrationTimeout)
	defer cancel()

	if _, err := provider.Up(migrateCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("migrations timed out")
		}
		if errors.Is(err, context.Canceled) {
			return errors.New("migrations canceled")
		}
		return errors.New("apply migrations failed")
	}

	slog.Info("database migrations applied")

	return nil
}
