package testutils

import (
	"database/sql"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

func NewDevice() device.Device {
	return device.Device{
		Name:      "TestDevice",
		HealthURL: "http://localhost:8081/health",
	}
}

func InsertNewDeviceIntoDb(t *testing.T, db *sql.DB, tx *sql.Tx) *device.Device {
	t.Helper()

	input := NewDevice()
	query := `
		INSERT INTO devices (id, name, health_url)
		VALUES (gen_random_uuid(), $1, $2)
		RETURNING id::text, name, health_url, created_at, updated_at, archived_at
	`

	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(t.Context(), query, input.Name, input.HealthURL)
	} else {
		row = db.QueryRowContext(t.Context(), query, input.Name, input.HealthURL)
	}

	var created device.Device
	err := row.Scan(
		&created.ID,
		&created.Name,
		&created.HealthURL,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.ArchivedAt,
	)
	if err != nil {
		t.Fatalf("insert test device: %v", err)
	}

	return &created
}

func NewDeviceStatus(deviceID string) devicestatus.DeviceStatus {
	checkedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	reportedStatus := "healthy"
	hardware := "1.0"
	software := "2.0"
	firmware := "3.0"
	checksum := "test-checksum"

	return devicestatus.DeviceStatus{
		DeviceID:              deviceID,
		ReachabilityStatus:    devicestatus.ReachabilityReachable,
		ConsecutiveFailures:   0,
		DiagnosticsComplete:   true,
		LastCheckedAt:         &checkedAt,
		LastReachableAt:       &checkedAt,
		DeviceReportedStatus:  &reportedStatus,
		HardwareVersion:       &hardware,
		SoftwareVersion:       &software,
		FirmwareVersion:       &firmware,
		Checksum:              &checksum,
		DiagnosticsObservedAt: &checkedAt,
	}
}

func NewDeviceStatusHistory(deviceID string) devicestatus.DeviceStatusHistory {
	status := NewDeviceStatus(deviceID)

	return devicestatus.DeviceStatusHistory{
		DeviceID:             deviceID,
		CheckedAt:            *status.LastCheckedAt,
		ReachabilityStatus:   status.ReachabilityStatus,
		ConsecutiveFailures:  status.ConsecutiveFailures,
		DiagnosticsComplete:  status.DiagnosticsComplete,
		FailureCategory:      status.LastFailureCategory,
		DeviceReportedStatus: status.DeviceReportedStatus,
		HardwareVersion:      status.HardwareVersion,
		SoftwareVersion:      status.SoftwareVersion,
		FirmwareVersion:      status.FirmwareVersion,
		Checksum:             status.Checksum,
	}
}

func InsertNewDeviceStatusIntoDb(
	t *testing.T,
	db *sql.DB,
	tx *sql.Tx,
	deviceID string,
) *devicestatus.DeviceStatus {
	t.Helper()

	input := NewDeviceStatus(deviceID)
	query := `
		INSERT INTO device_statuses (
			device_id, reachability_status, consecutive_failures,
			diagnostics_complete, last_checked_at, last_reachable_at,
			last_failure_category, device_reported_status, hardware_version,
			software_version, firmware_version, checksum, diagnostics_observed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING
			device_id::text, reachability_status, consecutive_failures,
			diagnostics_complete, last_checked_at, last_reachable_at,
			last_failure_category, device_reported_status, hardware_version,
			software_version, firmware_version, checksum, diagnostics_observed_at
	`
	args := []any{
		input.DeviceID, input.ReachabilityStatus, input.ConsecutiveFailures,
		input.DiagnosticsComplete, input.LastCheckedAt, input.LastReachableAt,
		input.LastFailureCategory, input.DeviceReportedStatus, input.HardwareVersion,
		input.SoftwareVersion, input.FirmwareVersion, input.Checksum, input.DiagnosticsObservedAt,
	}

	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(t.Context(), query, args...)
	} else {
		row = db.QueryRowContext(t.Context(), query, args...)
	}

	var created devicestatus.DeviceStatus
	err := row.Scan(
		&created.DeviceID, &created.ReachabilityStatus, &created.ConsecutiveFailures,
		&created.DiagnosticsComplete, &created.LastCheckedAt, &created.LastReachableAt,
		&created.LastFailureCategory, &created.DeviceReportedStatus, &created.HardwareVersion,
		&created.SoftwareVersion, &created.FirmwareVersion, &created.Checksum,
		&created.DiagnosticsObservedAt,
	)
	if err != nil {
		t.Fatalf("insert test device status: %v", err)
	}

	return &created
}

func InsertNewDeviceStatusHistoryIntoDb(
	t *testing.T,
	db *sql.DB,
	tx *sql.Tx,
	deviceID string,
) *devicestatus.DeviceStatusHistory {
	t.Helper()

	input := NewDeviceStatusHistory(deviceID)
	query := `
		INSERT INTO device_status_history (
			device_id, checked_at, reachability_status, consecutive_failures,
			diagnostics_complete, failure_category, device_reported_status,
			hardware_version, software_version, firmware_version, checksum
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING
			id::text, device_id::text, checked_at, reachability_status,
			consecutive_failures, diagnostics_complete, failure_category,
			device_reported_status, hardware_version, software_version,
			firmware_version, checksum
	`
	args := []any{
		input.DeviceID, input.CheckedAt, input.ReachabilityStatus,
		input.ConsecutiveFailures, input.DiagnosticsComplete, input.FailureCategory,
		input.DeviceReportedStatus, input.HardwareVersion, input.SoftwareVersion,
		input.FirmwareVersion, input.Checksum,
	}

	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(t.Context(), query, args...)
	} else {
		row = db.QueryRowContext(t.Context(), query, args...)
	}

	var created devicestatus.DeviceStatusHistory
	err := row.Scan(
		&created.ID, &created.DeviceID, &created.CheckedAt, &created.ReachabilityStatus,
		&created.ConsecutiveFailures, &created.DiagnosticsComplete, &created.FailureCategory,
		&created.DeviceReportedStatus, &created.HardwareVersion, &created.SoftwareVersion,
		&created.FirmwareVersion, &created.Checksum,
	)
	if err != nil {
		t.Fatalf("insert test device status history: %v", err)
	}

	return &created
}
