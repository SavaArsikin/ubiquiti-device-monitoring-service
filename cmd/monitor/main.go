package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/api/rest"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/database"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/device"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/logger"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/probe"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "monitor service failed:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	applicationConfig, err := config.Init(".env.monitor")
	if err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	applicationLogger, err := logger.New(os.Stdout)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	slog.SetDefault(applicationLogger)

	slog.Info("initialized config and logger")

	databasePool, err := database.Open(context.Background())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Nil values let cleanup handle failures before the server or scheduler
	// have been started.
	var apiServer *http.Server
	var schedulerFinished <-chan struct{}
	var cancelScheduler context.CancelFunc

	defer func() {
		shutdownError := shutdownMonitor(
			databasePool,
			apiServer,
			cancelScheduler,
			schedulerFinished,
		)
		err = errors.Join(err, shutdownError)
	}()

	if err := database.Migrate(context.Background(), databasePool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	deviceRepository := device.NewDeviceRepository(databasePool)
	statusRepository := devicestatus.NewDeviceStatusRepository(databasePool)
	historyRepository := devicestatus.NewDeviceStatusHistoryRepository(databasePool)

	// Do not follow redirects to a different device.
	restClient := probe.NewRESTClient(&http.Client{
		CheckRedirect: func(
			request *http.Request,
			previousRequests []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	})

	grpcClient := probe.NewGRPCClient()
	deviceClient := probe.NewDeviceClient(restClient, grpcClient)

	slog.Info("initialized clients")

	deviceService := service.NewDeviceService(
		databasePool,
		deviceRepository,
		statusRepository,
	)

	monitoringService := service.NewMonitoringService(
		databasePool,
		deviceRepository,
		statusRepository,
		historyRepository,
		deviceClient,
	)

	monitoringScheduler := service.NewMonitoringScheduler(
		deviceService,
		monitoringService,
	)

	apiHandler := rest.NewHandler(
		deviceService,
		monitoringService,
		applicationConfig.APIUsername,
		applicationConfig.APIPassword,
	)

	apiServer = &http.Server{
		Addr:              applicationConfig.HTTPAddr,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	httpServerErrors := make(chan error, 1)
	go func() {
		slog.Info("listening REST API", "address", applicationConfig.HTTPAddr)
		httpServerErrors <- apiServer.ListenAndServe()
	}()

	signalContext, stopSignalHandling := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignalHandling()

	schedulerContext, stopScheduler := context.WithCancel(context.Background())
	cancelScheduler = stopScheduler

	schedulerDone := make(chan struct{})
	schedulerFinished = schedulerDone

	var schedulerError error

	go func() {
		// Closing the channel signals that Run and its workers have finished.
		defer close(schedulerDone)
		schedulerError = monitoringScheduler.Run(schedulerContext)
	}()

	slog.Info("monitor started")

	select {
	case <-signalContext.Done():
		slog.Info("monitor shutdown requested")

		// A second signal can force termination during cleanup.
		stopSignalHandling()
		return nil

	case <-schedulerDone:
		return schedulerError

	case serverError := <-httpServerErrors:
		return fmt.Errorf("REST API server stopped: %w", serverError)
	}
}

func shutdownMonitor(
	databasePool *sql.DB,
	apiServer *http.Server,
	cancelScheduler context.CancelFunc,
	schedulerFinished <-chan struct{},
) error {
	applicationConfig := config.Get()

	// HTTP shutdown, scheduler completion, and database cleanup share
	// one time budget.
	shutdownContext, cancelShutdown := context.WithTimeout(
		context.Background(),
		applicationConfig.ShutdownTimeout,
	)
	defer cancelShutdown()

	// Ask scheduler workers to stop while HTTP requests are draining.
	if cancelScheduler != nil {
		cancelScheduler()
	}

	if apiServer != nil {
		// Shutdown stops accepting requests and waits for active handlers.
		if err := apiServer.Shutdown(shutdownContext); err != nil {
			shutdownError := fmt.Errorf("shut down REST API server: %w", err)

			// Close connections, but do not assume their handlers have exited.
			if closeError := apiServer.Close(); closeError != nil {
				shutdownError = errors.Join(
					shutdownError,
					fmt.Errorf("force-close REST API server: %w", closeError),
				)
			}

			// A handler may still use the pool. Return an error and let
			// the process exit rather than explicitly closing it here.
			return shutdownError
		}
	}

	if schedulerFinished != nil {
		select {
		case <-schedulerFinished:
			// Run and its workers have finished.

		case <-shutdownContext.Done():
			// Workers may still use the pool, so do not close it here.
			return fmt.Errorf(
				"wait for scheduler shutdown: %w",
				shutdownContext.Err(),
			)
		}
	}

	if err := shutdownContext.Err(); err != nil {
		return fmt.Errorf("monitor shutdown deadline: %w", err)
	}

	// DB.Close has no context parameter and can wait for ongoing queries.
	// At this point the HTTP handlers and scheduler have finished.
	databaseClosed := make(chan error, 1)
	go func() {
		databaseClosed <- databasePool.Close()
	}()

	select {
	case err := <-databaseClosed:
		if err != nil {
			return fmt.Errorf("close database connection pool: %w", err)
		}

		slog.Info("monitor cleanup completed")
		return nil

	case <-shutdownContext.Done():
		return fmt.Errorf(
			"close database connection pool exceeded shutdown timeout: %w",
			shutdownContext.Err(),
		)
	}
}
