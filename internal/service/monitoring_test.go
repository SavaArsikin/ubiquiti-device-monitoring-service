package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/service"
)

type monitoringStatusRepositoryStub struct {
	getResult    *devicestatus.DeviceStatus
	getErr       error
	updateResult *devicestatus.DeviceStatus
	updateErr    error

	getCalls    int
	updateCalls int
	getID       string
	getTx       *sql.Tx
	updateTx    *sql.Tx
	updateInput *devicestatus.DeviceStatus

	listResult  []*devicestatus.DeviceStatus
	listErr     error
	listCalls   int
	listTx      *sql.Tx
	listContext context.Context
}

func (repositoryStub *monitoringStatusRepositoryStub) Get(
	_ context.Context,
	deviceID string,
	transaction *sql.Tx,
) (*devicestatus.DeviceStatus, error) {
	repositoryStub.getCalls++
	repositoryStub.getID = deviceID
	repositoryStub.getTx = transaction

	return repositoryStub.getResult, repositoryStub.getErr
}

func (repositoryStub *monitoringStatusRepositoryStub) Update(
	_ context.Context,
	inputStatus *devicestatus.DeviceStatus,
	transaction *sql.Tx,
) (*devicestatus.DeviceStatus, error) {
	repositoryStub.updateCalls++
	repositoryStub.updateTx = transaction

	if inputStatus != nil {
		copiedStatus := *inputStatus
		repositoryStub.updateInput = &copiedStatus
	}

	return repositoryStub.updateResult, repositoryStub.updateErr
}

func (repositoryStub *monitoringStatusRepositoryStub) ListActive(
	requestContext context.Context,
	transaction *sql.Tx,
) ([]*devicestatus.DeviceStatus, error) {
	repositoryStub.listCalls++
	repositoryStub.listContext = requestContext
	repositoryStub.listTx = transaction

	return repositoryStub.listResult, repositoryStub.listErr
}

type monitoringHistoryRepositoryStub struct {
	result      *devicestatus.DeviceStatusHistory
	err         error
	calls       int
	createInput *devicestatus.DeviceStatusHistory
	createTx    *sql.Tx
}

func (repositoryStub *monitoringHistoryRepositoryStub) Create(
	_ context.Context,
	inputHistory *devicestatus.DeviceStatusHistory,
	transaction *sql.Tx,
) (*devicestatus.DeviceStatusHistory, error) {
	repositoryStub.calls++
	repositoryStub.createTx = transaction

	if inputHistory != nil {
		copiedHistory := *inputHistory
		repositoryStub.createInput = &copiedHistory
	}

	return repositoryStub.result, repositoryStub.err
}

type monitoringDiagnosticsClientStub struct {
	capabilities   *deviceprotocol.Capabilities
	discoverErr    error
	diagnostics    *deviceprotocol.Diagnostics
	diagnosticsErr error

	discoverCalls    int
	diagnosticsCalls int

	grpcDiagnostics      *deviceprotocol.Diagnostics
	grpcDiagnosticsErr   error
	grpcDiagnosticsCalls int
	grpcTarget           string
}

func (clientStub *monitoringDiagnosticsClientStub) Discover(
	_ context.Context,
	_ string,
) (*deviceprotocol.Capabilities, error) {
	clientStub.discoverCalls++
	return clientStub.capabilities, clientStub.discoverErr
}

func (clientStub *monitoringDiagnosticsClientStub) FetchDiagnostics(
	_ context.Context,
	_ string,
	_ string,
) (*deviceprotocol.Diagnostics, error) {
	clientStub.diagnosticsCalls++
	return clientStub.diagnostics, clientStub.diagnosticsErr
}

func (clientStub *monitoringDiagnosticsClientStub) FetchGRPCDiagnostics(
	_ context.Context,
	_ string,
	grpcTarget string,
) (*deviceprotocol.Diagnostics, error) {
	clientStub.grpcDiagnosticsCalls++
	clientStub.grpcTarget = grpcTarget

	return clientStub.grpcDiagnostics, clientStub.grpcDiagnosticsErr
}

