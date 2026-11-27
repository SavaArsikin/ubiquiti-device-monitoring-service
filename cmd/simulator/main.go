package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	devicev1 "github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol/grpc/v1"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/simulator"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", "", "path to simulator environment file")
	flag.Parse()

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "simulator failed:", err)
		os.Exit(1)
	}
}

// serverResult identifies which server stopped and why.
type serverResult struct {
	name string
	err  error
}

func run(configPath string) error {
	if configPath == "" {
		return errors.New("-config is required")
	}

	simulatorConfig, err := simulator.LoadConfig(configPath)
	if err != nil {
		return err
	}

	generator, err := simulator.NewCommandChecksumGenerator(simulatorConfig)
	if err != nil {
		return err
	}

	// Bind both listeners before starting either server. This avoids
	// advertising gRPC when its listener could not be created.
	httpListener, err := net.Listen("tcp", simulatorConfig.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen for simulator HTTP requests: %w", err)
	}

	var grpcListener net.Listener
	if simulatorConfig.GRPCListenAddr != "" {
		grpcListener, err = net.Listen("tcp", simulatorConfig.GRPCListenAddr)
		if err != nil {
			return errors.Join(
				fmt.Errorf("listen for simulator gRPC requests: %w", err),
				httpListener.Close(),
			)
		}
	}

	httpServer := &http.Server{
		Addr:              simulatorConfig.HTTPAddr,
		Handler:           simulator.NewHandler(simulatorConfig, generator),
		ReadHeaderTimeout: simulatorConfig.ReadHeaderTimeout,
	}

	var grpcServer *grpc.Server
	if grpcListener != nil {
		grpcServer = grpc.NewServer()

		devicev1.RegisterDeviceDiagnosticsServer(
			grpcServer,
			simulator.NewGRPCDiagnosticsServer(simulatorConfig, generator),
		)
	}

	signalContext, stopSignalHandling := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignalHandling()

	// Each server reports once. The buffer lets servers finish even if
	// cleanup has already reached its deadline.
	serverResults := make(chan serverResult, 2)
	serverCount := 1

	go func() {
		serverResults <- serverResult{
			name: "HTTP",
			err:  httpServer.Serve(httpListener),
		}
	}()

	slog.Info("simulator HTTP server listening", "address", httpListener.Addr())

	if grpcServer != nil {
		serverCount++

		go func() {
			serverResults <- serverResult{
				name: "gRPC",
				err:  grpcServer.Serve(grpcListener),
			}
		}()

		slog.Info("simulator gRPC server listening", "address", grpcListener.Addr())
	}

	var runError error
	receivedResults := 0

	select {
	case <-signalContext.Done():
		slog.Info("simulator shutdown requested")

		// Restore default signal behavior during cleanup.
		stopSignalHandling()

	case result := <-serverResults:
		receivedResults++

		// A server stopping before shutdown is unexpected, even if its
		// Serve method returned nil.
		if result.err != nil {
			runError = fmt.Errorf("%s server stopped: %w", result.name, result.err)
		} else {
			runError = fmt.Errorf("%s server stopped unexpectedly", result.name)
		}
	}

	shutdownError := shutdownSimulator(
		simulatorConfig,
		httpServer,
		grpcServer,
		serverResults,
		serverCount-receivedResults,
	)

	return errors.Join(runError, shutdownError)
}

func shutdownSimulator(
	simulatorConfig simulator.Config,
	httpServer *http.Server,
	grpcServer *grpc.Server,
	serverResults <-chan serverResult,
	remainingResults int,
) error {
	shutdownContext, cancelShutdown := context.WithTimeout(
		context.Background(),
		simulatorConfig.ShutdownTimeout,
	)
	defer cancelShutdown()

	var shutdownErrors []error

	// Start gRPC graceful shutdown before waiting for HTTP, so both servers
	// can drain requests within the same timeout budget.
	grpcStopped := make(chan struct{})

	if grpcServer != nil {
		go func() {
			grpcServer.GracefulStop()
			close(grpcStopped)
		}()
	} else {
		close(grpcStopped)
	}

	if err := httpServer.Shutdown(shutdownContext); err != nil {
		shutdownErrors = append(
			shutdownErrors,
			fmt.Errorf("shutdown simulator HTTP server: %w", err),
		)

		if closeError := httpServer.Close(); closeError != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf("force-close simulator HTTP server: %w", closeError),
			)
		}
	}

	select {
	case <-grpcStopped:
		// The gRPC server stopped gracefully, or was not enabled.

	case <-shutdownContext.Done():
		if grpcServer != nil {
			grpcServer.Stop()
		}

		shutdownErrors = append(
			shutdownErrors,
			fmt.Errorf("shutdown simulator gRPC server: %w", shutdownContext.Err()),
		)
	}

	// Collect the remaining Serve results so unexpected server errors are
	// not silently lost during shutdown.
	for remainingResults > 0 {
		select {
		case result := <-serverResults:
			remainingResults--

			if result.err != nil &&
				!(result.name == "HTTP" && errors.Is(result.err, http.ErrServerClosed)) &&
				!(result.name == "gRPC" && errors.Is(result.err, grpc.ErrServerStopped)) {
				shutdownErrors = append(
					shutdownErrors,
					fmt.Errorf("%s server stopped: %w", result.name, result.err),
				)
			}

		case <-shutdownContext.Done():
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf("wait for simulator servers: %w", shutdownContext.Err()),
			)
			return errors.Join(shutdownErrors...)
		}
	}

	return errors.Join(shutdownErrors...)
}
