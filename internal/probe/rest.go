package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
)

var ErrInvalidDiagnostics = errors.New("device diagnostics are missing required fields")

type RESTClient struct {
	httpClient *http.Client
}

func NewRESTClient(httpClient *http.Client) *RESTClient {
	return &RESTClient{
		httpClient: httpClient,
	}
}

func (restClient *RESTClient) Discover(
	requestContext context.Context,
	healthURL string,
) (*deviceprotocol.Capabilities, error) {
	var deviceCapabilities deviceprotocol.Capabilities

	if err := restClient.getJSONResponse(
		requestContext,
		healthURL,
		&deviceCapabilities,
	); err != nil {
		return nil, fmt.Errorf("discover device capabilities: %w", err)
	}

	// Return what the device advertised even if it supports no protocol
	// that this service can use. The caller can record the device as
	// reachable while reporting that its diagnostics are unsupported.
	return &deviceCapabilities, nil
}

func (restClient *RESTClient) FetchDiagnostics(
	requestContext context.Context,
	healthURL string,
	diagnosticsPath string,
) (*deviceprotocol.Diagnostics, error) {
	parsedHealthURL, err := url.Parse(healthURL)
	if err != nil {
		return nil, errors.New("invalid health URL")
	}

	parsedDiagnosticsPath, err := url.Parse(diagnosticsPath)
	if err != nil ||
		parsedDiagnosticsPath.IsAbs() ||
		parsedDiagnosticsPath.Host != "" ||
		!strings.HasPrefix(parsedDiagnosticsPath.Path, "/") ||
		parsedDiagnosticsPath.Fragment != "" {
		return nil, errors.New("diagnostics path must be an absolute path on the device")
	}

	// Resolve the path against the device URL, rather than allowing a device
	// response to direct this client to an unrelated host.
	diagnosticsURL := parsedHealthURL.ResolveReference(parsedDiagnosticsPath)

	var deviceDiagnostics deviceprotocol.Diagnostics
	if err := restClient.getJSONResponse(
		requestContext,
		diagnosticsURL.String(),
		&deviceDiagnostics,
	); err != nil {
		return nil, fmt.Errorf("fetch device diagnostics: %w", err)
	}

	if deviceDiagnostics.HardwareVersion == "" ||
		deviceDiagnostics.SoftwareVersion == "" ||
		deviceDiagnostics.FirmwareVersion == "" ||
		deviceDiagnostics.Status == "" {
		// Return the received fields so the caller can retain them in poll
		// history, while still marking this diagnostics result incomplete.
		return &deviceDiagnostics, ErrInvalidDiagnostics
	}

	return &deviceDiagnostics, nil
}

func (restClient *RESTClient) getJSONResponse(
	requestContext context.Context,
	targetURL string,
	destination any,
) error {
	httpRequest, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		targetURL,
		nil,
	)
	if err != nil {
		return fmt.Errorf("create device request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")

	httpResponse, err := restClient.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("send device request: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("device returned HTTP status %d", httpResponse.StatusCode)
	}

	const maxResponseSizeBytes = 64 * 1024

	responseBody, err := io.ReadAll(
		io.LimitReader(httpResponse.Body, maxResponseSizeBytes+1),
	)
	if err != nil {
		return fmt.Errorf("read device response: %w", err)
	}
	if len(responseBody) > maxResponseSizeBytes {
		return errors.New("device response exceeds size limit")
	}

	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode device response: %w", err)
	}

	return nil
}
