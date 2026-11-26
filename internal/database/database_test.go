package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/database"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
)

func TestOpen(t *testing.T) {
	testutils.InitConfig(t, testutils.ValidEnvFile)

	t.Run("successfully connects to the db", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		db, err := database.Open(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		var result int
		if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
			t.Fatal("database query failed")
		}

		if result != 1 {
			t.Errorf("query result = %d, want 1", result)
		}

		var devicesExists bool
		err = db.QueryRowContext(ctx,
			"SELECT to_regclass('public.devices') IS NOT NULL",
		).Scan(&devicesExists)
		if err != nil {
			t.Fatal("read devices table columns failed")
		}
		if !devicesExists {
			t.Fatal("devices table is missing; run local setup first")
		}
	})

	t.Run("fails with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		db, err := database.Open(ctx)
		if db != nil {
			db.Close()
			t.Fatal("expected no database pool")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		db, err := database.Open(ctx)
		if db != nil {
			db.Close()
			t.Fatal("expected no database pool")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	})
}
