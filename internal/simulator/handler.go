package simulator

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/deviceprotocol"
)

type endpointBehavior struct {
	mode     string
	failures int
	delay    time.Duration
	status   int
	requests atomic.Uint64
}

func (b *endpointBehavior) allow(
	w http.ResponseWriter,
	r *http.Request,
) bool {
	requestNumber := b.requests.Add(1)

	switch b.mode {
	case "error":
		http.Error(w, "simulated device error", b.status)
		return false

	case "flaky":
		if requestNumber <= uint64(b.failures) {
			http.Error(w, "simulated temporary failure", b.status)
			return false
		}

	case "slow":
		if err := wait(r.Context(), b.delay); err != nil {
			slog.Debug("simulated response canceled", "error", err)
			return false
		}
	}

	return true
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NewHandler(cfg Config, generator checksumGenerator) http.Handler {
	healthBehavior := &endpointBehavior{
		mode:     cfg.HealthBehavior,
		failures: cfg.FailureCount,
		delay:    cfg.ResponseDelay,
		status:   cfg.ErrorStatus,
	}
	diagnosticsBehavior := &endpointBehavior{
		mode:     cfg.DiagnosticsBehavior,
		failures: cfg.FailureCount,
		delay:    cfg.ResponseDelay,
		status:   cfg.ErrorStatus,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if !healthBehavior.allow(w, r) {
			return
		}

		writeJSON(w, deviceprotocol.Capabilities{
			RESTDiagnosticsPath: "/diagnostics",
		})
	})

	mux.HandleFunc("GET /diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if !diagnosticsBehavior.allow(w, r) {
			return
		}

		diagnostics := deviceprotocol.Diagnostics{
			HardwareVersion: cfg.HardwareVersion,
			SoftwareVersion: cfg.SoftwareVersion,
			FirmwareVersion: cfg.FirmwareVersion,
			Status:          cfg.DeviceStatus,
		}

		if cfg.DiagnosticsBehavior == "incomplete" {
			diagnostics.FirmwareVersion = ""
			diagnostics.FailureCategory = "invalid_diagnostics"
			writeJSON(w, diagnostics)
			return
		}

		checksum, err := generator.Generate(r.Context(), diagnostics)
		if err != nil {
			slog.Warn("checksum unavailable", "error", err)
			diagnostics.FailureCategory = "checksum_unavailable"
		} else {
			diagnostics.Checksum = &checksum
		}

		writeJSON(w, diagnostics)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		slog.Error("encode simulator response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		slog.Error("write simulator response", "error", err)
	}
}