package testutils

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/database"
)

func TruncateTables(t testing.TB, db *sql.DB) {
	t.Helper()

	var name string
	if err := db.QueryRowContext(t.Context(), "SELECT current_database()").
		Scan(&name); err != nil {
		t.Fatal("identify test database failed")
	}
	if name != "device_monitoring_service_test" {
		t.Fatal("refusing to truncate outside the test database")
	}

	if _, err := db.ExecContext(
		t.Context(),
		"TRUNCATE TABLE device_status_history, device_statuses, devices RESTART IDENTITY"); err != nil {
		t.Fatal("truncate devices failed")
	}
}

func OpenDatabase(t *testing.T) *sql.DB {
	t.Helper()

	InitConfig(t, ValidEnvFile)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	db, err := database.Open(ctx)
	if err != nil {
		t.Fatal("connect to test database failed; run local setup first")
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error("close test database connection failed")
		}
	})

	TruncateTables(t, db)

	return db
}

func BeginTransaction(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()

	// In this case we can not use t.Context() because that context is cancelled before the Cleanup function runs.
	// So here we need to create a separate context and manage its lifecycle here.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("begin test transaction failed")
	}

	t.Cleanup(func() {
		// we are checking for ErrTxDone because tx may be committed or rolled back already
		// and that is not an error in cleanup, we did it on purpose in the test
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("rollback test transaction failed %v", err)
		}
	})

	return tx
}
