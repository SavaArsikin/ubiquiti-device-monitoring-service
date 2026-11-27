package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/probe"
)

var (
	ErrDeviceArchived = errors.New("device is archived")
	ErrDeviceChanged  = errors.New("device changed while being polled")
	ErrStalePoll      = errors.New("a newer poll result is already stored")
)

type monitoringDeviceRepository interface {
	Get(context.Context, string, *sql.Tx) (*device.Device, error)
}

type monitoringStatusRepository interface {
	Get(context.Context, string, *sql.Tx) (*devicestatus.DeviceStatus, error)
	Update(
		context.Context,
		*devicestatus.DeviceStatus,
		*sql.Tx,
	) (*devicestatus.DeviceStatus, error)
	ListActive(
		context.Context,
		*sql.Tx,
	) ([]*devicestatus.DeviceStatus, error)
}

type monitoringHistoryRepository interface {
	Create(
		context.Context,
		*devicestatus.DeviceStatusHistory,
		*sql.Tx,
	) (*devicestatus.DeviceStatusHistory, error)
}

type diagnosticsClient interface {
	Discover(
		context.Context,
		string,
	) (*deviceprotocol.Capabilities, error)

	FetchDiagnostics(
		context.Context,
		string,
		string,
	) (*deviceprotocol.Diagnostics, error)

	FetchGRPCDiagnostics(
		context.Context,
		string,
		string,
	) (*deviceprotocol.Diagnostics, error)
}

type MonitoringService struct {
	db       *sql.DB
	devices  monitoringDeviceRepository
	statuses monitoringStatusRepository
	history  monitoringHistoryRepository
	client   diagnosticsClient
}

func NewMonitoringService(
	db *sql.DB,
	devices monitoringDeviceRepository,
	statuses monitoringStatusRepository,
	history monitoringHistoryRepository,
	client diagnosticsClient,
) *MonitoringService {
	return &MonitoringService{
		db:       db,
		devices:  devices,
		statuses: statuses,
		history:  history,
		client:   client,
	}
}

type pollResult struct {
	reachable   bool
	complete    bool
	category    string
	diagnostics *deviceprotocol.Diagnostics
	reachableAt time.Time
	observedAt  time.Time
}

func (monitoringService *MonitoringService) PollDevice(
	ctx context.Context,
	deviceID string,
) (savedStatus *devicestatus.DeviceStatus, err error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	deviceBeforePolling, err := monitoringService.devices.Get(ctx, deviceID, nil)
	if err != nil {
		return nil, fmt.Errorf("read device before polling: %w", err)
	}
	if deviceBeforePolling.ArchivedAt != nil {
		return nil, ErrDeviceArchived
	}

	pollStartedAt := time.Now().UTC().Truncate(time.Microsecond)

	result, err := monitoringService.probeDeviceWithRetries(ctx, deviceBeforePolling)
	if err != nil {
		return nil, err
	}

	pollCompletedAt := time.Now().UTC().Truncate(time.Microsecond)

	// Keep network calls outside this transaction so an unresponsive device
	// does not hold database locks while retries are in progress.
	transaction, err := monitoringService.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin poll persistence transaction: %w", err)
	}

	defer func() {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil &&
			!errors.Is(rollbackErr, sql.ErrTxDone) {
			savedStatus = nil
			err = errors.Join(err,
				fmt.Errorf("rollback poll persistence transaction: %w", rollbackErr))
		}
	}()

	// Lock and recheck inventory before status. This avoids saving a result
	// for a device that was archived or edited while it was being probed.
	deviceAfterPolling, err := monitoringService.devices.Get(ctx, deviceID, transaction)
	if err != nil {
		return nil, fmt.Errorf("recheck device after polling: %w", err)
	}
	if deviceAfterPolling.ArchivedAt != nil {
		return nil, ErrDeviceArchived
	}
	if deviceAfterPolling.HealthURL != deviceBeforePolling.HealthURL ||
		!deviceAfterPolling.UpdatedAt.Equal(deviceBeforePolling.UpdatedAt) {
		return nil, ErrDeviceChanged
	}

	currentStatus, err := monitoringService.statuses.Get(ctx, deviceID, transaction)
	if err != nil {
		return nil, fmt.Errorf("read current device status: %w", err)
	}

	// Concurrent polls can finish out of order. Do not let an older result
	// overwrite a result from a poll that started later.
	if currentStatus.LastCheckedAt != nil &&
		!currentStatus.LastCheckedAt.Before(pollStartedAt) {
		return nil, ErrStalePoll
	}

	nextStatus, historyEntry := buildStatusSnapshotAndHistory(
		currentStatus,
		result,
		pollCompletedAt,
	)

	savedStatus, err = monitoringService.statuses.Update(
		ctx,
		&nextStatus,
		transaction,
	)
	if err != nil {
		return nil, fmt.Errorf("update device status: %w", err)
	}

	if _, err := monitoringService.history.Create(ctx, &historyEntry, transaction); err != nil {
		return nil, fmt.Errorf("create device status history: %w", err)
	}

	// Snapshot and history describe the same poll cycle, so they must commit
	// together or both be rolled back.
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit poll persistence transaction: %w", err)
	}

	if currentStatus.ReachabilityStatus != savedStatus.ReachabilityStatus {
		slog.Info("device reachability changed",
			"device_id", deviceID,
			"previous", currentStatus.ReachabilityStatus,
			"current", savedStatus.ReachabilityStatus,
		)
	}
	if result.category != "" {
		slog.Warn("device poll incomplete",
			"device_id", deviceID,
			"category", result.category,
			"consecutive_failures", savedStatus.ConsecutiveFailures,
		)
	}

	return savedStatus, nil
}