func newMonitoringServiceTestFixture(
	t *testing.T,
	maxRetries int,
	failureThreshold int,
	initialStatus *devicestatus.DeviceStatus,
	diagnosticsClient *monitoringDiagnosticsClientStub,
) (
	*sql.DB,
	sqlmock.Sqlmock,
	*service.MonitoringService,
	*monitoringStatusRepositoryStub,
	*monitoringHistoryRepositoryStub,
) {
	t.Helper()

	initializeMonitoringTestConfig(t, maxRetries, failureThreshold)

	db, databaseMock := openMockDatabase(t)

	monitoredDevice := &device.Device{
		ID:        testDeviceID,
		Name:      "Test router",
		HealthURL: "http://localhost:8081/health",
	}

	deviceRepository := &deviceRepositoryStub{
		result: monitoredDevice,
	}
	statusRepository := &monitoringStatusRepositoryStub{
		getResult: initialStatus,
	}
	historyRepository := &monitoringHistoryRepositoryStub{}

	monitoringService := service.NewMonitoringService(
		db,
		deviceRepository,
		statusRepository,
		historyRepository,
		diagnosticsClient,
	)

	return db, databaseMock, monitoringService, statusRepository, historyRepository
}

func initializeMonitoringTestConfig(
	t *testing.T,
	maxRetries int,
	failureThreshold int,
) {
	t.Helper()

	configPath := t.TempDir() + "/monitor.env"
	configContents := fmt.Sprintf(`LOG_FORMAT=local
	LOG_LEVEL=info
	DATABASE_URL=postgres://test:test@localhost:5432/test
	DB_MAX_OPEN_CONNS=5
	DB_MAX_IDLE_CONNS=2
	DB_CONN_MAX_LIFETIME=1h
	DB_CONNECT_TIMEOUT=5s
	DB_MIGRATION_TIMEOUT=5s
	HTTP_ADDR=127.0.0.1:8080
	API_USERNAME=test
	API_PASSWORD=test
	POLL_INTERVAL=10s
	PROBE_TIMEOUT=1s
	RETRY_DELAY=1ms
	MAX_RETRIES=%d
	FAILURE_THRESHOLD=%d
	MAX_CONCURRENT_PROBES=2
	SHUTDOWN_TIMEOUT=10s
	`, maxRetries, failureThreshold)

	if err := os.WriteFile(configPath, []byte(configContents), 0600); err != nil {
		t.Fatalf("write monitoring test config: %v", err)
	}

	if _, err := config.Init(configPath); err != nil {
		t.Fatalf("initialize monitoring test config: %v", err)
	}
}

func TestPollDeviceSuccess(t *testing.T) {
	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}
	checksum := "checksum-value"

	diagnosticsClient := &monitoringDiagnosticsClientStub{
		capabilities: &deviceprotocol.Capabilities{
			RESTDiagnosticsPath: "/diagnostics",
		},
		diagnostics: &deviceprotocol.Diagnostics{
			HardwareVersion: "hardware-1",
			SoftwareVersion: "software-1",
			FirmwareVersion: "firmware-1",
			Status:          "healthy",
			Checksum:        &checksum,
		},
	}

	_, databaseMock, monitoringService, statusRepository, historyRepository :=
		newMonitoringServiceTestFixture(t, 0, 3, initialStatus, diagnosticsClient)

	databaseMock.ExpectBegin()
	databaseMock.ExpectCommit()

	statusRepository.updateResult = &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}

	savedStatus, err := monitoringService.PollDevice(context.Background(), testDeviceID)
	if err != nil {
		t.Fatalf("poll device: %v", err)
	}

	if savedStatus.ReachabilityStatus != devicestatus.ReachabilityReachable {
		t.Errorf("expected reachable status, got %q", savedStatus.ReachabilityStatus)
	}
	if statusRepository.getCalls != 1 || statusRepository.updateCalls != 1 {
		t.Fatalf("expected one status read and update, got reads=%d updates=%d",
			statusRepository.getCalls, statusRepository.updateCalls)
	}
	if statusRepository.getTx == nil ||
		statusRepository.updateTx != statusRepository.getTx ||
		historyRepository.createTx != statusRepository.getTx {
		t.Error("expected status and history repositories to share the transaction")
	}

	updatedStatus := statusRepository.updateInput
	if updatedStatus == nil {
		t.Fatal("expected an updated status to be sent to the repository")
	}
	if updatedStatus.ReachabilityStatus != devicestatus.ReachabilityReachable ||
		updatedStatus.ConsecutiveFailures != 0 ||
		!updatedStatus.DiagnosticsComplete {
		t.Errorf("unexpected updated status: %+v", updatedStatus)
	}
	if updatedStatus.LastCheckedAt == nil ||
		updatedStatus.LastReachableAt == nil ||
		updatedStatus.DiagnosticsObservedAt == nil {
		t.Error("expected successful poll timestamps to be recorded")
	}
	if updatedStatus.Checksum == nil || *updatedStatus.Checksum != checksum {
		t.Errorf("expected checksum %q in snapshot", checksum)
	}

	if historyRepository.calls != 1 || historyRepository.createInput == nil {
		t.Fatal("expected one history entry to be created")
	}
	if !historyRepository.createInput.DiagnosticsComplete ||
		historyRepository.createInput.HardwareVersion == nil ||
		*historyRepository.createInput.HardwareVersion != "hardware-1" {
		t.Errorf("unexpected history entry: %+v", historyRepository.createInput)
	}
	if diagnosticsClient.discoverCalls != 1 || diagnosticsClient.diagnosticsCalls != 1 {
		t.Errorf("unexpected probe calls: discovery=%d diagnostics=%d",
			diagnosticsClient.discoverCalls, diagnosticsClient.diagnosticsCalls)
	}
}

