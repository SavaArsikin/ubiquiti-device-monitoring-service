package rest

import (
	"context"
	"net/http"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

type deviceOperations interface {
	Create(context.Context, *device.Device) (*device.Device, error)
	Get(context.Context, string) (*device.Device, error)
	ListActive(context.Context) ([]*device.Device, error)
	ListArchived(context.Context) ([]*device.Device, error)
	Update(context.Context, *device.Device) (*device.Device, error)
	Archive(context.Context, string) error
	Restore(context.Context, string) (*device.Device, error)
}

type statusQueries interface {
	ListStatuses(context.Context) ([]*devicestatus.DeviceStatus, error)
	GetStatus(context.Context, string) (*devicestatus.DeviceStatus, error)
}

type handler struct {
	devices  deviceOperations
	statuses statusQueries
	username string
	password string
}

func NewHandler(
	deviceService deviceOperations,
	monitoringService statusQueries,
	username string,
	password string,
) http.Handler {
	apiHandler := &handler{
		devices:  deviceService,
		statuses: monitoringService,
		username: username,
		password: password,
	}

	router := http.NewServeMux()
	router.HandleFunc("/api/v1/devices", apiHandler.handleDeviceCollection)
	router.HandleFunc("/api/v1/devices/{deviceID}/restore", apiHandler.handleDeviceRestore)
	router.HandleFunc("/api/v1/devices/{deviceID}/status", apiHandler.handleDeviceStatus)
	router.HandleFunc("/api/v1/devices/{deviceID}", apiHandler.handleDevice)
	router.HandleFunc("/api/v1/status", apiHandler.handleStatusCollection)
	router.HandleFunc("/", apiHandler.handleNotFound)

	return apiHandler.requireBasicAuthentication(router)
}

func (apiHandler *handler) handleNotFound(
	responseWriter http.ResponseWriter,
	_ *http.Request,
) {
	apiHandler.writeAPIError(
		responseWriter,
		http.StatusNotFound,
		"not_found",
		"route not found",
	)
}
