package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/config"
)

const ValidEnvFile = `
LOG_FORMAT=local
LOG_LEVEL=info
DATABASE_URL=postgres://admin:admin@127.0.0.1:5432/device_monitoring_service_test?sslmode=disable
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=2
DB_CONN_MAX_LIFETIME=30m
DB_CONNECT_TIMEOUT=5s
DB_MIGRATION_TIMEOUT=30s
HTTP_ADDR=127.0.0.1:8080
API_USERNAME=demo
API_PASSWORD=demo-password
POLL_INTERVAL=10s
PROBE_TIMEOUT=2s
RETRY_DELAY=250ms
MAX_RETRIES=1
FAILURE_THRESHOLD=3
MAX_CONCURRENT_PROBES=10
SHUTDOWN_TIMEOUT=10s
`

// InitConfig writes a complete temporary config file and initializes config.
func InitConfig(t testing.TB, envFile string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env.monitor")

	if err := os.WriteFile(path, []byte(envFile), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := config.Init(path)
	if err != nil {
		t.Fatalf("initialize test config: %v", err)
	}
}
