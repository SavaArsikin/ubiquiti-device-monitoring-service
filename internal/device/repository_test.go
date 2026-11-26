package device_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateDevice(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("successfully create new device", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		deviceInput := testutils.NewDevice()

		created, err := repository.Create(t.Context(), &deviceInput, nil)
		if err != nil {
			t.Fatalf("failed to create device %v", err)
		}
		if created == nil {
			t.Fatal("expected saved device, got nil")
		}
		if created.ID == "" {
			t.Error("expected a generated device ID")
		}
		if created.Name != deviceInput.Name || created.HealthURL != deviceInput.HealthURL {
			t.Errorf("returned device does not match input: %+v", created)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Fatalf("expected non zero created and updated timestamps")
		}
		if created.ArchivedAt != nil {
			t.Error("expected new device to be active")
		}

		var persisted device.Device

		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices
			WHERE id = $1
		`, created.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}

		if diff := cmp.Diff(*created, persisted); diff != "" {
			t.Errorf("persisted device mismatch (-returned +persisted):\n%s", diff)
		}

	})

	for _, tc := range []struct {
		name   string
		commit bool
	}{
		{name: "create with transaction and commit", commit: true},
		{name: "create with transaction and rollback", commit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutils.TruncateTables(t, db)
			tx := testutils.BeginTransaction(t, db)
			input := testutils.NewDevice()

			created, err := repository.Create(t.Context(), &input, tx)
			if err != nil {
				t.Fatalf("create device: %v", err)
			}
			if created == nil {
				t.Fatal("expected saved device, got nil")
			}

			var persisted device.Device
			err = tx.QueryRowContext(t.Context(), `
				SELECT id::text, name, health_url, created_at, updated_at, archived_at
				FROM devices
				WHERE id = $1
			`, created.ID).Scan(
				&persisted.ID,
				&persisted.Name,
				&persisted.HealthURL,
				&persisted.CreatedAt,
				&persisted.UpdatedAt,
				&persisted.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read device within transaction: %v", err)
			}
			if diff := cmp.Diff(*created, persisted); diff != "" {
				t.Errorf("persisted device mismatch (-returned +persisted):\n%s", diff)
			}

			// A separate connection must not see the uncommitted row while tx is open
			var id string
			err = db.QueryRowContext(t.Context(),
				"SELECT id::text FROM devices WHERE id = $1", created.ID,
			).Scan(&id)
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected uncommitted device to be invisible, got error: %v", err)
			}

			if tc.commit {
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit transaction: %v", err)
				}
			} else {
				if err := tx.Rollback(); err != nil {
					t.Fatalf("rollback transaction: %v", err)
				}
			}

			// Check visibility after commit/rollback
			err = db.QueryRowContext(t.Context(),
				"SELECT id::text FROM devices WHERE id = $1", created.ID,
			).Scan(&id)
			if tc.commit {
				if err != nil {
					t.Fatalf("read committed device: %v", err)
				}
				if id != created.ID {
					t.Errorf("got ID %q, want %q", id, created.ID)
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected rolled-back device to be absent, got error: %v", err)
			}
		})
	}

	t.Run("reject nil input", func(t *testing.T) {
		created, err := repository.Create(t.Context(), nil, nil)
		if err == nil {
			t.Fatal("expected an error for nil input")
		}
		if created != nil {
			t.Errorf("expected nil device, got %+v", created)
		}
	})

	t.Run("return error for canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		input := testutils.NewDevice()

		created, err := repository.Create(ctx, &input, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if created != nil {
			t.Errorf("expected nil device, got %+v", created)
		}
	})
}

func TestGetDevice(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("get device without transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		created := testutils.InsertNewDeviceIntoDb(t, db, nil)

		got, err := repository.Get(t.Context(), created.ID, nil)
		if err != nil {
			t.Fatalf("get device: %v", err)
		}
		if diff := cmp.Diff(created, got); diff != "" {
			t.Errorf("device mismatch (-created +got):\n%s", diff)
		}
	})

	t.Run("get uncommitted device in its transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		tx := testutils.BeginTransaction(t, db)
		created := testutils.InsertNewDeviceIntoDb(t, db, tx)

		got, err := repository.Get(t.Context(), created.ID, tx)
		if err != nil {
			t.Logf("err: %v", err)
			t.Fatalf("get device in transaction: %v", err)
		}
		if diff := cmp.Diff(created, got); diff != "" {
			t.Errorf("device mismatch (-created +got):\n%s", diff)
		}
	})

	t.Run("lock only when using a transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		created := testutils.InsertNewDeviceIntoDb(t, db, nil)
		tx := testutils.BeginTransaction(t, db)

		_, err := repository.Get(t.Context(), created.ID, tx)
		if err != nil {
			t.Fatalf("get device in transaction: %v", err)
		}

		// A second connection cannot acquire the row lock. No waiting for release
		var id string
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text FROM devices WHERE id = $1 FOR UPDATE NOWAIT
		`, created.ID).Scan(&id)

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			t.Fatalf("expected lock-not-available error, got: %v", err)
		}

		// An ordinary read should still work while the row is locked.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()

		got, err := repository.Get(ctx, created.ID, nil)
		if err != nil {
			t.Fatalf("get device without transaction while locked: %v", err)
		}
		if diff := cmp.Diff(created, got); diff != "" {
			t.Errorf("device mismatch (-created +got):\n%s", diff)
		}
	})

	t.Run("device not found", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		id := "00000000-0000-0000-0000-000000000000"

		got, err := repository.Get(t.Context(), id, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil device, got %+v", got)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		got, err := repository.Get(ctx, "00000000-0000-0000-0000-000000000000", nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil device, got %+v", got)
		}
	})
}

