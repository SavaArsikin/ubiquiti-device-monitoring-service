package service_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/service"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
)

const schedulerTestTimeout = 2 * time.Second

type schedulerDeviceListerStub struct {
	mu sync.Mutex

	results [][]*device.Device
	err     error
	calls   int
}

func (deviceLister *schedulerDeviceListerStub) ListActive(
	_ context.Context,
) ([]*device.Device, error) {
	deviceLister.mu.Lock()
	defer deviceLister.mu.Unlock()

	deviceLister.calls++

	resultIndex := deviceLister.calls - 1
	if resultIndex >= len(deviceLister.results) {
		resultIndex = len(deviceLister.results) - 1
	}

	if resultIndex < 0 {
		return nil, deviceLister.err
	}

	return deviceLister.results[resultIndex], deviceLister.err
}

func (deviceLister *schedulerDeviceListerStub) callCount() int {
	deviceLister.mu.Lock()
	defer deviceLister.mu.Unlock()

	return deviceLister.calls
}

type schedulerDevicePollerStub struct {
	mu sync.Mutex

	calls              []string
	activePolls        int
	maximumActivePolls int

	// When enabled, each poll waits for a release signal or cancellation.
	blockUntilReleased bool
	release            chan struct{}
	started            chan string

	failedDeviceID string
	pollError      error
}

func (devicePoller *schedulerDevicePollerStub) PollDevice(
	requestContext context.Context,
	deviceID string,
) (*devicestatus.DeviceStatus, error) {
	devicePoller.mu.Lock()
	devicePoller.calls = append(devicePoller.calls, deviceID)
	devicePoller.activePolls++

	if devicePoller.activePolls > devicePoller.maximumActivePolls {
		devicePoller.maximumActivePolls = devicePoller.activePolls
	}
	devicePoller.mu.Unlock()

	if devicePoller.started != nil {
		devicePoller.started <- deviceID
	}

	var pollError error
	if devicePoller.blockUntilReleased {
		select {
		case <-devicePoller.release:
		case <-requestContext.Done():
			pollError = requestContext.Err()
		}
	}

	devicePoller.mu.Lock()
	devicePoller.activePolls--
	devicePoller.mu.Unlock()

	if pollError != nil {
		return nil, pollError
	}
	if deviceID == devicePoller.failedDeviceID {
		return nil, devicePoller.pollError
	}

	return &devicestatus.DeviceStatus{
		DeviceID:           deviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}, nil
}

func (devicePoller *schedulerDevicePollerStub) recordedDeviceIDs() []string {
	devicePoller.mu.Lock()
	defer devicePoller.mu.Unlock()

	return append([]string(nil), devicePoller.calls...)
}

func (devicePoller *schedulerDevicePollerStub) peakConcurrentPolls() int {
	devicePoller.mu.Lock()
	defer devicePoller.mu.Unlock()

	return devicePoller.maximumActivePolls
}

func initializeSchedulerTestConfig(
	t *testing.T,
	pollInterval time.Duration,
	maxConcurrentProbes int,
) {
	t.Helper()

	envFile := strings.Replace(
		testutils.ValidEnvFile,
		"POLL_INTERVAL=10s",
		fmt.Sprintf("POLL_INTERVAL=%s", pollInterval),
		1,
	)
	envFile = strings.Replace(
		envFile,
		"MAX_CONCURRENT_PROBES=10",
		fmt.Sprintf("MAX_CONCURRENT_PROBES=%d", maxConcurrentProbes),
		1,
	)

	testutils.InitConfig(t, envFile)
}

func newSchedulerTestDevice(deviceID string) *device.Device {
	return &device.Device{
		ID:        deviceID,
		Name:      "Test device " + deviceID,
		HealthURL: "http://localhost:8081/health",
	}
}

func receivePollStart(t *testing.T, startedDevices <-chan string) string {
	t.Helper()

	select {
	case deviceID := <-startedDevices:
		return deviceID
	case <-time.After(schedulerTestTimeout):
		t.Fatal("timed out waiting for a device poll to start")
		return ""
	}
}

func waitForSchedulerCompletion(
	t *testing.T,
	schedulerFinished <-chan error,
) error {
	t.Helper()

	select {
	case schedulerError := <-schedulerFinished:
		return schedulerError
	case <-time.After(schedulerTestTimeout):
		t.Fatal("timed out waiting for the scheduler to finish")
		return nil
	}
}

