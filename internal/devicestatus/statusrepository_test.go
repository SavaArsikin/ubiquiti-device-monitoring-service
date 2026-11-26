package devicestatus_test

import (
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
	"github.com/google/go-cmp/cmp"
)

func TestCreateDeviceStatus(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusRepository(db)

	t.Run("successfully create device status", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		device := testutils.InsertNewDeviceIntoDb(t, db, nil)
		input := testutils.NewDeviceStatus(device.ID)

		created, err := repository.Create(t.Context(), &input, nil)
		if err != nil {
			t.Fatalf("create device status: %v", err)
		}
		if created == nil {
			t.Fatal("expected created status, got nil")
		}
		if diff := cmp.Diff(input, *created); diff != "" {
			t.Errorf("created status mismatch (-input +created):\n%s", diff)
		}

		var persisted devicestatus.DeviceStatus
		err = db.QueryRowContext(t.Context(), `
			SELECT device_id::text, reachability_status, consecutive_failures,
				last_checked_at, last_reachable_at, last_failure_category,
				device_reported_status, diagnostics_complete, hardware_version,
				software_version, firmware_version, checksum, diagnostics_observed_at
			FROM device_statuses WHERE device_id = $1
		`, device.ID).Scan(
			&persisted.DeviceID, &persisted.ReachabilityStatus, &persisted.ConsecutiveFailures,
			&persisted.LastCheckedAt, &persisted.LastReachableAt, &persisted.LastFailureCategory,
			&persisted.DeviceReportedStatus, &persisted.DiagnosticsComplete, &persisted.HardwareVersion,
			&persisted.SoftwareVersion, &persisted.FirmwareVersion, &persisted.Checksum,
			&persisted.DiagnosticsObservedAt,
		)
		if err != nil {
			t.Fatalf("read persisted status: %v", err)
		}
		if diff := cmp.Diff(*created, persisted); diff != "" {
			t.Errorf("persisted status mismatch (-created +persisted):\n%s", diff)
		}
	})
}

func TestGetDeviceStatus(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusRepository(db)

	t.Run("successfully get device status", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		device := testutils.InsertNewDeviceIntoDb(t, db, nil)
		existing := testutils.InsertNewDeviceStatusIntoDb(t, db, nil, device.ID)

		got, err := repository.Get(t.Context(), device.ID, nil)
		if err != nil {
			t.Fatalf("get device status: %v", err)
		}
		if diff := cmp.Diff(existing, got); diff != "" {
			t.Errorf("status mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestListActiveDeviceStatuses(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusRepository(db)

	t.Run("list statuses for active devices", func(t *testing.T) {
		testutils.TruncateTables(t, db)

		activeDevice := testutils.InsertNewDeviceIntoDb(t, db, nil)
		activeStatus := testutils.InsertNewDeviceStatusIntoDb(t, db, nil, activeDevice.ID)

		archivedDevice := testutils.InsertNewDeviceIntoDb(t, db, nil)
		testutils.InsertNewDeviceStatusIntoDb(t, db, nil, archivedDevice.ID)
		_, err := db.ExecContext(t.Context(),
			"UPDATE devices SET archived_at = CURRENT_TIMESTAMP WHERE id = $1",
			archivedDevice.ID,
		)
		if err != nil {
			t.Fatalf("archive test device: %v", err)
		}

		got, err := repository.ListActive(t.Context(), nil)
		if err != nil {
			t.Fatalf("list active statuses: %v", err)
		}
		if diff := cmp.Diff([]*devicestatus.DeviceStatus{activeStatus}, got); diff != "" {
			t.Errorf("status list mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestUpdateDeviceStatus(t *testing.T) {
	db := testutils.OpenDatabase(t)
	repository := devicestatus.NewDeviceStatusRepository(db)

	t.Run("successfully update device status", func(t *testing.T) {
		testutils.TruncateTables(t, db)
		device := testutils.InsertNewDeviceIntoDb(t, db, nil)
		existing := testutils.InsertNewDeviceStatusIntoDb(t, db, nil, device.ID)

		checkedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
		failureCategory := "timeout"
		input := *existing
		input.ReachabilityStatus = devicestatus.ReachabilityDegraded
		input.ConsecutiveFailures = 1
		input.DiagnosticsComplete = false
		input.LastCheckedAt = &checkedAt
		input.LastFailureCategory = &failureCategory

		updated, err := repository.Update(t.Context(), &input, nil)
		if err != nil {
			t.Fatalf("update device status: %v", err)
		}
		if updated == nil {
			t.Fatal("expected updated status, got nil")
		}
		if diff := cmp.Diff(input, *updated); diff != "" {
			t.Errorf("updated status mismatch (-input +updated):\n%s", diff)
		}

		var persisted devicestatus.DeviceStatus
		err = db.QueryRowContext(t.Context(), `
			SELECT device_id::text, reachability_status, consecutive_failures,
				last_checked_at, last_reachable_at, last_failure_category,
				device_reported_status, diagnostics_complete, hardware_version,
				software_version, firmware_version, checksum, diagnostics_observed_at
			FROM device_statuses WHERE device_id = $1
		`, device.ID).Scan(
			&persisted.DeviceID, &persisted.ReachabilityStatus, &persisted.ConsecutiveFailures,
			&persisted.LastCheckedAt, &persisted.LastReachableAt, &persisted.LastFailureCategory,
			&persisted.DeviceReportedStatus, &persisted.DiagnosticsComplete, &persisted.HardwareVersion,
			&persisted.SoftwareVersion, &persisted.FirmwareVersion, &persisted.Checksum,
			&persisted.DiagnosticsObservedAt,
		)
		if err != nil {
			t.Fatalf("read persisted status: %v", err)
		}
		if diff := cmp.Diff(*updated, persisted); diff != "" {
			t.Errorf("persisted status mismatch (-updated +persisted):\n%s", diff)
		}
	})
}
