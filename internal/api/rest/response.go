package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/service"
)

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toDeviceResponse(monitoredDevice *device.Device) deviceResponse {
	return deviceResponse{
		ID:         monitoredDevice.ID,
		Name:       monitoredDevice.Name,
		HealthURL:  monitoredDevice.HealthURL,
		CreatedAt:  monitoredDevice.CreatedAt,
		UpdatedAt:  monitoredDevice.UpdatedAt,
		ArchivedAt: monitoredDevice.ArchivedAt,
	}
}

func toDeviceStatusResponse(
	status *devicestatus.DeviceStatus,
) deviceStatusResponse {
	return deviceStatusResponse{
		DeviceID:              status.DeviceID,
		ReachabilityStatus:    status.ReachabilityStatus,
		ConsecutiveFailures:   status.ConsecutiveFailures,
		DiagnosticsComplete:   status.DiagnosticsComplete,
		LastCheckedAt:         status.LastCheckedAt,
		LastReachableAt:       status.LastReachableAt,
		LastFailureCategory:   status.LastFailureCategory,
		DeviceReportedStatus:  status.DeviceReportedStatus,
		HardwareVersion:       status.HardwareVersion,
		SoftwareVersion:       status.SoftwareVersion,
		FirmwareVersion:       status.FirmwareVersion,
		Checksum:              status.Checksum,
		DiagnosticsObservedAt: status.DiagnosticsObservedAt,
	}
}

func (apiHandler *handler) writeRequestError(
	responseWriter http.ResponseWriter,
	requestError error,
) {
	if errors.Is(requestError, errRequestBodyTooLarge) {
		apiHandler.writeAPIError(
			responseWriter,
			http.StatusRequestEntityTooLarge,
			"request_too_large",
			"request body exceeds the allowed size",
		)
		return
	}

	apiHandler.writeAPIError(
		responseWriter,
		http.StatusBadRequest,
		"invalid_json",
		"request body must contain a valid JSON object",
	)
}

func (apiHandler *handler) writeServiceError(
	responseWriter http.ResponseWriter,
	serviceError error,
) {
	switch {
	case errors.Is(serviceError, service.ErrInvalidDeviceID),
		errors.Is(serviceError, service.ErrDeviceRequired),
		errors.Is(serviceError, service.ErrDeviceNameRequired),
		errors.Is(serviceError, service.ErrInvalidHealthURL):
		apiHandler.writeAPIError(
			responseWriter,
			http.StatusBadRequest,
			"invalid_request",
			serviceError.Error(),
		)

	case errors.Is(serviceError, device.ErrDeviceNotFound),
		errors.Is(serviceError, devicestatus.ErrDeviceStatusNotFound):
		apiHandler.writeAPIError(
			responseWriter,
			http.StatusNotFound,
			"not_found",
			"device or status not found",
		)

	default:
		slog.Error("REST API service request failed", "error", serviceError)
		apiHandler.writeAPIError(
			responseWriter,
			http.StatusInternalServerError,
			"internal_error",
			"an internal error occurred",
		)
	}
}

func (apiHandler *handler) writeMethodNotAllowed(
	responseWriter http.ResponseWriter,
	allowedMethods ...string,
) {
	responseWriter.Header().Set("Allow", strings.Join(allowedMethods, ", "))
	apiHandler.writeAPIError(
		responseWriter,
		http.StatusMethodNotAllowed,
		"method_not_allowed",
		"HTTP method is not allowed for this route",
	)
}

func (apiHandler *handler) writeAPIError(
	responseWriter http.ResponseWriter,
	statusCode int,
	errorCode string,
	message string,
) {
	apiHandler.writeJSON(
		responseWriter,
		statusCode,
		apiErrorResponse{
			Error: apiError{
				Code:    errorCode,
				Message: message,
			},
		},
	)
}

func (apiHandler *handler) writeJSON(
	responseWriter http.ResponseWriter,
	statusCode int,
	responseBody any,
) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)

	if err := json.NewEncoder(responseWriter).Encode(responseBody); err != nil {
		slog.Error("write REST API response", "error", err)
	}
}
