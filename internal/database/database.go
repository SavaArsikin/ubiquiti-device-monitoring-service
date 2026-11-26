package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context) (*sql.DB, error) {
	cfg := config.Get()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to open database")
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	defer cancel()

	pingErr := db.PingContext(pingCtx)
	if pingErr != nil {
		// Check the child context before returning so cancellation/deadline remains detectable by the caller.
		contextErr := pingCtx.Err()

		closeErr := db.Close()

		var openError error
		if contextErr != nil {
			openError = fmt.Errorf("database connectivity check: %w", contextErr)
		} else {
			// Don't return the raw driver error; it may reveal connection details.
			openError = errors.New("database connectivity check failed")
		}

		if closeErr != nil {
			openError = errors.Join(openError, errors.New("closing database connection pool failed"))
		}

		return nil, fmt.Errorf("connect to database: %w", openError)
	}

	slog.Info("database connected")

	return db, nil
}