func TestPollDeviceWithUnsupportedProtocol(t *testing.T) {
	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}
	diagnosticsClient := &monitoringDiagnosticsClientStub{
		// An empty capability response means /health responded but advertised
		// no diagnostics protocol supported by this service.
		capabilities: &deviceprotocol.Capabilities{},
	}

	_, databaseMock, monitoringService, statusRepository, historyRepository :=
		newMonitoringServiceTestFixture(t, 0, 3, initialStatus, diagnosticsClient)

	databaseMock.ExpectBegin()
	databaseMock.ExpectCommit()

	statusRepository.updateResult = &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}

	_, err := monitoringService.PollDevice(context.Background(), testDeviceID)
	if err != nil {
		t.Fatalf("poll device: %v", err)
	}

	updatedStatus := statusRepository.updateInput
	if updatedStatus == nil {
		t.Fatal("expected an updated status to be sent to the repository")
	}
	if updatedStatus.ReachabilityStatus != devicestatus.ReachabilityReachable {
		t.Errorf("expected reachable status, got %q", updatedStatus.ReachabilityStatus)
	}
	if updatedStatus.DiagnosticsComplete {
		t.Error("unsupported protocol must not produce complete diagnostics")
	}
	if updatedStatus.LastFailureCategory == nil ||
		*updatedStatus.LastFailureCategory != "unsupported_protocol" {
		t.Errorf("expected unsupported_protocol category, got %v",
			updatedStatus.LastFailureCategory)
	}
	if diagnosticsClient.diagnosticsCalls != 0 {
		t.Errorf("expected no diagnostics request, got %d",
			diagnosticsClient.diagnosticsCalls)
	}
	if historyRepository.createInput == nil ||
		historyRepository.createInput.FailureCategory == nil ||
		*historyRepository.createInput.FailureCategory != "unsupported_protocol" {
		t.Errorf("unexpected history entry: %+v", historyRepository.createInput)
	}
}

func TestPollDeviceRetriesThenStoresOnePollResult(t *testing.T) {
	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}
	diagnosticsClient := &monitoringDiagnosticsClientStub{
		discoverErr: errors.New("device did not respond"),
	}

	_, databaseMock, monitoringService, statusRepository, historyRepository :=
		newMonitoringServiceTestFixture(t, 2, 3, initialStatus, diagnosticsClient)

	databaseMock.ExpectBegin()
	databaseMock.ExpectCommit()

	statusRepository.updateResult = &devicestatus.DeviceStatus{
		DeviceID:            testDeviceID,
		ReachabilityStatus:  devicestatus.ReachabilityDegraded,
		ConsecutiveFailures: 1,
	}

	_, err := monitoringService.PollDevice(context.Background(), testDeviceID)
	if err != nil {
		t.Fatalf("poll device: %v", err)
	}

	if diagnosticsClient.discoverCalls != 3 {
		t.Errorf("expected 3 discovery attempts, got %d", diagnosticsClient.discoverCalls)
	}
	if statusRepository.updateCalls != 1 || historyRepository.calls != 1 {
		t.Errorf("expected one persisted poll cycle, got status updates=%d history entries=%d",
			statusRepository.updateCalls, historyRepository.calls)
	}
	if statusRepository.updateInput.ConsecutiveFailures != 1 {
		t.Errorf("expected one failure counted for the poll cycle, got %d",
			statusRepository.updateInput.ConsecutiveFailures)
	}
}

