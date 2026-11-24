package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/logger"
	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/testutils"
)

func decodeEntry(t *testing.T, output *bytes.Buffer) map[string]any {
	t.Helper()

	var entry map[string]any

	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry %q: %v", output.String(), err)
	}

	if entry == nil {
		t.Fatal("expected a JSON object")
	}

	return entry
}

func TestNewLocalLogger(t *testing.T) {
	testutils.InitConfig(t, testutils.ValidEnvFile)

	t.Run("successfully creates local development logger", func(t *testing.T) {
		var output bytes.Buffer

		testLogger, err := logger.New(&output)
		if err != nil {
			t.Fatalf("create logger: %v", err)
		}

		tests := []struct {
			name      string
			log       func(string, ...any) // log function signature
			shouldLog bool
		}{
			{name: "debug", log: testLogger.Debug, shouldLog: false},
			{name: "info", log: testLogger.Info, shouldLog: true},
			{name: "error", log: testLogger.Error, shouldLog: true},
			{name: "warn", log: testLogger.Warn, shouldLog: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				output.Reset()
				tt.log("test message")
				if tt.shouldLog && output.Len() == 0 {
					t.Errorf("expected log for %s", tt.name)
				}
				if !tt.shouldLog && output.Len() > 0 {
					t.Errorf("unexpected log for %s", tt.name)
				}

			})
		}
	})

	t.Run("logs nested object", func(t *testing.T) {
		var output bytes.Buffer

		testLogger, err := logger.New(&output)
		if err != nil {
			t.Fatalf("create logger: %v", err)
		}

		testLogger.Info("test message",
			slog.Group("nested", slog.String("foo", "bar")),
		)

		// strip ANSI codes for color added by tint; gets plain text for testing
		ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		got := ansi.ReplaceAllString(output.String(), "")

		for _, want := range []string{"test message", "nested.foo=bar"} {
			if !strings.Contains(got, want) {
				t.Errorf("output %q does not contain %q", output.String(), want)
			}
		}
	})
}

func TestNewJSONLogger(t *testing.T) {
	enfFile := strings.Replace(
		testutils.ValidEnvFile,
		"LOG_FORMAT=local\n",
		"LOG_FORMAT=json\n",
		1,
	)
	testutils.InitConfig(t, enfFile)

	t.Run("creates json logger", func(t *testing.T) {
		var output bytes.Buffer

		testLogger, err := logger.New(&output)
		if err != nil {
			t.Fatalf("create logger: %v", err)
		}

		testLogger.Debug("filtered debug")
		if output.Len() != 0 {
			t.Fatalf("logs below INFO were not filtered: %s", output.String())
		}

		testLogger.Info("test", slog.Group("nested", slog.String("foo", "bar")))

		entry := decodeEntry(t, &output)

		if msg, ok := entry["msg"]; !ok || msg != "test" {
			t.Errorf("msg not logged, got = %v", entry["msg"])
		}

		if level, ok := entry["level"]; !ok || level != "INFO" {
			t.Errorf("level not logged, got = %v", entry["level"])
		}

		if _, ok := entry["time"]; !ok {
			t.Errorf("time not logged, got = %v", entry["time"])
		}

		if _, ok := entry["nested"]; !ok {
			t.Errorf("nested not logged, got = %v", entry["nested"])
		}

		nestedObj, ok := entry["nested"].(map[string]any)
		if !ok {
			t.Fatalf("nested must be an object, got %T", entry["nested"])
		}

		if got := nestedObj["foo"]; got != "bar" {
			t.Errorf("nested.foo = %v, want bar", got)
		}
	})
}