func (monitoringService *MonitoringService) probeDeviceWithRetries(
	ctx context.Context,
	monitoredDevice *device.Device,
) (pollResult, error) {
	applicationConfig := config.Get()
	var bestResult pollResult

	for attemptNumber := 0; attemptNumber <= applicationConfig.MaxRetries; attemptNumber++ {
		if err := ctx.Err(); err != nil {
			return pollResult{}, err
		}

		if attemptNumber > 0 {
			slog.Warn("retrying device probe",
				"device_id", monitoredDevice.ID,
				"attempt", attemptNumber+1,
				"category", bestResult.category,
			)

			if err := waitForRetry(ctx, applicationConfig.RetryDelay); err != nil {
				return pollResult{}, err
			}
		}

		attemptContext, cancelAttempt := context.WithTimeout(
			ctx,
			applicationConfig.ProbeTimeout,
		)
		attemptResult := monitoringService.probeDeviceOnce(
			attemptContext,
			monitoredDevice.HealthURL,
		)
		cancelAttempt()

		// Cancellation of the whole monitoring operation is not evidence that
		// the device is down, so do not return it as a poll failure.
		if err := ctx.Err(); err != nil {
			return pollResult{}, err
		}

		// Preserve a successful contact result if a later retry fails during
		// discovery; a transient retry failure should not create a false alarm.
		if attemptResult.reachable || !bestResult.reachable {
			bestResult = attemptResult
		}
		if attemptResult.complete {
			return attemptResult, nil
		}
	}

	return bestResult, nil
}

