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

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/simulator"
)

func main() {
	configPath := flag.String("config", "", "path to simulator environment file")
	flag.Parse()

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "simulator failed:", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	if configPath == "" {
		return errors.New("-config is required")
	}

	cfg, err := simulator.LoadConfig(configPath)
	if err != nil {
		return err
	}

	generator, err := simulator.NewCommandChecksumGenerator(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           simulator.NewHandler(cfg, generator),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen for simulator requests: %w", err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	slog.Info("simulator listening", "address", listener.Addr().String())

	select {
	case err := <-serverErrors:
		return fmt.Errorf("serve simulator: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Join(
			fmt.Errorf("shutdown simulator: %w", err),
			server.Close(),
		)
	}

	if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve simulator: %w", err)
	}

	return nil
}