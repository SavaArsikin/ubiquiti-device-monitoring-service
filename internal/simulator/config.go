package simulator

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr          string        `env:"HTTP_ADDR,required,notEmpty"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT,required,notEmpty"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT,required,notEmpty"`

	HardwareVersion string `env:"HARDWARE_VERSION,required,notEmpty"`
	SoftwareVersion string `env:"SOFTWARE_VERSION,required,notEmpty"`
	FirmwareVersion string `env:"FIRMWARE_VERSION,required,notEmpty"`
	DeviceStatus    string `env:"DEVICE_STATUS,required,notEmpty"`

	HealthBehavior      string        `env:"HEALTH_BEHAVIOR,required,notEmpty"`
	DiagnosticsBehavior string        `env:"DIAGNOSTICS_BEHAVIOR,required,notEmpty"`
	ResponseDelay       time.Duration `env:"RESPONSE_DELAY,required,notEmpty"`
	FailureCount        int           `env:"FAILURE_COUNT,required"`
	ErrorStatus         int           `env:"ERROR_STATUS,required,notEmpty"`

	ChecksumBinary  string        `env:"CHECKSUM_BINARY,required,notEmpty"`
	ChecksumArgs    []string      `env:"CHECKSUM_ARGS,required"`
	ChecksumTimeout time.Duration `env:"CHECKSUM_TIMEOUT,required,notEmpty"`
}

func LoadConfig(path string) (Config, error) {
	values, err := godotenv.Read(path)
	if err != nil {
		return Config{}, fmt.Errorf("read simulator config: %w", err)
	}

	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{
		Environment: values,
	}); err != nil {
		return Config{}, fmt.Errorf("parse simulator config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate simulator config: %w", err)
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	var problems []error

	host, portText, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil || host == "" {
		problems = append(problems, errors.New("HTTP_ADDR must be a host:port address"))
	} else {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			problems = append(problems, errors.New("HTTP_ADDR has an invalid port"))
		}
	}

	for name, duration := range map[string]time.Duration{
		"READ_HEADER_TIMEOUT": cfg.ReadHeaderTimeout,
		"SHUTDOWN_TIMEOUT":    cfg.ShutdownTimeout,
		"RESPONSE_DELAY":      cfg.ResponseDelay,
		"CHECKSUM_TIMEOUT":    cfg.ChecksumTimeout,
	} {
		if duration <= 0 {
			problems = append(problems, fmt.Errorf("%s must be positive", name))
		}
	}

	if !validBehavior(cfg.HealthBehavior, false) {
		problems = append(problems, errors.New("invalid HEALTH_BEHAVIOR"))
	}
	if !validBehavior(cfg.DiagnosticsBehavior, true) {
		problems = append(problems, errors.New("invalid DIAGNOSTICS_BEHAVIOR"))
	}
	if cfg.FailureCount < 0 {
		problems = append(problems, errors.New("FAILURE_COUNT must not be negative"))
	}
	if cfg.ErrorStatus < 400 || cfg.ErrorStatus > 599 {
		problems = append(problems, errors.New("ERROR_STATUS must be between 400 and 599"))
	}

	return errors.Join(problems...)
}

func validBehavior(value string, diagnostics bool) bool {
	switch value {
	case "healthy", "slow", "flaky", "error":
		return true
	case "incomplete":
		return diagnostics
	default:
		return false
	}
}