func TestPollDeviceFailureThresholdAndRecovery(t *testing.T) {
	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}
	diagnosticsClient := &monitoringDiagnosticsClientStub{
		discoverErr: errors.New("device did not respond"),
	}

	_, databaseMock, monitoringService, statusRepository, _ :=
		newMonitoringServiceTestFixture(t, 0, 2, initialStatus, diagnosticsClient)

	pollCases := []struct {
		name                string
		reachabilityStatus  devicestatus.ReachabilityStatus
		consecutiveFailures int
		recover             bool
	}{
		{
			name:                "first failed poll is degraded",
			reachabilityStatus:  devicestatus.ReachabilityDegraded,
			consecutiveFailures: 1,
		},
		{
			name:                "second failed poll reaches threshold",
			reachabilityStatus:  devicestatus.ReachabilityUnreachable,
			consecutiveFailures: 2,
		},
		{
			name:               "successful poll recovers device",
			reachabilityStatus: devicestatus.ReachabilityReachable,
			recover:            true,
		},
	}

	for pollIndex, pollCase := range pollCases {
		t.Run(pollCase.name, func(t *testing.T) {
			databaseMock.ExpectBegin()
			databaseMock.ExpectCommit()

			if pollCase.recover {
				checksum := "recovered-checksum"
				diagnosticsClient.discoverErr = nil
				diagnosticsClient.capabilities = &deviceprotocol.Capabilities{
					RESTDiagnosticsPath: "/diagnostics",
				}
				diagnosticsClient.diagnostics = &deviceprotocol.Diagnostics{
					HardwareVersion: "hardware-1",
					SoftwareVersion: "software-1",
					FirmwareVersion: "firmware-1",
					Status:          "healthy",
					Checksum:        &checksum,
				}

				statusRepository.updateResult = &devicestatus.DeviceStatus{
					DeviceID:           testDeviceID,
					ReachabilityStatus: devicestatus.ReachabilityReachable,
				}
			} else {
				statusRepository.updateResult = &devicestatus.DeviceStatus{
					DeviceID:            testDeviceID,
					ReachabilityStatus:  pollCase.reachabilityStatus,
					ConsecutiveFailures: pollCase.consecutiveFailures,
				}
			}

			savedStatus, err := monitoringService.PollDevice(
				context.Background(),
				testDeviceID,
			)
			if err != nil {
				t.Fatalf("poll device: %v", err)
			}
			if savedStatus.ReachabilityStatus != pollCase.reachabilityStatus {
				t.Errorf("expected status %q, got %q",
					pollCase.reachabilityStatus,
					savedStatus.ReachabilityStatus,
				)
			}

			updatedStatus := statusRepository.updateInput
			if updatedStatus == nil {
				t.Fatal("expected an updated status to be sent to the repository")
			}
			if updatedStatus.ConsecutiveFailures != pollCase.consecutiveFailures {
				t.Errorf("expected %d consecutive failures, got %d",
					pollCase.consecutiveFailures,
					updatedStatus.ConsecutiveFailures,
				)
			}

			// The static stub does not persist data automatically. Feed its
			// captured update into the next poll as the database's current row.
			statusRepository.getResult = updatedStatus

			if pollIndex < len(pollCases)-1 {
				// PollDevice rejects results older than the status already stored.
				time.Sleep(2 * time.Millisecond)
			}
		})
	}
}

