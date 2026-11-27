package probe

import (
	"context"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
)

// DeviceClient combines HTTP health discovery with the two diagnostics
// transports. The monitoring service decides which transport to use.
type DeviceClient struct {
	restClient *RESTClient
	grpcClient *GRPCClient
}

func NewDeviceClient(
	restClient *RESTClient,
	grpcClient *GRPCClient,
) *DeviceClient {
	return &DeviceClient{
		restClient: restClient,
		grpcClient: grpcClient,
	}
}

func (deviceClient *DeviceClient) Discover(
	requestContext context.Context,
	healthURL string,
) (*deviceprotocol.Capabilities, error) {
	return deviceClient.restClient.Discover(requestContext, healthURL)
}

func (deviceClient *DeviceClient) FetchDiagnostics(
	requestContext context.Context,
	healthURL string,
	diagnosticsPath string,
) (*deviceprotocol.Diagnostics, error) {
	return deviceClient.restClient.FetchDiagnostics(
		requestContext,
		healthURL,
		diagnosticsPath,
	)
}

func (deviceClient *DeviceClient) FetchGRPCDiagnostics(
	requestContext context.Context,
	healthURL string,
	grpcTarget string,
) (*deviceprotocol.Diagnostics, error) {
	return deviceClient.grpcClient.FetchDiagnostics(
		requestContext,
		healthURL,
		grpcTarget,
	)
}
