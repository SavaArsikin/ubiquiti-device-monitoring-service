package devicestatus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type DeviceStatusHistoryRepository struct {
	db *sql.DB
}

const deviceStatusHistoryColumns = `
	id::text,
	device_id::text,
	checked_at,
	reachability_status,
	consecutive_failures,
	diagnostics_complete,
	failure_category,
	device_reported_status,
	hardware_version,
	software_version,
	firmware_version,
	checksum
`

func NewDeviceStatusHistoryRepository(db *sql.DB) *DeviceStatusHistoryRepository {
	return &DeviceStatusHistoryRepository{db: db}
}

func scanDeviceStatusHistory(row rowScanner) (DeviceStatusHistory, error) {
	var history DeviceStatusHistory

	err := row.Scan(
		&history.ID,
		&history.DeviceID,
		&history.CheckedAt,
		&history.ReachabilityStatus,
		&history.ConsecutiveFailures,
		&history.DiagnosticsComplete,
		&history.FailureCategory,
		&history.DeviceReportedStatus,
		&history.HardwareVersion,
		&history.SoftwareVersion,
		&history.FirmwareVersion,
		&history.Checksum,
	)
	if err != nil {
		return DeviceStatusHistory{}, err
	}

	return history, nil
}

func (r *DeviceStatusHistoryRepository) Create(
	ctx context.Context,
	input *DeviceStatusHistory,
	tx *sql.Tx,
) (*DeviceStatusHistory, error) {
	if input == nil {
		return nil, errors.New("input device status history is nil")
	}

	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		INSERT INTO device_status_history (
			device_id,
			checked_at,
			reachability_status,
			consecutive_failures,
			diagnostics_complete,
			failure_category,
			device_reported_status,
			hardware_version,
			software_version,
			firmware_version,
			checksum
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + deviceStatusHistoryColumns

	history, err := scanDeviceStatusHistory(db.QueryRowContext(
		ctx,
		query,
		input.DeviceID,
		input.CheckedAt,
		input.ReachabilityStatus,
		input.ConsecutiveFailures,
		input.DiagnosticsComplete,
		input.FailureCategory,
		input.DeviceReportedStatus,
		input.HardwareVersion,
		input.SoftwareVersion,
		input.FirmwareVersion,
		input.Checksum,
	))
	if err != nil {
		return nil, fmt.Errorf("create device status history: %w", err)
	}

	return &history, nil
}

func (r *DeviceStatusHistoryRepository) DeleteBefore(
	ctx context.Context,
	cutoff time.Time,
	tx *sql.Tx,
) error {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	_, err := db.ExecContext(ctx, `
		DELETE FROM device_status_history WHERE checked_at < $1
	`, cutoff)
	if err != nil {
		return fmt.Errorf("delete device status history: %w", err)
	}

	return nil
}
