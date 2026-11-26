package device

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type DeviceRepository struct {
	db *sql.DB
}

const deviceColumns = `
	id::text, name, health_url, created_at, updated_at, archived_at
`

// rowScanner abstracts sql row scanning so scanDevice can use it for rows and row
// both QueryRowContext and QueryContext rows implement Scan function
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDevice(row rowScanner) (Device, error) {
	var device Device
	var archivedAt sql.NullTime

	err := row.Scan(
		&device.ID,
		&device.Name,
		&device.HealthURL,
		&device.CreatedAt,
		&device.UpdatedAt,
		&archivedAt,
	)
	if err != nil {
		return Device{}, err
	}
	if archivedAt.Valid {
		device.ArchivedAt = &archivedAt.Time
	}
	return device, nil
}

// Both *sql.DB and *sql.Tx implement these methods.
type queryExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) Create(
	ctx context.Context,
	inputDevice *Device,
	tx *sql.Tx,
) (*Device, error) {
	if inputDevice == nil {
		return nil, errors.New("input device is nil")
	}

	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		INSERT INTO devices (id, name, health_url)
		VALUES (gen_random_uuid(), $1, $2)
		RETURNING ` + deviceColumns

	device, err := scanDevice(db.QueryRowContext(ctx, query, inputDevice.Name, inputDevice.HealthURL))
	if err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}
	return &device, nil
}

func (r *DeviceRepository) Get(ctx context.Context, id string, tx *sql.Tx) (*Device, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `SELECT ` + deviceColumns + ` FROM devices WHERE id = $1`
	if tx != nil {
		query += ` FOR UPDATE`
	}

	device, err := scanDevice(db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	return &device, nil
}

func (r *DeviceRepository) ListActive(ctx context.Context, tx *sql.Tx) ([]*Device, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		SELECT ` + deviceColumns + `
		FROM devices
		WHERE archived_at IS NULL
		ORDER BY updated_at, id
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	devices := make([]*Device, 0)
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, &device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate devices: %w", err)
	}
	return devices, nil
}

func (r *DeviceRepository) ListArchived(
	ctx context.Context,
	tx *sql.Tx,
) ([]*Device, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		SELECT ` + deviceColumns + `
		FROM devices
		WHERE archived_at IS NOT NULL
		ORDER BY updated_at, id
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list archived devices: %w", err)
	}
	defer rows.Close()

	devices := make([]*Device, 0)
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived device: %w", err)
		}
		devices = append(devices, &device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived devices: %w", err)
	}

	return devices, nil
}

func (r *DeviceRepository) Update(
	ctx context.Context,
	inputDevice *Device,
	tx *sql.Tx,
) (*Device, error) {
	if inputDevice == nil {
		return nil, errors.New("input device is nil")
	}

	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		UPDATE devices
		SET name = $2, health_url = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND archived_at IS NULL
		RETURNING ` + deviceColumns

	device, err := scanDevice(db.QueryRowContext(ctx, query, inputDevice.ID, inputDevice.Name, inputDevice.HealthURL))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update device: %w", err)
	}
	return &device, nil
}

func (r *DeviceRepository) Archive(ctx context.Context, id string, tx *sql.Tx) error {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	result, err := db.ExecContext(ctx, `
		UPDATE devices
		SET archived_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND archived_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("archive device: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check archived device: %w", err)
	}
	if rowsAffected == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

func (r *DeviceRepository) Restore(ctx context.Context, id string, tx *sql.Tx) (*Device, error) {
	var db queryExecutor = r.db
	if tx != nil {
		db = tx
	}

	query := `
		UPDATE devices
		SET archived_at = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND archived_at IS NOT NULL
		RETURNING ` + deviceColumns

	device, err := scanDevice(db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("restore device: %w", err)
	}
	return &device, nil
}