func TestMonitoringSchedulerLimitsConcurrentPolls(t *testing.T) {
	initializeSchedulerTestConfig(t, time.Second, 3)

	activeDevices := []*device.Device{
		newSchedulerTestDevice("device-1"),
		newSchedulerTestDevice("device-2"),
		newSchedulerTestDevice("device-3"),
		newSchedulerTestDevice("device-4"),
		newSchedulerTestDevice("device-5"),
		newSchedulerTestDevice("device-6"),
		newSchedulerTestDevice("device-7"),
	}

	deviceLister := &schedulerDeviceListerStub{
		results: [][]*device.Device{activeDevices},
	}
	devicePoller := &schedulerDevicePollerStub{
		blockUntilReleased: true,
		release:            make(chan struct{}),
		started:            make(chan string, len(activeDevices)),
	}

	monitoringScheduler := service.NewMonitoringScheduler(deviceLister, devicePoller)
	schedulerContext, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()

	schedulerFinished := make(chan error, 1)
	go func() {
		schedulerFinished <- monitoringScheduler.Run(schedulerContext)
	}()

	// The first three devices occupy all three workers.
	for startedCount := 0; startedCount < 3; startedCount++ {
		receivePollStart(t, devicePoller.started)
	}
	if got := devicePoller.peakConcurrentPolls(); got != 3 {
		t.Fatalf("expected 3 concurrent polls, got %d", got)
	}

	// Release one worker at a time. Each worker should pick up one queued device.
	for startedCount := 3; startedCount < len(activeDevices); startedCount++ {
		devicePoller.release <- struct{}{}
		receivePollStart(t, devicePoller.started)

		if got := devicePoller.peakConcurrentPolls(); got > 3 {
			t.Fatalf("worker limit exceeded: maximum concurrent polls = %d", got)
		}
	}

	// Release the final active workers, then cancel Run before it starts
	// another sweep. Run continues scheduling sweeps until its context ends.
	for workerNumber := 0; workerNumber < 3; workerNumber++ {
		devicePoller.release <- struct{}{}
	}
	cancelScheduler()

	if err := waitForSchedulerCompletion(t, schedulerFinished); err != nil {
		t.Fatalf("scheduler stopped with an error: %v", err)
	}

	recordedIDs := devicePoller.recordedDeviceIDs()
	sort.Strings(recordedIDs)

	expectedIDs := []string{
		"device-1",
		"device-2",
		"device-3",
		"device-4",
		"device-5",
		"device-6",
		"device-7",
	}
	sort.Strings(expectedIDs)

	if fmt.Sprint(recordedIDs) != fmt.Sprint(expectedIDs) {
		t.Errorf("polled device IDs mismatch: got %v, want %v", recordedIDs, expectedIDs)
	}
	if got := devicePoller.peakConcurrentPolls(); got != 3 {
		t.Errorf("expected peak concurrency of 3, got %d", got)
	}
}

func TestMonitoringSchedulerReloadsInventoryForEachSweep(t *testing.T) {
	initializeSchedulerTestConfig(t, 10*time.Millisecond, 2)

	deviceLister := &schedulerDeviceListerStub{
		results: [][]*device.Device{
			{newSchedulerTestDevice("first-sweep-device")},
			{newSchedulerTestDevice("second-sweep-device")},
		},
	}
	devicePoller := &schedulerDevicePollerStub{
		started: make(chan string, 2),
	}

	monitoringScheduler := service.NewMonitoringScheduler(deviceLister, devicePoller)
	schedulerContext, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()

	schedulerFinished := make(chan error, 1)
	go func() {
		schedulerFinished <- monitoringScheduler.Run(schedulerContext)
	}()

	firstPolledID := receivePollStart(t, devicePoller.started)
	if firstPolledID != "first-sweep-device" {
		t.Fatalf("first sweep polled %q, want first-sweep-device", firstPolledID)
	}

	secondPolledID := receivePollStart(t, devicePoller.started)
	if secondPolledID != "second-sweep-device" {
		t.Fatalf("second sweep polled %q, want second-sweep-device", secondPolledID)
	}

	cancelScheduler()

	if err := waitForSchedulerCompletion(t, schedulerFinished); err != nil {
		t.Fatalf("scheduler stopped with an error: %v", err)
	}
	if got := deviceLister.callCount(); got != 2 {
		t.Errorf("expected inventory to be loaded twice, got %d calls", got)
	}
}

func TestMonitoringSchedulerStopsWhenContextIsCancelled(t *testing.T) {
	initializeSchedulerTestConfig(t, time.Second, 1)

	deviceLister := &schedulerDeviceListerStub{
		results: [][]*device.Device{
			{newSchedulerTestDevice("blocked-device")},
		},
	}
	devicePoller := &schedulerDevicePollerStub{
		blockUntilReleased: true,
		started:            make(chan string, 1),
	}

	monitoringScheduler := service.NewMonitoringScheduler(deviceLister, devicePoller)
	schedulerContext, cancelScheduler := context.WithCancel(context.Background())

	schedulerFinished := make(chan error, 1)
	go func() {
		schedulerFinished <- monitoringScheduler.Run(schedulerContext)
	}()

	receivePollStart(t, devicePoller.started)
	cancelScheduler()

	if err := waitForSchedulerCompletion(t, schedulerFinished); err != nil {
		t.Fatalf("expected clean scheduler shutdown, got %v", err)
	}
	if got := deviceLister.callCount(); got != 1 {
		t.Errorf("expected one inventory read before cancellation, got %d", got)
	}
}

func TestMonitoringSchedulerContinuesAfterDevicePollError(t *testing.T) {
	initializeSchedulerTestConfig(t, time.Second, 1)

	activeDevices := []*device.Device{
		newSchedulerTestDevice("failing-device"),
		newSchedulerTestDevice("healthy-device"),
	}
	deviceLister := &schedulerDeviceListerStub{
		results: [][]*device.Device{activeDevices},
	}
	devicePoller := &schedulerDevicePollerStub{
		started:        make(chan string, len(activeDevices)),
		failedDeviceID: "failing-device",
		pollError:      errors.New("simulated device poll failure"),
	}

	monitoringScheduler := service.NewMonitoringScheduler(deviceLister, devicePoller)
	schedulerContext, cancelScheduler := context.WithCancel(context.Background())
	defer cancelScheduler()

	schedulerFinished := make(chan error, 1)
	go func() {
		schedulerFinished <- monitoringScheduler.Run(schedulerContext)
	}()

	// With one worker, the healthy device can only start if the worker
	// continues after the first device's error.
	firstPolledID := receivePollStart(t, devicePoller.started)
	secondPolledID := receivePollStart(t, devicePoller.started)

	cancelScheduler()

	if err := waitForSchedulerCompletion(t, schedulerFinished); err != nil {
		t.Fatalf("scheduler stopped with an error: %v", err)
	}
	if firstPolledID != "failing-device" || secondPolledID != "healthy-device" {
		t.Errorf(
			"expected failing device then healthy device, got %q then %q",
			firstPolledID,
			secondPolledID,
		)
	}
}