func TestListActiveDevices(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("return an empty list", func(t *testing.T) {
		testutils.TruncateTables(t, db)

		got, err := repository.ListActive(t.Context(), nil)
		if err != nil {
			t.Fatalf("list devices: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected no devices, got %d", len(got))
		}
	})

	t.Run("return active devices in order", func(t *testing.T) {
		testutils.TruncateTables(t, db)

		later := testutils.InsertNewDeviceIntoDb(t, db, nil)
		earlier := testutils.InsertNewDeviceIntoDb(t, db, nil)
		archived := testutils.InsertNewDeviceIntoDb(t, db, nil)

		// Set explicit timestamps so the ordering test is deterministic.
		earlier.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		later.UpdatedAt = earlier.UpdatedAt.Add(time.Hour)

		for _, d := range []*device.Device{earlier, later} {
			_, err := db.ExecContext(t.Context(),
				"UPDATE devices SET updated_at = $2 WHERE id = $1",
				d.ID, d.UpdatedAt,
			)
			if err != nil {
				t.Fatalf("set device timestamp: %v", err)
			}
		}

		_, err := db.ExecContext(t.Context(),
			"UPDATE devices SET archived_at = CURRENT_TIMESTAMP WHERE id = $1",
			archived.ID,
		)
		if err != nil {
			t.Fatalf("archive test device: %v", err)
		}

		got, err := repository.ListActive(t.Context(), nil)
		if err != nil {
			t.Fatalf("list devices: %v", err)
		}

		want := []*device.Device{earlier, later}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("device list mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("include uncommitted device in its transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)

		tx := testutils.BeginTransaction(t, db)
		created := testutils.InsertNewDeviceIntoDb(t, db, tx)

		got, err := repository.ListActive(t.Context(), tx)
		if err != nil {
			t.Fatalf("list devices in transaction: %v", err)
		}
		if diff := cmp.Diff([]*device.Device{created}, got); diff != "" {
			t.Errorf("device list mismatch (-want +got):\n%s", diff)
		}

		outsideTx, err := repository.ListActive(t.Context(), nil)
		if err != nil {
			t.Fatalf("list devices outside transaction: %v", err)
		}
		if len(outsideTx) != 0 {
			t.Errorf("expected uncommitted device to be invisible, got %d devices", len(outsideTx))
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		got, err := repository.ListActive(ctx, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil list on error, got %+v", got)
		}
	})

	t.Run("closed transaction", func(t *testing.T) {
		tx := testutils.BeginTransaction(t, db)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback transaction: %v", err)
		}

		got, err := repository.ListActive(t.Context(), tx)
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected closed transaction error, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil list on error, got %+v", got)
		}
	})
}

