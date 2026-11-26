package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/logger"
	"github.com/jackc/pgx/v5"
)

func EnsureDatabase(ctx context.Context, cfg config.Config) (err error) {

	dbUrl, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return errors.New("DB_CONNECTION_STRING must be a valid URL")
	}

	databaseName := strings.TrimPrefix(dbUrl.Path, "/")

	adminDb, err := OpenMaintenanceConnection(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := adminDb.Close(); closeErr != nil {
			err = errors.Join(err,
				errors.New("close maintenance connection pool failed"))
		}
	}()

	var dbExists bool
	err = adminDb.QueryRowContext(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)",
		databaseName,
	).Scan(&dbExists)
	if err != nil {
		return errors.New("check test database existence failed")
	}

	if dbExists {
		return nil
	}

	createSql := "CREATE DATABASE " + pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminDb.ExecContext(ctx, createSql); err != nil {
		return errors.New("create test database failed")
	}

	return nil
}

func OpenMaintenanceConnection(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	maintenanceURL, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL")
	}
	maintenanceURL.Path = "/postgres"
	maintenanceURL.RawPath = "" // clear raw path so it causes no issues

	// Query parameters can override the database in the URL path.
	if maintenanceURL.Query().Has("dbname") {
		return nil, errors.New("database URL must not contain a dbname query parameter")
	}

	adminDb, err := sql.Open("pgx", maintenanceURL.String())
	if err != nil {
		return nil, errors.New("open maintenance connection failed")
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	pingErr := adminDb.PingContext(pingCtx)
	defer pingCancel()
	if pingErr != nil {
		failure := errors.New("connect to maintenance database failed")
		if closeErr := adminDb.Close(); closeErr != nil {
			failure = errors.Join(failure,
				errors.New("close maintenance connection pool failed"))
		}
		return nil, failure
	}

	return adminDb, nil
}

func LocalSetup(ctx context.Context, shouldMigrate bool) (err error) {
	cfg := config.Get()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	logger, err := logger.New(os.Stdout)
	if err != nil {
		return fmt.Errorf("initialized logger: %w", err)
	}
	slog.SetDefault(logger)

	// ensure test database exists - create if missing
	err = EnsureDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open test database: %w", err)
	}

	// connect to db
	db, err := Open(ctx)
	if err != nil {
		return fmt.Errorf("open test connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database connection pool failed"))
		}
	}()

	if shouldMigrate {
		// migrate database
		if err := Migrate(ctx, db); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	return nil
}
