package logger

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/lmittmann/tint"
)

var logLevelMap = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

func New(output io.Writer) (*slog.Logger, error) {
	cfg := config.Get()

	var logger *slog.Logger
	if _, ok := logLevelMap[cfg.LogLevel]; !ok {
		return nil, fmt.Errorf("invalid log level: %s", cfg.LogLevel)
	}
	logLevel := logLevelMap[cfg.LogLevel]

	switch cfg.LogFormat {
	case "json":
		handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
			Level:     logLevel,
			AddSource: true,
		})
		logger = slog.New(handler)

	case "local":
		handler := tint.NewTextHandler(output, &tint.Options{
			Level:      logLevel,
			AddSource:  true,
			TimeFormat: time.RFC3339,
		})
		logger = slog.New(handler)

	default:
		return nil, fmt.Errorf("unsupported log format: %s", cfg.LogFormat)
	}

	return logger, nil
}
