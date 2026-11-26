package devicestatus_test

import (
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
	"github.com/google/go-cmp/cmp"
)

func TestCreateDeviceStatusHistory(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusHistoryRepository(db)

	t.Run("successfully create history entry", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		device := testutils.InsertNewDeviceIntoDb(t, db, nil)
		input := testutils.NewDeviceStatusHistory(device.ID)

		created, err := repository.Create(t.Context(), &input, nil)
		if err != nil {
			t.Fatalf("create history entry: %v", err)
		}
		if created == nil {
			t.Fatal("expected created history entry, got nil")
		}
		if created.ID == "" {
			t.Fatal("expected a generated history ID")
		}

		expected := input
		expected.ID = created.ID
		if diff := cmp.Diff(expected, *created); diff != "" {
			t.Errorf("created history mismatch (-want +got):\n%s", diff)
		}

		var persisted devicestatus.DeviceStatusHistory
		err = db.QueryRowContext(t.Context(), `
			SELECT id::text, device_id::text, checked_at, reachability_status,
				consecutive_failures, diagnostics_complete, failure_category,
				device_reported_status, hardware_version, software_version,
				firmware_version, checksum
			FROM device_status_history WHERE id = $1
		`, created.ID).Scan(
			&persisted.ID, &persisted.DeviceID, &persisted.CheckedAt,
			&persisted.ReachabilityStatus, &persisted.ConsecutiveFailures,
			&persisted.DiagnosticsComplete, &persisted.FailureCategory,
			&persisted.DeviceReportedStatus, &persisted.HardwareVersion,
			&persisted.SoftwareVersion, &persisted.FirmwareVersion,
			&persisted.Checksum,
		)
		if err != nil {
			t.Fatalf("read persisted history: %v", err)
		}
		if diff := cmp.Diff(*created, persisted); diff != "" {
			t.Errorf("persisted history mismatch (-created +persisted):\n%s", diff)
		}
	})
}

func TestDeleteDeviceStatusHistoryBefore(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusHistoryRepository(db)

	t.Run("delete only history older than cutoff", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		device := testutils.InsertNewDeviceIntoDb(t, db, nil)

		older := testutils.InsertNewDeviceStatusHistoryIntoDb(t, db, nil, device.ID)
		atCutoff := testutils.InsertNewDeviceStatusHistoryIntoDb(t, db, nil, device.ID)
		newer := testutils.InsertNewDeviceStatusHistoryIntoDb(t, db, nil, device.ID)

		cutoff := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
		for _, entry := range []struct {
			id        string
			checkedAt time.Time
		}{
			{older.ID, cutoff.Add(-time.Hour)},
			{atCutoff.ID, cutoff},
			{newer.ID, cutoff.Add(time.Hour)},
		} {
			_, err := db.ExecContext(t.Context(),
				"UPDATE device_status_history SET checked_at = $2 WHERE id = $1",
				entry.id, entry.checkedAt,
			)
			if err != nil {
				t.Fatalf("set history timestamp: %v", err)
			}
		}

		if err := repository.DeleteBefore(t.Context(), cutoff, nil); err != nil {
			t.Fatalf("delete old history: %v", err)
		}

		for _, entry := range []struct {
			id   string
			want bool
		}{
			{older.ID, false},
			{atCutoff.ID, true},
			{newer.ID, true},
		} {
			var exists bool
			err := db.QueryRowContext(t.Context(), `
				SELECT EXISTS (SELECT 1 FROM device_status_history WHERE id = $1)
			`, entry.id).Scan(&exists)
			if err != nil {
				t.Fatalf("check history entry: %v", err)
			}
			if exists != entry.want {
				t.Errorf("history %s exists = %t, want %t", entry.id, exists, entry.want)
			}
		}
	})
}
