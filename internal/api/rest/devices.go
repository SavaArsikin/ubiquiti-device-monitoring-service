package rest

import (
	"net/http"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
)

func (apiHandler *handler) handleDeviceCollection(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	switch request.Method {
	case http.MethodGet:
		apiHandler.listDevices(responseWriter, request)
	case http.MethodPost:
		apiHandler.createDevice(responseWriter, request)
	default:
		apiHandler.writeMethodNotAllowed(
			responseWriter,
			http.MethodGet,
			http.MethodPost,
		)
	}
}

func (apiHandler *handler) listDevices(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	var (
		listedDevices []*device.Device
		err           error
	)

	switch request.URL.Query().Get("state") {
	case "", "active":
		listedDevices, err = apiHandler.devices.ListActive(request.Context())
	case "archived":
		listedDevices, err = apiHandler.devices.ListArchived(request.Context())
	default:
		apiHandler.writeAPIError(
			responseWriter,
			http.StatusBadRequest,
			"invalid_state",
			"state must be active or archived",
		)
		return
	}

	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	deviceResponses := make([]deviceResponse, 0, len(listedDevices))
	for _, listedDevice := range listedDevices {
		deviceResponses = append(deviceResponses, toDeviceResponse(listedDevice))
	}

	apiHandler.writeJSON(responseWriter, http.StatusOK, deviceResponses)
}

func (apiHandler *handler) createDevice(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	var input deviceInput
	if err := decodeJSONRequest(responseWriter, request, &input); err != nil {
		apiHandler.writeRequestError(responseWriter, err)
		return
	}

	createdDevice, err := apiHandler.devices.Create(
		request.Context(),
		input.toDevice(),
	)
	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	responseWriter.Header().Set(
		"Location",
		"/api/v1/devices/"+createdDevice.ID,
	)
	apiHandler.writeJSON(
		responseWriter,
		http.StatusCreated,
		toDeviceResponse(createdDevice),
	)
}

func (apiHandler *handler) handleDevice(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	switch request.Method {
	case http.MethodGet:
		apiHandler.getDevice(responseWriter, request)
	case http.MethodPut:
		apiHandler.replaceDevice(responseWriter, request)
	case http.MethodDelete:
		apiHandler.archiveDevice(responseWriter, request)
	default:
		apiHandler.writeMethodNotAllowed(
			responseWriter,
			http.MethodGet,
			http.MethodPut,
			http.MethodDelete,
		)
	}
}

func (apiHandler *handler) getDevice(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	foundDevice, err := apiHandler.devices.Get(
		request.Context(),
		request.PathValue("deviceID"),
	)
	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	apiHandler.writeJSON(
		responseWriter,
		http.StatusOK,
		toDeviceResponse(foundDevice),
	)
}

func (apiHandler *handler) replaceDevice(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	var input deviceInput
	if err := decodeJSONRequest(responseWriter, request, &input); err != nil {
		apiHandler.writeRequestError(responseWriter, err)
		return
	}

	replacementDevice := input.toDevice()
	replacementDevice.ID = request.PathValue("deviceID")

	updatedDevice, err := apiHandler.devices.Update(
		request.Context(),
		replacementDevice,
	)
	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	apiHandler.writeJSON(
		responseWriter,
		http.StatusOK,
		toDeviceResponse(updatedDevice),
	)
}

func (apiHandler *handler) archiveDevice(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if err := apiHandler.devices.Archive(
		request.Context(),
		request.PathValue("deviceID"),
	); err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	responseWriter.WriteHeader(http.StatusNoContent)
}

func (apiHandler *handler) handleDeviceRestore(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodPost {
		apiHandler.writeMethodNotAllowed(responseWriter, http.MethodPost)
		return
	}

	restoredDevice, err := apiHandler.devices.Restore(
		request.Context(),
		request.PathValue("deviceID"),
	)
	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	apiHandler.writeJSON(
		responseWriter,
		http.StatusOK,
		toDeviceResponse(restoredDevice),
	)
}
