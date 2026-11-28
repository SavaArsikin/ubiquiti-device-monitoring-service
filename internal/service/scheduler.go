package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

type activeDeviceLister interface {
	ListActive(context.Context) ([]*device.Device, error)
}

type devicePoller interface {
	PollDevice(
		context.Context,
		string,
	) (*devicestatus.DeviceStatus, error)
}

type MonitoringScheduler struct {
	devices           activeDeviceLister
	monitoringService devicePoller
}

func NewMonitoringScheduler(
	deviceLister activeDeviceLister,
	monitoringService devicePoller,
) *MonitoringScheduler {
	return &MonitoringScheduler{
		devices:           deviceLister,
		monitoringService: monitoringService,
	}
}

// Run starts a sweep immediately, then waits one polling interval after each
// completed sweep. It returns nil when the context is cancelled for shutdown.
func (scheduler *MonitoringScheduler) Run(ctx context.Context) error {
	applicationConfig := config.Get()

	for {
		if ctx.Err() != nil {
			return nil
		}

		// Inventory is read again for every sweep so additions, edits, and
		// archives are reflected without restarting the service.
		if err := scheduler.pollActiveDevices(ctx, applicationConfig.MaxConcurrentProbes); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			// A temporary inventory/database error should delay this sweep,
			// not permanently stop future polling.
			slog.Error("could not load active devices for polling", "error", err)
		}

		if ctx.Err() != nil {
			return nil
		}

		// Wait for next poll interval or shutdown.
		pollingTimer := time.NewTimer(applicationConfig.PollInterval)
		select {
		case <-pollingTimer.C:
			// The next sweep begins after the configured delay.
		case <-ctx.Done():
			pollingTimer.Stop()
			return nil
		}
	}
}

func (scheduler *MonitoringScheduler) pollActiveDevices(
	ctx context.Context,
	maxConcurrentProbes int,
) error {
	activeDevices, err := scheduler.devices.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active devices: %w", err)
	}

	if len(activeDevices) == 0 {
		slog.Debug("no active devices to poll")
		return nil
	}

	slog.Info("polling active devices", "count", len(activeDevices))

	workerCount := maxConcurrentProbes
	if workerCount > len(activeDevices) {
		workerCount = len(activeDevices)
	}

	// Workers consume devices from this queue. Devices beyond the worker
	// limit wait here rather than being dropped or polled concurrently.
	deviceQueue := make(chan *device.Device)
	var workerGroup sync.WaitGroup

	for workerNumber := 0; workerNumber < workerCount; workerNumber++ {
		workerGroup.Add(1)

		go func() {
			defer workerGroup.Done()
			scheduler.runPollDeviceWorker(ctx, deviceQueue)
		}()
	}

	for _, monitoredDevice := range activeDevices {
		select {
		case deviceQueue <- monitoredDevice:
			// A worker accepted this device; continue feeding the queue.
		case <-ctx.Done():
			// Stop assigning work, then wait for in-flight polls to observe
			// cancellation and finish before returning.
			close(deviceQueue)
			workerGroup.Wait()
			return ctx.Err()
		}
	}
	// All devices assigned, close queue
	close(deviceQueue)
	workerGroup.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

func (scheduler *MonitoringScheduler) runPollDeviceWorker(
	ctx context.Context,
	deviceQueue <-chan *device.Device,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case monitoredDevice, queueOpen := <-deviceQueue:
			// Check if queue closed
			if !queueOpen {
				return
			}

			// Cancellation and queued work can become ready together.
			if ctx.Err() != nil {
				return
			}

			if monitoredDevice == nil {
				slog.Error("active device list contains a nil device")
				continue
			}

			_, err := scheduler.monitoringService.PollDevice(
				ctx,
				monitoredDevice.ID,
			)
			if err != nil {
				// Cancellation is expected during shutdown, not a
				// device failure to report as an error.
				if ctx.Err() != nil {
					return
				}

				// One device's problem must not prevent other devices
				// in this sweep from being checked. So just log.
				slog.Error(
					"device poll failed",
					"device_id", monitoredDevice.ID,
					"error", err,
				)
			}
		}
	}
}
