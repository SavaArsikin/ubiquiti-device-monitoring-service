package devicestatus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type DeviceStatusRepository struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// Both *sql.DB and *sql.Tx implement this method.
type queryExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

const deviceStatusColumns = `
	device_id::text,
	reachability_status,
	consecutive_failures,
	last_checked_at,
	last_reachable_at,
	last_failure_category,
	device_reported_status,
	diagnostics_complete,
	hardware_version,
	software_version,
	firmware_version,
	checksum,
	diagnostics_observed_at
`

func NewDeviceStatusRepository(db *sql.DB) *DeviceStatusRepository {
	return &DeviceStatusRepository{db: db}
}

func scanDeviceStatus(row rowScanner) (DeviceStatus, error) {
	var status DeviceStatus

	err := row.Scan(
		&status.DeviceID,
		&status.ReachabilityStatus,
		&status.ConsecutiveFailures,
		&status.LastCheckedAt,
		&status.LastReachableAt,
		&status.LastFailureCategory,
		&status.DeviceReportedStatus,
		&status.DiagnosticsComplete,
		&status.HardwareVersion,
		&status.SoftwareVersion,
		&status.FirmwareVersion,
		&status.Checksum,
		&status.DiagnosticsObservedAt,
	)
	if err != nil {
		return DeviceStatus{}, err
	}

	return status, nil
}

func (r *DeviceStatusRepository) Create(
	ctx context.Context,
	input *DeviceStatus,
	tx *sql.Tx,
) (*DeviceStatus, error) {
	if input == nil {
		return nil, errors.New("input device status is nil")
	}

	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		INSERT INTO device_statuses (
			device_id,
			reachability_status,
			consecutive_failures,
			last_checked_at,
			last_reachable_at,
			last_failure_category,
			device_reported_status,
			diagnostics_complete,
			hardware_version,
			software_version,
			firmware_version,
			checksum,
			diagnostics_observed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING ` + deviceStatusColumns

	created, err := scanDeviceStatus(db.QueryRowContext(
		ctx,
		query,
		input.DeviceID,
		input.ReachabilityStatus,
		input.ConsecutiveFailures,
		input.LastCheckedAt,
		input.LastReachableAt,
		input.LastFailureCategory,
		input.DeviceReportedStatus,
		input.DiagnosticsComplete,
		input.HardwareVersion,
		input.SoftwareVersion,
		input.FirmwareVersion,
		input.Checksum,
		input.DiagnosticsObservedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("create device status: %w", err)
	}

	return &created, nil
}

func (r *DeviceStatusRepository) Get(
	ctx context.Context,
	deviceID string,
	tx *sql.Tx,
) (*DeviceStatus, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `SELECT ` + deviceStatusColumns + `
		FROM device_statuses WHERE device_id = $1`
	if tx != nil {
		query += ` FOR UPDATE`
	}

	status, err := scanDeviceStatus(db.QueryRowContext(ctx, query, deviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceStatusNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get device status: %w", err)
	}

	return &status, nil
}

func (r *DeviceStatusRepository) ListActive(
	ctx context.Context,
	tx *sql.Tx,
) ([]*DeviceStatus, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		SELECT ` + deviceStatusColumns + `
		FROM device_statuses
		WHERE EXISTS (
			SELECT 1 FROM devices
			WHERE devices.id = device_statuses.device_id
			  AND devices.archived_at IS NULL
		)
		ORDER BY device_id
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list device statuses: %w", err)
	}
	defer rows.Close()

	statuses := make([]*DeviceStatus, 0)
	for rows.Next() {
		status, err := scanDeviceStatus(rows)
		if err != nil {
			return nil, fmt.Errorf("scan device status: %w", err)
		}
		statuses = append(statuses, &status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device statuses: %w", err)
	}

	return statuses, nil
}

func (r *DeviceStatusRepository) Update(
	ctx context.Context,
	input *DeviceStatus,
	tx *sql.Tx,
) (*DeviceStatus, error) {
	if input == nil {
		return nil, errors.New("input device status is nil")
	}

	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		UPDATE device_statuses
		SET reachability_status = $2,
		    consecutive_failures = $3,
		    last_checked_at = $4,
		    last_reachable_at = $5,
		    last_failure_category = $6,
		    device_reported_status = $7,
		    diagnostics_complete = $8,
		    hardware_version = $9,
		    software_version = $10,
		    firmware_version = $11,
		    checksum = $12,
		    diagnostics_observed_at = $13
		WHERE device_id = $1
		RETURNING ` + deviceStatusColumns

	status, err := scanDeviceStatus(db.QueryRowContext(
		ctx,
		query,
		input.DeviceID,
		input.ReachabilityStatus,
		input.ConsecutiveFailures,
		input.LastCheckedAt,
		input.LastReachableAt,
		input.LastFailureCategory,
		input.DeviceReportedStatus,
		input.DiagnosticsComplete,
		input.HardwareVersion,
		input.SoftwareVersion,
		input.FirmwareVersion,
		input.Checksum,
		input.DiagnosticsObservedAt,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceStatusNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update device status: %w", err)
	}

	return &status, nil
}