func TestListArchivedDevices(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("return an empty list", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		testutils.InsertNewDeviceIntoDb(t, db, nil)

		got, err := repository.ListArchived(t.Context(), nil)
		if err != nil {
			t.Fatalf("list archived devices: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected no archived devices, got %d", len(got))
		}
	})

	t.Run("return archived devices in order", func(t *testing.T) {
		testutils.TruncateTables(t, db)

		later := testutils.InsertNewDeviceIntoDb(t, db, nil)
		earlier := testutils.InsertNewDeviceIntoDb(t, db, nil)
		testutils.InsertNewDeviceIntoDb(t, db, nil)

		baseTime := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Hour)
		for _, entry := range []struct {
			device    *device.Device
			updatedAt time.Time
		}{
			{earlier, baseTime},
			{later, baseTime.Add(time.Hour)},
		} {
			err := db.QueryRowContext(t.Context(), `
				UPDATE devices
				SET archived_at = $2, updated_at = $2
				WHERE id = $1
				RETURNING archived_at, updated_at
			`, entry.device.ID, entry.updatedAt).Scan(
				&entry.device.ArchivedAt,
				&entry.device.UpdatedAt,
			)
			if err != nil {
				t.Fatalf("archive test device: %v", err)
			}
		}

		got, err := repository.ListArchived(t.Context(), nil)
		if err != nil {
			t.Fatalf("list archived devices: %v", err)
		}

		want := []*device.Device{earlier, later}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("device list mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("include uncommitted archive in its transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)
		tx := testutils.BeginTransaction(t, db)

		err := tx.QueryRowContext(t.Context(), `
			UPDATE devices
			SET archived_at = CURRENT_TIMESTAMP,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING archived_at, updated_at
		`, existing.ID).Scan(&existing.ArchivedAt, &existing.UpdatedAt)
		if err != nil {
			t.Fatalf("archive device in transaction: %v", err)
		}

		got, err := repository.ListArchived(t.Context(), tx)
		if err != nil {
			t.Fatalf("list archived devices in transaction: %v", err)
		}
		if diff := cmp.Diff([]*device.Device{existing}, got); diff != "" {
			t.Errorf("device list mismatch (-want +got):\n%s", diff)
		}

		outsideTx, err := repository.ListArchived(t.Context(), nil)
		if err != nil {
			t.Fatalf("list archived devices outside transaction: %v", err)
		}
		if len(outsideTx) != 0 {
			t.Errorf("uncommitted archive was visible, got %d devices", len(outsideTx))
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		got, err := repository.ListArchived(ctx, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil list, got %+v", got)
		}
	})

	t.Run("closed transaction", func(t *testing.T) {
		tx := testutils.BeginTransaction(t, db)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback transaction: %v", err)
		}

		got, err := repository.ListArchived(t.Context(), tx)
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected closed transaction error, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil list, got %+v", got)
		}
	})
}

