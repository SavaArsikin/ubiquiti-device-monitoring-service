package rest

import "net/http"

func (apiHandler *handler) handleStatusCollection(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodGet {
		apiHandler.writeMethodNotAllowed(responseWriter, http.MethodGet)
		return
	}

	deviceStatuses, err := apiHandler.statuses.ListStatuses(request.Context())
	if err != nil {
		apiHandler.writeServiceError(responseWriter, err)
		return
	}

	statusResponses := make([]deviceStatusResponse, 0, len(deviceStatuses))
	for _, deviceStatus := range deviceStatuses {
		statusResponses = append(
			statusResponses,
			toDeviceStatusResponse(deviceStatus),
		)
	}

	apiHandler.writeJSON(responseWriter, http.StatusOK, statusResponses)
}

func (apiHandler *handler) handleDeviceStatus(
	responseWriter http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodGet {
		apiHandler.writeMethodNotAllowed(responseWriter, http.MethodGet)
		return
	}

	deviceStatus, err := apiHandler.statuses.GetStatus(
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
		toDeviceStatusResponse(deviceStatus),
	)
}