func (monitoringService *MonitoringService) probeDeviceOnce(
	ctx context.Context,
	healthURL string,
) pollResult {
	capabilities, err := monitoringService.client.Discover(ctx, healthURL)
	if err != nil {
		return pollResult{
			category: probeFailureCategory(err, "discovery_failed"),
		}
	}

	result := pollResult{
		reachable:   true,
		reachableAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	// Do not silently use REST when the device advertises gRPC; that would
	// hide the fact that the advertised protocol is not implemented yet.
	var diagnostics *deviceprotocol.Diagnostics

	switch {
	case capabilities.GRPCTarget != "":
		// Prefer gRPC when both transports are advertised.
		diagnostics, err = monitoringService.client.FetchGRPCDiagnostics(
			ctx,
			healthURL,
			capabilities.GRPCTarget,
		)

	case capabilities.RESTDiagnosticsPath != "":
		diagnostics, err = monitoringService.client.FetchDiagnostics(
			ctx,
			healthURL,
			capabilities.RESTDiagnosticsPath,
		)

	default:
		result.category = "unsupported_protocol"
		return result
	}

	result.diagnostics = diagnostics

	if diagnostics != nil {
		result.observedAt = time.Now().UTC().Truncate(time.Microsecond)
	}

	if err != nil {
		if errors.Is(err, probe.ErrInvalidDiagnostics) {
			result.category = "invalid_diagnostics"
		} else {
			result.category = probeFailureCategory(err, "diagnostics_failed")
		}
		return result
	}

	if diagnostics.Checksum == nil ||
		strings.TrimSpace(*diagnostics.Checksum) == "" {
		result.category = "checksum_unavailable"
		return result
	}

	result.complete = true
	return result
}

func buildStatusSnapshotAndHistory(
	currentStatus *devicestatus.DeviceStatus,
	result pollResult,
	pollCompletedAt time.Time,
) (devicestatus.DeviceStatus, devicestatus.DeviceStatusHistory) {
	applicationConfig := config.Get()
	nextStatus := *currentStatus
	nextStatus.LastCheckedAt = &pollCompletedAt
	nextStatus.DiagnosticsComplete = result.complete
	nextStatus.LastFailureCategory = stringPointerIfNotEmpty(result.category)

	if result.reachable {
		nextStatus.ReachabilityStatus = devicestatus.ReachabilityReachable
		nextStatus.ConsecutiveFailures = 0
		nextStatus.LastReachableAt = &result.reachableAt
	} else {
		nextStatus.ConsecutiveFailures++
		nextStatus.ReachabilityStatus = devicestatus.ReachabilityDegraded

		if nextStatus.ConsecutiveFailures >= applicationConfig.FailureThreshold {
			nextStatus.ReachabilityStatus = devicestatus.ReachabilityUnreachable
		}
	}

	historyEntry := devicestatus.DeviceStatusHistory{
		DeviceID:            currentStatus.DeviceID,
		CheckedAt:           pollCompletedAt,
		ReachabilityStatus:  nextStatus.ReachabilityStatus,
		ConsecutiveFailures: nextStatus.ConsecutiveFailures,
		DiagnosticsComplete: result.complete,
		FailureCategory:     stringPointerIfNotEmpty(result.category),
	}

	if result.diagnostics != nil {
		diagnostics := result.diagnostics

		historyEntry.DeviceReportedStatus = stringPointerIfNotEmpty(diagnostics.Status)
		historyEntry.HardwareVersion = stringPointerIfNotEmpty(diagnostics.HardwareVersion)
		historyEntry.SoftwareVersion = stringPointerIfNotEmpty(diagnostics.SoftwareVersion)
		historyEntry.FirmwareVersion = stringPointerIfNotEmpty(diagnostics.FirmwareVersion)
		historyEntry.Checksum = diagnostics.Checksum

		// Incomplete results belong in history, but must not replace the
		// snapshot's last fully validated diagnostics.
		if result.complete {
			nextStatus.DeviceReportedStatus = historyEntry.DeviceReportedStatus
			nextStatus.HardwareVersion = historyEntry.HardwareVersion
			nextStatus.SoftwareVersion = historyEntry.SoftwareVersion
			nextStatus.FirmwareVersion = historyEntry.FirmwareVersion
			nextStatus.Checksum = historyEntry.Checksum
			nextStatus.DiagnosticsObservedAt = &result.observedAt
		}
	}

	return nextStatus, historyEntry
}

func stringPointerIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func probeFailureCategory(err error, fallbackCategory string) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}

	return fallbackCategory
}

func waitForRetry(ctx context.Context, retryDelay time.Duration) error {
	retryTimer := time.NewTimer(retryDelay)
	defer retryTimer.Stop()

	select {
	case <-retryTimer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ListStatuses returns the latest status snapshot for every active device.
// Archived devices are excluded because they are no longer being monitored.
func (monitoringService *MonitoringService) ListStatuses(
	requestContext context.Context,
) ([]*devicestatus.DeviceStatus, error) {
	deviceStatuses, err := monitoringService.statuses.ListActive(
		requestContext,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("list active device statuses: %w", err)
	}

	return deviceStatuses, nil
}

// GetStatus returns the latest status snapshot for the requested device.
func (monitoringService *MonitoringService) GetStatus(
	requestContext context.Context,
	deviceID string,
) (*devicestatus.DeviceStatus, error) {
	if err := validateDeviceID(deviceID); err != nil {
		return nil, err
	}

	deviceStatus, err := monitoringService.statuses.Get(
		requestContext,
		deviceID,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("get device status: %w", err)
	}

	return deviceStatus, nil
}