func TestUpdateDevice(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("update device without transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		existing.Name = "Updated device"

		updated, err := repository.Update(t.Context(), existing, nil)
		if err != nil {
			t.Fatalf("update device: %v", err)
		}
		if updated == nil {
			t.Fatal("expected updated device, got nil")
		}
		if !updated.UpdatedAt.After(existing.UpdatedAt) {
			t.Error("expected updated timestamp to advance")
		}
		if updated.Name != "Updated device" {
			t.Error("name not updated")
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices
			WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}
		if diff := cmp.Diff(*updated, persisted); diff != "" {
			t.Errorf("persisted device mismatch (-updated +persisted):\n%s", diff)
		}
	})

	for _, tc := range []struct {
		name   string
		commit bool
	}{
		{name: "update with transaction and commit", commit: true},
		{name: "update with transaction and rollback", commit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutils.TruncateTables(t, db)
			existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

			// deliberately set old timestamp to test update
			err := db.QueryRowContext(t.Context(), `
				UPDATE devices
				SET updated_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
				WHERE id = $1
				RETURNING updated_at
			`, existing.ID).Scan(&existing.UpdatedAt)
			if err != nil {
				t.Fatalf("set old update timestamp: %v", err)
			}

			tx := testutils.BeginTransaction(t, db)

			input := *existing
			input.Name = "Updated device"

			updated, err := repository.Update(t.Context(), &input, tx)
			if err != nil {
				t.Fatalf("update device: %v", err)
			}
			if updated == nil {
				t.Fatal("expected updated device, got nil")
			}
			if !updated.UpdatedAt.After(existing.UpdatedAt) {
				t.Error("expected updated timestamp to advance")
			}
			if updated.Name != "Updated device" {
				t.Error("name not updated")
			}

			if tc.commit {
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit transaction: %v", err)
				}
			} else {
				if err := tx.Rollback(); err != nil {
					t.Fatalf("rollback transaction: %v", err)
				}
			}

			expected := existing
			if tc.commit {
				expected = updated
			}
			var persisted device.Device
			err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices
			WHERE id = $1
		`, existing.ID).Scan(
				&persisted.ID,
				&persisted.Name,
				&persisted.HealthURL,
				&persisted.CreatedAt,
				&persisted.UpdatedAt,
				&persisted.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read persisted device: %v", err)
			}
			if diff := cmp.Diff(*expected, persisted); diff != "" {
				t.Errorf("persisted device mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("reject nil input", func(t *testing.T) {
		updated, err := repository.Update(t.Context(), nil, nil)
		if err == nil {
			t.Fatal("expected an error for nil input")
		}
		if updated != nil {
			t.Errorf("expected nil device, got %+v", updated)
		}
	})

	t.Run("device not found", func(t *testing.T) {
		input := testutils.NewDevice()
		input.ID = "00000000-0000-0000-0000-000000000000"

		updated, err := repository.Update(t.Context(), &input, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if updated != nil {
			t.Errorf("expected nil device, got %+v", updated)
		}
	})

	t.Run("cannot update archived device", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		err := db.QueryRowContext(t.Context(), `
		UPDATE devices
		SET archived_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING archived_at
		`,
			existing.ID).Scan(&existing.ArchivedAt)
		if err != nil {
			t.Fatalf("archive test device: %v", err)
		}

		input := *existing
		input.Name = "Should not be saved"

		updated, err := repository.Update(t.Context(), &input, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if updated != nil {
			t.Errorf("expected nil device, got %+v", updated)
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
		SELECT id::text, name, health_url, created_at, updated_at, archived_at
		FROM devices
		WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}

		if diff := cmp.Diff(*existing, persisted); diff != "" {
			t.Errorf("archived device changed (-before +after):\n%s", diff)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		input := testutils.NewDevice()
		input.ID = "00000000-0000-0000-0000-000000000000"

		updated, err := repository.Update(ctx, &input, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if updated != nil {
			t.Errorf("expected nil device, got %+v", updated)
		}
	})
}

func TestArchiveDevice(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("archive device without transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		err := db.QueryRowContext(t.Context(), `
			UPDATE devices
			SET updated_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
			WHERE id = $1
			RETURNING updated_at
		`, existing.ID).Scan(&existing.UpdatedAt)
		if err != nil {
			t.Fatalf("set old update timestamp: %v", err)
		}

		if err := repository.Archive(t.Context(), existing.ID, nil); err != nil {
			t.Fatalf("archive device: %v", err)
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices
			WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}

		if persisted.ArchivedAt == nil {
			t.Fatal("expected device to be archived")
		}
		if !persisted.UpdatedAt.After(existing.UpdatedAt) {
			t.Error("expected updated timestamp to advance")
		}
		if !persisted.UpdatedAt.Equal(*persisted.ArchivedAt) {
			t.Error("expected updated and archived timestamps to match")
		}

		expected := *existing
		expected.UpdatedAt = persisted.UpdatedAt
		expected.ArchivedAt = persisted.ArchivedAt
		if diff := cmp.Diff(expected, persisted); diff != "" {
			t.Errorf("persisted device mismatch (-want +got):\n%s", diff)
		}
	})

	for _, tc := range []struct {
		name   string
		commit bool
	}{
		{name: "archive with transaction and commit", commit: true},
		{name: "archive with transaction and rollback", commit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutils.TruncateTables(t, db)
			existing := testutils.InsertNewDeviceIntoDb(t, db, nil)
			tx := testutils.BeginTransaction(t, db)

			if err := repository.Archive(t.Context(), existing.ID, tx); err != nil {
				t.Fatalf("archive device in transaction: %v", err)
			}

			var updated device.Device
			err := tx.QueryRowContext(t.Context(), `
				SELECT id::text, name, health_url, created_at, updated_at, archived_at
				FROM devices
				WHERE id = $1
			`, existing.ID).Scan(
				&updated.ID,
				&updated.Name,
				&updated.HealthURL,
				&updated.CreatedAt,
				&updated.UpdatedAt,
				&updated.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read device in transaction: %v", err)
			}
			if updated.ArchivedAt == nil {
				t.Fatal("expected device to be archived in transaction")
			}

			// A separate connection still sees the unarchived row.
			var notUpdated device.Device
			err = db.QueryRowContext(t.Context(), `
				SELECT id::text, name, health_url, created_at, updated_at, archived_at
				FROM devices
				WHERE id = $1
			`, existing.ID).Scan(
				&notUpdated.ID,
				&notUpdated.Name,
				&notUpdated.HealthURL,
				&notUpdated.CreatedAt,
				&notUpdated.UpdatedAt,
				&notUpdated.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read device outside transaction: %v", err)
			}
			if notUpdated.ArchivedAt != nil {
				t.Fatal("uncommitted archive was visible outside transaction")
			}

			if tc.commit {
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit transaction: %v", err)
				}
			} else {
				if err := tx.Rollback(); err != nil {
					t.Fatalf("rollback transaction: %v", err)
				}
			}

			expected := *existing
			if tc.commit {
				expected.ArchivedAt = updated.ArchivedAt
				expected.UpdatedAt = updated.UpdatedAt
			}

			var persisted device.Device
			err = db.QueryRowContext(t.Context(), `
				SELECT id::text, name, health_url, created_at, updated_at, archived_at
				FROM devices
				WHERE id = $1
			`, existing.ID).Scan(
				&persisted.ID,
				&persisted.Name,
				&persisted.HealthURL,
				&persisted.CreatedAt,
				&persisted.UpdatedAt,
				&persisted.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read persisted device: %v", err)
			}
			if diff := cmp.Diff(expected, persisted); diff != "" {
				t.Errorf("persisted device mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("device not found", func(t *testing.T) {
		id := "00000000-0000-0000-0000-000000000000"

		err := repository.Archive(t.Context(), id, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
	})

	t.Run("device already archived", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		err := db.QueryRowContext(t.Context(), `
			UPDATE devices SET archived_at = CURRENT_TIMESTAMP
			WHERE id = $1 RETURNING archived_at
		`, existing.ID).Scan(&existing.ArchivedAt)
		if err != nil {
			t.Fatalf("archive test device: %v", err)
		}

		err = repository.Archive(t.Context(), existing.ID, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}
		if diff := cmp.Diff(*existing, persisted); diff != "" {
			t.Errorf("archived device changed (-before +after):\n%s", diff)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := repository.Archive(ctx, "00000000-0000-0000-0000-000000000000", nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
	})

	t.Run("closed transaction", func(t *testing.T) {
		tx := testutils.BeginTransaction(t, db)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback transaction: %v", err)
		}

		err := repository.Archive(t.Context(), "00000000-0000-0000-0000-000000000000", tx)
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected closed transaction error, got: %v", err)
		}
	})
}

func TestRestoreDevice(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := device.NewDeviceRepository(db)

	t.Run("restore device without transaction", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		archivedAt := time.Now().UTC()
		_, err := db.ExecContext(t.Context(), `
			UPDATE devices SET archived_at = $2, updated_at = $2
			WHERE id = $1
		`, existing.ID, archivedAt)
		if err != nil {
			t.Fatalf("archive test device: %v", err)
		}
		existing.ArchivedAt = &archivedAt
		existing.UpdatedAt = archivedAt

		restored, err := repository.Restore(t.Context(), existing.ID, nil)
		if err != nil {
			t.Fatalf("restore device: %v", err)
		}
		if restored == nil {
			t.Fatal("expected restored device, got nil")
		}
		if restored.ArchivedAt != nil {
			t.Error("expected device to be active")
		}
		if !restored.UpdatedAt.After(existing.UpdatedAt) {
			t.Error("expected updated timestamp to advance")
		}

		expected := *existing
		expected.ArchivedAt = nil
		expected.UpdatedAt = restored.UpdatedAt
		if diff := cmp.Diff(expected, *restored); diff != "" {
			t.Errorf("restored device mismatch (-want +got):\n%s", diff)
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}
		if diff := cmp.Diff(*restored, persisted); diff != "" {
			t.Errorf("persisted device mismatch (-restored +persisted):\n%s", diff)
		}
	})

	for _, tc := range []struct {
		name   string
		commit bool
	}{
		{name: "restore with transaction and commit", commit: true},
		{name: "restore with transaction and rollback", commit: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutils.TruncateTables(t, db)
			existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

			archivedAt := time.Now().UTC()
			_, err := db.ExecContext(t.Context(), `
				UPDATE devices SET archived_at = $2, updated_at = $2
				WHERE id = $1
			`, existing.ID, archivedAt)
			if err != nil {
				t.Fatalf("archive test device: %v", err)
			}
			existing.ArchivedAt = &archivedAt
			existing.UpdatedAt = archivedAt

			tx := testutils.BeginTransaction(t, db)
			restored, err := repository.Restore(t.Context(), existing.ID, tx)
			if err != nil {
				t.Fatalf("restore device in transaction: %v", err)
			}
			if restored == nil {
				t.Fatal("expected restored device, got nil")
			}
			if restored.ArchivedAt != nil {
				t.Error("expected device to be active in transaction")
			}
			if !restored.UpdatedAt.After(existing.UpdatedAt) {
				t.Error("expected updated timestamp to advance")
			}

			expectedRestored := *existing
			expectedRestored.ArchivedAt = nil
			expectedRestored.UpdatedAt = restored.UpdatedAt
			if diff := cmp.Diff(expectedRestored, *restored); diff != "" {
				t.Errorf("restored device mismatch (-want +got):\n%s", diff)
			}

			// A separate connection still sees the archived device.
			var outsideArchivedAt *time.Time
			err = db.QueryRowContext(t.Context(), `
				SELECT archived_at FROM devices WHERE id = $1
			`, existing.ID).Scan(&outsideArchivedAt)
			if err != nil {
				t.Fatalf("read device outside transaction: %v", err)
			}
			if outsideArchivedAt == nil || !outsideArchivedAt.Equal(archivedAt) {
				t.Fatal("uncommitted restore was visible outside transaction")
			}

			if tc.commit {
				if err := tx.Commit(); err != nil {
					t.Fatalf("commit transaction: %v", err)
				}
			} else {
				if err := tx.Rollback(); err != nil {
					t.Fatalf("rollback transaction: %v", err)
				}
			}

			expected := existing
			if tc.commit {
				expected = restored
			}

			var persisted device.Device
			err = db.QueryRowContext(t.Context(), `
				SELECT id::text, name, health_url, created_at, updated_at, archived_at
				FROM devices WHERE id = $1
			`, existing.ID).Scan(
				&persisted.ID,
				&persisted.Name,
				&persisted.HealthURL,
				&persisted.CreatedAt,
				&persisted.UpdatedAt,
				&persisted.ArchivedAt,
			)
			if err != nil {
				t.Fatalf("read persisted device: %v", err)
			}
			if diff := cmp.Diff(*expected, persisted); diff != "" {
				t.Errorf("persisted device mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("device not found", func(t *testing.T) {
		id := "00000000-0000-0000-0000-000000000000"

		restored, err := repository.Restore(t.Context(), id, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if restored != nil {
			t.Errorf("expected nil device, got %+v", restored)
		}
	})

	t.Run("device already active", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		existing := testutils.InsertNewDeviceIntoDb(t, db, nil)

		restored, err := repository.Restore(t.Context(), existing.ID, nil)
		if !errors.Is(err, device.ErrDeviceNotFound) {
			t.Fatalf("expected device not found, got: %v", err)
		}
		if restored != nil {
			t.Errorf("expected nil device, got %+v", restored)
		}

		var persisted device.Device
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, name, health_url, created_at, updated_at, archived_at
			FROM devices WHERE id = $1
		`, existing.ID).Scan(
			&persisted.ID,
			&persisted.Name,
			&persisted.HealthURL,
			&persisted.CreatedAt,
			&persisted.UpdatedAt,
			&persisted.ArchivedAt,
		)
		if err != nil {
			t.Fatalf("read persisted device: %v", err)
		}
		if diff := cmp.Diff(*existing, persisted); diff != "" {
			t.Errorf("active device changed (-before +after):\n%s", diff)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		restored, err := repository.Restore(ctx, "00000000-0000-0000-0000-000000000000", nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got: %v", err)
		}
		if restored != nil {
			t.Errorf("expected nil device, got %+v", restored)
		}
	})

	t.Run("closed transaction", func(t *testing.T) {
		tx := testutils.BeginTransaction(t, db)
		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback transaction: %v", err)
		}

		restored, err := repository.Restore(t.Context(), "00000000-0000-0000-0000-000000000000", tx)
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("expected closed transaction error, got: %v", err)
		}
		if restored != nil {
			t.Errorf("expected nil device, got %+v", restored)
		}
	})
}
