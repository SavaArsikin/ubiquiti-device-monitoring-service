package rest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var (
	errInvalidJSONRequest  = errors.New("request body must be one valid JSON value")
	errRequestBodyTooLarge = errors.New("request body exceeds size limit")
)

const maxJSONRequestBytes = 1 << 20 // 1 MiB

func decodeJSONRequest(
	responseWriter http.ResponseWriter,
	request *http.Request,
	destination any,
) error {
	requestBody := http.MaxBytesReader(
		responseWriter,
		request.Body,
		maxJSONRequestBytes,
	)
	decoder := json.NewDecoder(requestBody)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return errRequestBodyTooLarge
		}

		return errInvalidJSONRequest
	}

	// A second decode must reach EOF; otherwise the body contains trailing
	// data or another JSON value that the handler would silently ignore.
	var trailingValue json.RawMessage
	if err := decoder.Decode(&trailingValue); !errors.Is(err, io.EOF) {
		return errInvalidJSONRequest
	}

	return nil
}
