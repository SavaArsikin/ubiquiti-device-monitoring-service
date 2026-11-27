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
	if _, err := config.Init(".env.monitor"); err != nil {
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

	var schedulerFinished <-chan struct{}
	var cancelScheduler context.CancelFunc

	// Register cleanup before migrations and service construction so startup
	// failures also close the database. These variables are read on exit.
	defer func() {
		shutdownError := shutdownMonitor(
			databasePool,
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

	// Setup REST client for probes, no redirects
	restClient := probe.NewRESTClient(&http.Client{
		CheckRedirect: func(
			request *http.Request,
			previousRequests []*http.Request,
		) error {
			return http.ErrUseLastResponse
		},
	})

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
		restClient,
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

	// Keep scheduler cancellation separate from signal detection.
	// Deferred cleanup owns cancellation and the subsequent timed wait.
	schedulerContext, stopScheduler := context.WithCancel(context.Background())
	cancelScheduler = stopScheduler

	schedulerDone := make(chan struct{})
	schedulerFinished = schedulerDone

	var schedulerError error

	go func() {
		// Closing this channel signals that Run and its workers have finished.
		// Read schedulerError only after receiving from this channel.
		defer close(schedulerDone)
		schedulerError = monitoringScheduler.Run(schedulerContext)
	}()

	select {
	case <-signalContext.Done():
		slog.Info("monitor shutdown requested")

		// A second Ctrl+C or SIGTERM can force termination during cleanup.
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
	cancelScheduler context.CancelFunc,
	schedulerFinished <-chan struct{},
) error {
	applicationConfig := config.Get()

	// Shutdown needs its own deadline because the running context will
	// be canceled. This budget covers both workers and database cleanup.
	shutdownContext, cancelShutdown := context.WithTimeout(
		context.Background(),
		applicationConfig.ShutdownTimeout,
	)
	defer cancelShutdown()

	// cancel the scheduler if nil means it wasn't started
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
	}()

	select {
	case cleanupError := <-cleanupFinished:
		if cleanupError != nil {
			return cleanupError
		}

		slog.Info("monitor cleanup completed")
		return nil

	case <-shutdownContext.Done():
		return fmt.Errorf(
			"monitor cleanup exceeded %s: %w",
			applicationConfig.ShutdownTimeout,
			shutdownContext.Err(),
		)
	}
}
