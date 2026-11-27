package probe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
	devicev1 "github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol/grpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
)

type GRPCClient struct{}

func NewGRPCClient() *GRPCClient {
	return &GRPCClient{}
}

func (grpcClient *GRPCClient) FetchDiagnostics(
	requestContext context.Context,
	healthURL string,
	grpcTarget string,
) (*deviceprotocol.Diagnostics, error) {
	if err := requestContext.Err(); err != nil {
		return nil, fmt.Errorf("fetch gRPC diagnostics: %w", err)
	}

	if err := validateGRPCTarget(healthURL, grpcTarget); err != nil {
		return nil, fmt.Errorf("validate gRPC target: %w", err)
	}

	// This is an unencrypted connection for the local simulator only.
	// passthrough treats the validated host:port as the target directly.
	connection, err := grpc.NewClient(
		"passthrough:///"+grpcTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("create gRPC connection: %w", err)
	}

	defer func() {
		if closeError := connection.Close(); closeError != nil {
			slog.Warn("close gRPC connection", "error", closeError)
		}
	}()

	client := devicev1.NewDeviceDiagnosticsClient(connection)

	response, err := client.GetDiagnostics(
		requestContext,
		&devicev1.GetDiagnosticsRequest{},
	)
	if err != nil {
		// gRPC returns status errors. Preserve context cancellation and
		// deadline information for the monitoring service's classification.
		switch grpcstatus.Code(err) {
		case codes.Canceled:
			return nil, fmt.Errorf(
				"fetch gRPC diagnostics: %w",
				errors.Join(err, context.Canceled),
			)
		case codes.DeadlineExceeded:
			return nil, fmt.Errorf(
				"fetch gRPC diagnostics: %w",
				errors.Join(err, context.DeadlineExceeded),
			)
		default:
			return nil, fmt.Errorf("fetch gRPC diagnostics: %w", err)
		}
	}

	diagnostics := &deviceprotocol.Diagnostics{
		HardwareVersion: response.HardwareVersion,
		SoftwareVersion: response.SoftwareVersion,
		FirmwareVersion: response.FirmwareVersion,
		Status:          response.Status,
		Checksum:        response.Checksum,
		FailureCategory: response.FailureCategory,
	}

	// Match the existing REST client's validation. Return partial data so
	// the monitoring service can retain it in history.
	if diagnostics.HardwareVersion == "" ||
		diagnostics.SoftwareVersion == "" ||
		diagnostics.FirmwareVersion == "" ||
		diagnostics.Status == "" {
		return diagnostics, ErrInvalidDiagnostics
	}

	return diagnostics, nil
}

func validateGRPCTarget(healthURL string, grpcTarget string) error {
	parsedHealthURL, err := url.Parse(healthURL)
	if err != nil || parsedHealthURL.Hostname() == "" {
		return errors.New("invalid health URL")
	}

	targetHost, portText, err := net.SplitHostPort(grpcTarget)
	if err != nil || targetHost == "" {
		return errors.New("gRPC target must be a host:port address")
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("gRPC target port must be between 1 and 65535")
	}

	// Like the REST probe's same-device restriction, do not allow a health
	// response to direct the monitor to an unrelated host.
	healthHost := parsedHealthURL.Hostname()

	healthIP := net.ParseIP(healthHost)
	targetIP := net.ParseIP(targetHost)

	if healthIP != nil && targetIP != nil {
		if !healthIP.Equal(targetIP) {
			return errors.New("gRPC target host must match health host")
		}
	} else if !strings.EqualFold(healthHost, targetHost) {
		return errors.New("gRPC target host must match health host")
	}

	return nil
}
