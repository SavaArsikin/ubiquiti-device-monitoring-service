package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	LogFormat           string        `env:"LOG_FORMAT,required,notEmpty"`
	LogLevel            string        `env:"LOG_LEVEL,required,notEmpty"`
	DatabaseURL         string        `env:"DATABASE_URL,required,notEmpty"`
	HTTPAddr            string        `env:"HTTP_ADDR,required,notEmpty"`
	GRPCAddr            string        `env:"GRPC_ADDR,required,notEmpty"`
	APIUsername         string        `env:"API_USERNAME,required,notEmpty"`
	APIPassword         string        `env:"API_PASSWORD,required,notEmpty"`
	PollInterval        time.Duration `env:"POLL_INTERVAL,required,notEmpty"`
	ProbeTimeout        time.Duration `env:"PROBE_TIMEOUT,required,notEmpty"`
	RetryDelay          time.Duration `env:"RETRY_DELAY,required,notEmpty"`
	MaxRetries          int           `env:"MAX_RETRIES,required"`
	FailureThreshold    int           `env:"FAILURE_THRESHOLD,required"`
	HistoryRetention    time.Duration `env:"HISTORY_RETENTION,required,notEmpty"`
	MaxConcurrentProbes int           `env:"MAX_CONCURRENT_PROBES,required"`
	// LogColor            bool          `env:"LOG_COLOR,required"`
}

// global config should be immutable after init
var globalConfig *Config

func Init(path string) (Config, error) {
	// we are reading .env file because we do not want to contaminate config with potential residual values
	// if any error occurs, we clear globalConfig to avoid using partial config
	values, err := godotenv.Read(path)
	if err != nil {
		globalConfig = nil
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	err = env.ParseWithOptions(&cfg, env.Options{
		Environment:     values,
		RequiredIfNoDef: true,
	})
	if err != nil {
		globalConfig = nil
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		globalConfig = nil
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	globalConfig = &cfg

	return cfg, nil
}

func Get() Config {
	if globalConfig == nil {
		panic("Config not initialized. Call Init() first.")
	}
	return *globalConfig
}

func validateListenAddr(name, addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return fmt.Errorf("%s must be a host:port address", name)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must have a port between 1 and 65535", name)
	}
	return nil
}

func (cfg Config) Validate() error {
	var problems []error

	switch cfg.LogFormat {
	case "local", "json":
	default:
		problems = append(problems, errors.New("LOG_FORMAT must be local or json"))
	}

	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems,
			errors.New("LOG_LEVEL must be debug, info, warn, or error"))
	}

	dbUrl, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		problems = append(problems, errors.New("invalid DATABASE_URL"))
	} else {
		switch dbUrl.Scheme {
		case "postgres", "postgresql":
		default:
			problems = append(problems, errors.New("DATABASE_URL must use the postgres or postgresql scheme"))
		}

		if dbUrl.Hostname() == "" {
			problems = append(problems, errors.New("DATABASE_URL must have a valid hostname"))
		}

		if dbUrl.Path == "" || dbUrl.Path == "/" {
			problems = append(problems, errors.New("DATABASE_URL must have a valid database name in the path"))
		}
	}

	if err := validateListenAddr("HTTP_ADDR", cfg.HTTPAddr); err != nil {
		problems = append(problems, err)
	}
	if err := validateListenAddr("GRPC_ADDR", cfg.GRPCAddr); err != nil {
		problems = append(problems, err)
	}

	if cfg.PollInterval <= 0 {
		problems = append(problems, errors.New("POLL_INTERVAL must be positive"))
	}
	if cfg.ProbeTimeout <= 0 {
		problems = append(problems, errors.New("PROBE_TIMEOUT must be positive"))
	}
	if cfg.RetryDelay <= 0 {
		problems = append(problems, errors.New("RETRY_DELAY must be positive"))
	}
	if cfg.MaxRetries < 0 {
		problems = append(problems, errors.New("MAX_RETRIES must be zero or greater"))
	}
	if cfg.FailureThreshold < 1 {
		problems = append(problems, errors.New("FAILURE_THRESHOLD must be at least 1"))
	}
	if cfg.HistoryRetention <= 0 {
		problems = append(problems, errors.New("HISTORY_RETENTION must be positive"))
	}
	if cfg.MaxConcurrentProbes < 1 {
		problems = append(problems, errors.New("MAX_CONCURRENT_PROBES must be at least 1"))
	}

	return errors.Join(problems...)
}
