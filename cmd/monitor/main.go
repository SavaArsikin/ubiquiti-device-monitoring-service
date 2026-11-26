package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/database"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "monitor service failed: ", err)
		os.Exit(1)
	}
}

func run() (err error) {
	_, err = config.Init(".env.monitor")
	if err != nil {
		return fmt.Errorf("initialized config: %w", err)
	}

	logger, err := logger.New(os.Stdout)
	if err != nil {
		return fmt.Errorf("initialized logger: %w", err)
	}
	slog.SetDefault(logger)

	logger.Info("initialized config and logger")

	db, err := database.Open(context.Background())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database connection pool failed"))
		}
	}()

	if err := database.Migrate(context.Background(), db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}
