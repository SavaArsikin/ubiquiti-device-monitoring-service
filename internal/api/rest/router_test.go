package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

const (
	testAPIUsername = "test-user"
	testAPIPassword = "test-password"
	testDeviceID    = "123e4567-e89b-12d3-a456-426614174000"
)

type routeDeviceServiceStub struct {
	device *device.Device
	err    error
}

func (stub *routeDeviceServiceStub) Create(
	_ context.Context,
	_ *device.Device,
) (*device.Device, error) {
	return stub.device, stub.err
}

func (stub *routeDeviceServiceStub) Get(
	_ context.Context,
	_ string,
) (*device.Device, error) {
	return stub.device, stub.err
}

func (stub *routeDeviceServiceStub) ListActive(
	_ context.Context,
) ([]*device.Device, error) {
	return []*device.Device{}, stub.err
}

func (stub *routeDeviceServiceStub) ListArchived(
	_ context.Context,
) ([]*device.Device, error) {
	return []*device.Device{}, stub.err
}

func (stub *routeDeviceServiceStub) Update(
	_ context.Context,
	_ *device.Device,
) (*device.Device, error) {
	return stub.device, stub.err
}

func (stub *routeDeviceServiceStub) Archive(
	_ context.Context,
	_ string,
) error {
	return stub.err
}

func (stub *routeDeviceServiceStub) Restore(
	_ context.Context,
	_ string,
) (*device.Device, error) {
	return stub.device, stub.err
}

type routeStatusQueriesStub struct {
	err error
}

func (stub *routeStatusQueriesStub) ListStatuses(
	_ context.Context,
) ([]*devicestatus.DeviceStatus, error) {
	return []*devicestatus.DeviceStatus{}, stub.err
}

func (stub *routeStatusQueriesStub) GetStatus(
	_ context.Context,
	_ string,
) (*devicestatus.DeviceStatus, error) {
	return &devicestatus.DeviceStatus{
		DeviceID:           testDeviceID,
		ReachabilityStatus: devicestatus.ReachabilityReachable,
	}, stub.err
}

func TestRoutesStatusCodes(t *testing.T) {
	testDevice := &device.Device{
		ID:        testDeviceID,
		Name:      "Convention center router",
		HealthURL: "http://192.0.2.10/health",
	}

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		username       string
		password       string
		deviceError    error
		statusError    error
		expectedStatus int
	}{
		{
			name:           "list devices",
			method:         http.MethodGet,
			path:           "/api/v1/devices",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "create device",
			method:         http.MethodPost,
			path:           "/api/v1/devices",
			body:           `{"name":"Router","health_url":"http://192.0.2.10/health"}`,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "get device",
			method:         http.MethodGet,
			path:           "/api/v1/devices/" + testDeviceID,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "replace device",
			method:         http.MethodPut,
			path:           "/api/v1/devices/" + testDeviceID,
			body:           `{"name":"Updated router","health_url":"http://192.0.2.11/health"}`,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "archive device",
			method:         http.MethodDelete,
			path:           "/api/v1/devices/" + testDeviceID,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "restore device",
			method:         http.MethodPost,
			path:           "/api/v1/devices/" + testDeviceID + "/restore",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "list statuses",
			method:         http.MethodGet,
			path:           "/api/v1/status",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get device status",
			method:         http.MethodGet,
			path:           "/api/v1/devices/" + testDeviceID + "/status",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			path:           "/api/v1/devices",
			body:           `{"name":`,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid device state filter",
			method:         http.MethodGet,
			path:           "/api/v1/devices?state=unknown",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing credentials",
			method:         http.MethodGet,
			path:           "/api/v1/status",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "incorrect credentials",
			method:         http.MethodGet,
			path:           "/api/v1/status",
			username:       "wrong-user",
			password:       "wrong-password",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "device not found",
			method:         http.MethodGet,
			path:           "/api/v1/devices/" + testDeviceID,
			username:       testAPIUsername,
			password:       testAPIPassword,
			deviceError:    device.ErrDeviceNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown route",
			method:         http.MethodGet,
			path:           "/not-a-route",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "method not allowed",
			method:         http.MethodDelete,
			path:           "/api/v1/status",
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "request body too large",
			method:         http.MethodPost,
			path:           "/api/v1/devices",
			body:           `"` + strings.Repeat("x", maxJSONRequestBytes) + `"`,
			username:       testAPIUsername,
			password:       testAPIPassword,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "internal service error",
			method:         http.MethodGet,
			path:           "/api/v1/status",
			username:       testAPIUsername,
			password:       testAPIPassword,
			statusError:    errors.New("database unavailable"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			deviceService := &routeDeviceServiceStub{
				device: testDevice,
				err:    testCase.deviceError,
			}
			statusService := &routeStatusQueriesStub{
				err: testCase.statusError,
			}

			handler := NewHandler(
				deviceService,
				statusService,
				testAPIUsername,
				testAPIPassword,
			)

			request := httptest.NewRequest(
				testCase.method,
				testCase.path,
				strings.NewReader(testCase.body),
			)
			if testCase.username != "" {
				request.SetBasicAuth(testCase.username, testCase.password)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != testCase.expectedStatus {
				t.Fatalf(
					"unexpected status code: got %d, want %d; body: %s",
					response.Code,
					testCase.expectedStatus,
					response.Body.String(),
				)
			}

			if testCase.expectedStatus != http.StatusNoContent &&
				response.Header().Get("Content-Type") != "application/json" {
				t.Errorf(
					"unexpected Content-Type: got %q, want application/json",
					response.Header().Get("Content-Type"),
				)
			}

			if testCase.expectedStatus == http.StatusUnauthorized &&
				response.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 response is missing the WWW-Authenticate header")
			}

			if testCase.expectedStatus == http.StatusMethodNotAllowed &&
				response.Header().Get("Allow") == "" {
				t.Error("405 response is missing the Allow header")
			}
		})
	}
}
