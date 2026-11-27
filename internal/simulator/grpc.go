package simulator

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
	devicev1 "github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol/grpc/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type grpcDiagnosticsServer struct {
	// Embed by value, as required by the generated server interface.
	devicev1.UnimplementedDeviceDiagnosticsServer

	config    Config
	generator checksumGenerator
	requests  atomic.Uint64
}

func NewGRPCDiagnosticsServer(
	simulatorConfig Config,
	generator checksumGenerator,
) devicev1.DeviceDiagnosticsServer {
	return &grpcDiagnosticsServer{
		config:    simulatorConfig,
		generator: generator,
	}
}

func (server *grpcDiagnosticsServer) GetDiagnostics(
	requestContext context.Context,
	_ *devicev1.GetDiagnosticsRequest,
) (*devicev1.GetDiagnosticsResponse, error) {
	if err := requestContext.Err(); err != nil {
		return nil, grpcstatus.FromContextError(err).Err()
	}

	requestNumber := server.requests.Add(1)

	// These behaviors mirror the existing REST simulator. The gRPC
	// endpoint has its own request counter.
	switch server.config.DiagnosticsBehavior {
	case "healthy", "incomplete":
		// Continue to build the diagnostics response.

	case "error":
		return nil, grpcstatus.Error(
			codes.Unavailable,
			"simulated device error",
		)

	case "flaky":
		if requestNumber <= uint64(server.config.FailureCount) {
			return nil, grpcstatus.Error(
				codes.Unavailable,
				"simulated temporary failure",
			)
		}

	case "slow":
		if err := wait(requestContext, server.config.ResponseDelay); err != nil {
			return nil, grpcstatus.FromContextError(err).Err()
		}

	default:
		return nil, grpcstatus.Error(
			codes.Internal,
			"invalid simulator diagnostics behavior",
		)
	}

	diagnostics := deviceprotocol.Diagnostics{
		HardwareVersion: server.config.HardwareVersion,
		SoftwareVersion: server.config.SoftwareVersion,
		FirmwareVersion: server.config.FirmwareVersion,
		Status:          server.config.DeviceStatus,
	}

	response := &devicev1.GetDiagnosticsResponse{
		HardwareVersion: diagnostics.HardwareVersion,
		SoftwareVersion: diagnostics.SoftwareVersion,
		FirmwareVersion: diagnostics.FirmwareVersion,
		Status:          diagnostics.Status,
	}

	if server.config.DiagnosticsBehavior == "incomplete" {
		response.FirmwareVersion = ""
		response.FailureCategory = "invalid_diagnostics"
		return response, nil
	}

	// Reuse the same external checksum generator as the REST endpoint.
	checksum, err := server.generator.Generate(requestContext, diagnostics)
	if err != nil {
		// A canceled RPC should stop rather than return a partial response.
		if contextError := requestContext.Err(); contextError != nil {
			return nil, grpcstatus.FromContextError(contextError).Err()
		}

		slog.Warn("checksum unavailable", "error", err)
		response.FailureCategory = "checksum_unavailable"
		return response, nil
	}

	response.Checksum = &checksum
	return response, nil
}
