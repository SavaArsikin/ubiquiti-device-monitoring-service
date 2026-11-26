package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/database"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "local setup:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	fmt.Println("starting local db setup")

	_, err = config.Init(".env.monitor")
	if err != nil {
		return fmt.Errorf("init config: %w", err)
	}

	err = database.LocalSetup(context.Background(), false)
	if err != nil {
		return fmt.Errorf("local database setup: %w", err)
	}

	fmt.Println("local db setup done")

	fmt.Println("starting test db setup")

	tempFile, err := os.CreateTemp("", ".env.monitor-test")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}

	path := tempFile.Name()
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil {
			err = errors.Join(err, errors.New("remove temporary config failed"))
		}
	}()

	_, writeErr := tempFile.WriteString(testutils.ValidEnvFile)
	closeErr := tempFile.Close()
	if writeErr != nil {
		err = fmt.Errorf("write temporary config: %w", writeErr)
	}
	if closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close temporary config: %w", closeErr))
	}
	if err != nil {
		return err
	}

	_, err = config.Init(path)
	if err != nil {
		return fmt.Errorf("init test config: %w", err)
	}

	err = database.LocalSetup(context.Background(), true)
	if err != nil {
		return fmt.Errorf("test database setup: %w", err)
	}

	return nil
}