func TestPollDeviceRollsBackWhenHistoryInsertFails(t *testing.T) {
	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}
	diagnosticsClient := &monitoringDiagnosticsClientStub{
		discoverErr: errors.New("device did not respond"),
	}

	_, databaseMock, monitoringService, statusRepository, historyRepository :=
		newMonitoringServiceTestFixture(t, 0, 3, initialStatus, diagnosticsClient)

	databaseMock.ExpectBegin()
	databaseMock.ExpectRollback()

	statusRepository.updateResult = &devicestatus.DeviceStatus{
		DeviceID:            testDeviceID,
		ReachabilityStatus:  devicestatus.ReachabilityDegraded,
		ConsecutiveFailures: 1,
	}
	expectedHistoryError := errors.New("history insert failed")
	historyRepository.err = expectedHistoryError

	_, err := monitoringService.PollDevice(context.Background(), testDeviceID)
	if !errors.Is(err, expectedHistoryError) {
		t.Fatalf("expected history insert error, got: %v", err)
	}
	if statusRepository.updateCalls != 1 || historyRepository.calls != 1 {
		t.Errorf("expected status update and history insert, got updates=%d history=%d",
			statusRepository.updateCalls, historyRepository.calls)
	}
}

func TestListStatuses(t *testing.T) {
	repositoryError := errors.New("list statuses failed")

	for _, testCase := range []struct {
		name            string
		statuses        []*devicestatus.DeviceStatus
		repositoryError error
	}{
		{
			name: "returns active statuses",
			statuses: []*devicestatus.DeviceStatus{
				{
					DeviceID:           testDeviceID,
					ReachabilityStatus: devicestatus.ReachabilityReachable,
				},
			},
		},
		{
			name:     "returns empty list",
			statuses: []*devicestatus.DeviceStatus{},
		},
		{
			name:            "returns repository error",
			repositoryError: repositoryError,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			statusRepository := &monitoringStatusRepositoryStub{
				listResult: testCase.statuses,
				listErr:    testCase.repositoryError,
			}
			monitoringService := service.NewMonitoringService(
				db,
				&deviceRepositoryStub{},
				statusRepository,
				&monitoringHistoryRepositoryStub{},
				&monitoringDiagnosticsClientStub{},
			)

			requestContext := t.Context()
			gotStatuses, err := monitoringService.ListStatuses(requestContext)

			if statusRepository.listCalls != 1 {
				t.Fatalf("expected one ListActive call, got %d", statusRepository.listCalls)
			}
			if statusRepository.listContext != requestContext {
				t.Error("expected request context to be passed to the repository")
			}
			if statusRepository.listTx != nil {
				t.Error("expected status listing not to use a transaction")
			}

			if testCase.repositoryError != nil {
				if !errors.Is(err, testCase.repositoryError) {
					t.Fatalf("expected repository error, got: %v", err)
				}
				if gotStatuses != nil {
					t.Errorf("expected nil statuses on error, got %+v", gotStatuses)
				}
				return
			}

			if err != nil {
				t.Fatalf("list statuses: %v", err)
			}
			if len(gotStatuses) != len(testCase.statuses) {
				t.Fatalf("expected %d statuses, got %d",
					len(testCase.statuses), len(gotStatuses))
			}
			if len(gotStatuses) > 0 &&
				gotStatuses[0].DeviceID != testCase.statuses[0].DeviceID {
				t.Errorf("expected device ID %q, got %q",
					testCase.statuses[0].DeviceID, gotStatuses[0].DeviceID)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	repositoryError := errors.New("get status failed")
	expectedStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}

	for _, testCase := range []struct {
		name             string
		deviceID         string
		repositoryResult *devicestatus.DeviceStatus
		repositoryError  error
		wantError        string
	}{
		{
			name:             "returns status",
			deviceID:         testDeviceID,
			repositoryResult: expectedStatus,
		},
		{
			name:      "rejects invalid device ID",
			deviceID:  "not-a-uuid",
			wantError: "device ID must be a valid UUID",
		},
		{
			name:            "returns repository error",
			deviceID:        testDeviceID,
			repositoryError: repositoryError,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, _ := openMockDatabase(t)
			statusRepository := &monitoringStatusRepositoryStub{
				getResult: testCase.repositoryResult,
				getErr:    testCase.repositoryError,
			}
			monitoringService := service.NewMonitoringService(
				db,
				&deviceRepositoryStub{},
				statusRepository,
				&monitoringHistoryRepositoryStub{},
				&monitoringDiagnosticsClientStub{},
			)

			requestContext := t.Context()
			gotStatus, err := monitoringService.GetStatus(
				requestContext,
				testCase.deviceID,
			)

			if testCase.wantError != "" {
				if err == nil || err.Error() != testCase.wantError {
					t.Fatalf("expected error %q, got: %v", testCase.wantError, err)
				}
				if gotStatus != nil {
					t.Errorf("expected nil status, got %+v", gotStatus)
				}
				if statusRepository.getCalls != 0 {
					t.Error("repository should not be called for an invalid ID")
				}
				return
			}

			if testCase.repositoryError != nil {
				if !errors.Is(err, testCase.repositoryError) {
					t.Fatalf("expected repository error, got: %v", err)
				}
				if gotStatus != nil {
					t.Errorf("expected nil status on error, got %+v", gotStatus)
				}
			} else {
				if err != nil {
					t.Fatalf("get status: %v", err)
				}
				if gotStatus == nil || gotStatus.DeviceID != expectedStatus.DeviceID {
					t.Errorf("expected status for %q, got %+v",
						expectedStatus.DeviceID, gotStatus)
				}
			}

			if statusRepository.getCalls != 1 {
				t.Fatalf("expected one Get call, got %d", statusRepository.getCalls)
			}
			if statusRepository.getID != testCase.deviceID {
				t.Errorf("expected device ID %q, got %q",
					testCase.deviceID, statusRepository.getID)
			}
			if statusRepository.getTx != nil {
				t.Error("expected status lookup not to use a transaction")
			}
		})
	}
}

func TestPollDevicePrefersGRPC(t *testing.T) {
	checksum := "test-checksum"

	initialStatus := &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityUnknown,
	}

	diagnosticsClient := &monitoringDiagnosticsClientStub{
		capabilities: &deviceprotocol.Capabilities{
			RESTDiagnosticsPath: "/diagnostics",
			GRPCTarget:          "localhost:9091",
		},
		grpcDiagnostics: &deviceprotocol.Diagnostics{
			HardwareVersion: "hardware-1",
			SoftwareVersion: "software-1",
			FirmwareVersion: "firmware-1",
			Status:          "healthy",
			Checksum:        &checksum,
		},
	}

	_, databaseMock, monitoringService, statusRepository, historyRepository :=
		newMonitoringServiceTestFixture(t, 0, 3, initialStatus, diagnosticsClient)

	databaseMock.ExpectBegin()
	databaseMock.ExpectCommit()

	statusRepository.updateResult = &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}

	_, err := monitoringService.PollDevice(context.Background(), testDeviceID)
	if err != nil {
		t.Fatalf("poll device: %v", err)
	}

	if diagnosticsClient.grpcDiagnosticsCalls != 1 {
		t.Errorf(
			"expected one gRPC diagnostics call, got %d",
			diagnosticsClient.grpcDiagnosticsCalls,
		)
	}

	if diagnosticsClient.grpcTarget != "localhost:9091" {
		t.Errorf("unexpected gRPC target: %q", diagnosticsClient.grpcTarget)
	}

	if diagnosticsClient.diagnosticsCalls != 0 {
		t.Errorf(
			"expected no REST diagnostics calls, got %d",
			diagnosticsClient.diagnosticsCalls,
		)
	}

	if statusRepository.updateInput == nil {
		t.Fatal("expected an updated status")
	}

	if !statusRepository.updateInput.DiagnosticsComplete {
		t.Error("expected complete diagnostics")
	}

	if statusRepository.updateInput.ReachabilityStatus != devicestatus.ReachabilityReachable {
		t.Errorf(
			"expected reachable status, got %q",
			statusRepository.updateInput.ReachabilityStatus,
		)
	}

	if historyRepository.createInput == nil ||
		historyRepository.createInput.Checksum == nil ||
		*historyRepository.createInput.Checksum != checksum {
		t.Errorf("unexpected history entry: %+v", historyRepository.createInput)
	}

	if err := databaseMock.ExpectationsWereMet(); err != nil {
		t.Errorf("database expectations: %v", err)
	}
}
