package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
	"github.com/google/go-cmp/cmp"
)

func validConfig() config.Config {
	return config.Config{
		LogFormat:           "local",
		LogLevel:            "info",
		DatabaseURL:         "postgres://admin:admin@127.0.0.1:5432/device_monitoring_service_test?sslmode=disable",
		DBMaxOpenConns:      10,
		DBMaxIdleConns:      2,
		DBConnMaxLifetime:   30 * time.Minute,
		DBConnectTimeout:    5 * time.Second,
		DBMigrationTimeout:  30 * time.Second,
		HTTPAddr:            "127.0.0.1:8080",
		GRPCAddr:            "127.0.0.1:9090",
		APIUsername:         "demo",
		APIPassword:         "demo-password",
		PollInterval:        10 * time.Second,
		ProbeTimeout:        2 * time.Second,
		RetryDelay:          250 * time.Millisecond,
		MaxRetries:          1,
		FailureThreshold:    3,
		HistoryRetention:    24 * time.Hour,
		MaxConcurrentProbes: 10,
	}
}

func writeEnvFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env.monitor")
	// temp file for env, write permission for owner only
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func TestInit(t *testing.T) {
	t.Run("successfully loads config", func(t *testing.T) {
		path := writeEnvFile(t, testutils.ValidEnvFile)

		cfg, err := config.Init(path)
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		if diff := cmp.Diff(validConfig(), cfg); diff != "" {
			t.Errorf("parsed config does not match the fixture (-want +got):\n%s", diff)
		}
	})

	t.Run("fails if required env variable is missing", func(t *testing.T) {
		withoutPassword := strings.Replace(
			testutils.ValidEnvFile,
			"API_PASSWORD=demo-password\n",
			"",
			1,
		)
		path := writeEnvFile(t, withoutPassword)

		cfg, err := config.Init(path)

		if cfg != (config.Config{}) {
			t.Fatal("config should not be initialized")
		}

		if err == nil {
			t.Fatal("Init() expected an error for missing API_PASSWORD")
		}

		if err.Error() != `parse config: env: required environment variable "API_PASSWORD" is not set` {
			t.Fatal("Init() returned unexpected error")
		}
	})

	t.Run("fails if env file missing", func(t *testing.T) {
		_, err := config.Init("nonexistent.env")
		if err == nil {
			t.Fatal("expected error for missing env file")
		}
	})

	t.Run("does not include environment variables not in .env file", func(t *testing.T) {
		// set env variable not in .env file
		t.Setenv("API_PASSWORD", "some-other-password")

		path := writeEnvFile(t, testutils.ValidEnvFile)

		cfg, err := config.Init(path)
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		if cfg.APIPassword != validConfig().APIPassword || cfg.APIPassword == "some-other-password" {
			t.Fatal("APIPassword mismatch")
		}
	})

	t.Run("it does not include environment variables even if they are missing from the .env file", func(t *testing.T) {
		// set env variable not in .env file
		t.Setenv("API_PASSWORD", "some-other-password")

		withoutPassword := strings.Replace(
			testutils.ValidEnvFile,
			"API_PASSWORD=demo-password\n",
			"",
			1,
		)
		path := writeEnvFile(t, withoutPassword)

		cfg, err := config.Init(path)

		if cfg != (config.Config{}) {
			t.Fatal("config should not be initialized")
		}

		if err == nil {
			t.Fatal("Init() expected an error for missing API_PASSWORD")
		}

		if err.Error() != `parse config: env: required environment variable "API_PASSWORD" is not set` {
			t.Fatal("Init() returned unexpected error")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*config.Config)
	}{
		{
			name: "unsupported log format",
			change: func(c *config.Config) {
				c.LogFormat = "xml"
			},
		},
		{
			name: "unsupported log level",
			change: func(c *config.Config) {
				c.LogLevel = "trace"
			},
		},
		{
			name: "non-Postgres database URL",
			change: func(c *config.Config) {
				c.DatabaseURL = "http://localhost/monitor"
			},
		},
		{
			name: "invalid DB_URL hostname",
			change: func(c *config.Config) {
				c.DatabaseURL = "postgres://:5432/monitor"
			},
		},
		{
			name: "invalid DB_URL path",
			change: func(c *config.Config) {
				c.DatabaseURL = "postgres://localhost:5432/"
			},
		},
		{
			name: "invalid DB_URL db name",
			change: func(c *config.Config) {
				c.DatabaseURL = "postgres://localhost:5432"
			},
		},
		{
			name: "invalid HTTP address",
			change: func(c *config.Config) {
				c.HTTPAddr = "localhost"
			},
		},
		{
			name: "non-positive poll interval",
			change: func(c *config.Config) {
				c.PollInterval = 0
			},
		},
		{
			name: "negative retry count",
			change: func(c *config.Config) {
				c.MaxRetries = -1
			},
		},
		{
			name: "invalid failure threshold",
			change: func(c *config.Config) {
				c.FailureThreshold = 0
			},
		},
		{
			name: "non-positive history retention",
			change: func(c *config.Config) {
				c.HistoryRetention = 0
			},
		},
		{
			name: "invalid worker count",
			change: func(c *config.Config) {
				c.MaxConcurrentProbes = 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.change(&cfg)

			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() expected an error")
			}
		})
	}
}

func TestGetReturnsCopy(t *testing.T) {
	t.Run("retrieves global config", func(t *testing.T) {
		path := writeEnvFile(t, testutils.ValidEnvFile)

		cfg, err := config.Init(path)
		if err != nil {
			t.Fatalf("Init() error = %v", err)
		}

		globalConfig := config.Get()

		if diff := cmp.Diff(cfg, globalConfig); diff != "" {
			t.Errorf("Initialized config mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("global config should not change when config is changed", func(t *testing.T) {
		previousConfig := config.Get()
		previousConfig.LogLevel = "debug"
		globalConfig := config.Get()
		if diff := cmp.Diff(previousConfig, globalConfig); diff == "" {
			t.Fatal("Global config changed")
		}
	})

	t.Run("it panics if not initialized", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		config.Init("") // force error to reset globalConfig
		config.Get()
	})

}
