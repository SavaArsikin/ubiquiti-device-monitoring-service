package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

var (
	ErrInvalidDeviceID    = errors.New("device ID must be a valid UUID")
	ErrDeviceRequired     = errors.New("device is required")
	ErrDeviceNameRequired = errors.New("device name is required")
	ErrInvalidHealthURL   = errors.New("health URL must be an HTTP or HTTPS URL with a host")
)

type deviceRepository interface {
	Create(context.Context, *device.Device, *sql.Tx) (*device.Device, error)
	Get(context.Context, string, *sql.Tx) (*device.Device, error)
	ListActive(context.Context, *sql.Tx) ([]*device.Device, error)
	ListArchived(context.Context, *sql.Tx) ([]*device.Device, error)
	Update(context.Context, *device.Device, *sql.Tx) (*device.Device, error)
	Archive(context.Context, string, *sql.Tx) error
	Restore(context.Context, string, *sql.Tx) (*device.Device, error)
}

type statusRepository interface {
	Create(
		context.Context,
		*devicestatus.DeviceStatus,
		*sql.Tx,
	) (*devicestatus.DeviceStatus, error)
}

type DeviceService struct {
	db       *sql.DB
	devices  deviceRepository
	statuses statusRepository
}

func NewDeviceService(
	db *sql.DB,
	deviceRepository deviceRepository,
	statusRepository statusRepository,
) *DeviceService {
	return &DeviceService{
		db:       db,
		devices:  deviceRepository,
		statuses: statusRepository,
	}
}

var deviceIDPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func validateDeviceID(deviceID string) error {
	if !deviceIDPattern.MatchString(deviceID) {
		return ErrInvalidDeviceID
	}

	return nil
}

func prepareDeviceInput(inputDevice *device.Device) (*device.Device, error) {
	if inputDevice == nil {
		return nil, ErrDeviceRequired
	}

	// Validate and normalize a copy so callers keep their original value.
	preparedDevice := *inputDevice
	preparedDevice.Name = strings.TrimSpace(preparedDevice.Name)
	preparedDevice.HealthURL = strings.TrimSpace(preparedDevice.HealthURL)

	if preparedDevice.Name == "" {
		return nil, ErrDeviceNameRequired
	}

	parsedHealthURL, err := url.Parse(preparedDevice.HealthURL)
	if err != nil ||
		parsedHealthURL.Hostname() == "" ||
		(parsedHealthURL.Scheme != "http" && parsedHealthURL.Scheme != "https") {
		return nil, ErrInvalidHealthURL
	}

	return &preparedDevice, nil
}

func (deviceService *DeviceService) Create(
	requestContext context.Context,
	inputDevice *device.Device,
) (createdDevice *device.Device, err error) {
	preparedDevice, err := prepareDeviceInput(inputDevice)
	if err != nil {
		return nil, err
	}

	transaction, err := deviceService.db.BeginTx(requestContext, nil)
	if err != nil {
		return nil, fmt.Errorf("begin device creation: %w", err)
	}

	// The inventory row and its initial status must be created together.
	// If either insert fails, neither should be left in the database.
	defer func() {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil &&
			!errors.Is(rollbackErr, sql.ErrTxDone) {
			createdDevice = nil
			err = errors.Join(
				err,
				fmt.Errorf("rollback device creation: %w", rollbackErr),
			)
		}
	}()

	createdDevice, err = deviceService.devices.Create(
		requestContext,
		preparedDevice,
		transaction,
	)
	if err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}

	initialDeviceStatus := devicestatus.DeviceStatus{
		DeviceID:           createdDevice.ID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}

	if _, err = deviceService.statuses.Create(
		requestContext,
		&initialDeviceStatus,
		transaction,
	); err != nil {
		return nil, fmt.Errorf("create initial device status: %w", err)
	}

	if err = transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit device creation: %w", err)
	}

	return createdDevice, nil
}

func (deviceService *DeviceService) Get(
	requestContext context.Context,
	deviceID string,
) (*device.Device, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	foundDevice, err := deviceService.devices.Get(requestContext, deviceID, nil)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	return foundDevice, nil
}

func (deviceService *DeviceService) ListActive(
	requestContext context.Context,
) ([]*device.Device, error) {
	activeDevices, err := deviceService.devices.ListActive(requestContext, nil)
	if err != nil {
		return nil, fmt.Errorf("list active devices: %w", err)
	}

	return activeDevices, nil
}

func (deviceService *DeviceService) ListArchived(
	requestContext context.Context,
) ([]*device.Device, error) {
	archivedDevices, err := deviceService.devices.ListArchived(requestContext, nil)
	if err != nil {
		return nil, fmt.Errorf("list archived devices: %w", err)
	}

	return archivedDevices, nil
}

func (deviceService *DeviceService) Update(
	requestContext context.Context,
	inputDevice *device.Device,
) (*device.Device, error) {
	preparedDevice, err := prepareDeviceInput(inputDevice)
	if err != nil {
		return nil, err
	}

	if err := validateDeviceID(preparedDevice.ID); err != nil {
		return nil, err
	}

	updatedDevice, err := deviceService.devices.Update(
		requestContext,
		preparedDevice,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("update device: %w", err)
	}

	return updatedDevice, nil
}

func (deviceService *DeviceService) Archive(
	requestContext context.Context,
	deviceID string,
) error {
	if err := validateDeviceID(deviceID); err != nil {
		return err
	}

	if err := deviceService.devices.Archive(requestContext, deviceID, nil); err != nil {
		return fmt.Errorf("archive device: %w", err)
	}

	return nil
}

func (deviceService *DeviceService) Restore(
	requestContext context.Context,
	deviceID string,
) (*device.Device, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	restoredDevice, err := deviceService.devices.Restore(
		requestContext,
		deviceID,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("restore device: %w", err)
	}

	return restoredDevice, nil
}
