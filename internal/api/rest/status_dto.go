package rest

import (
	"time"

	"github.com/SavaArsikin/ubiquiti-device-monitoring-service/internal/devicestatus"
)

type deviceStatusResponse struct {
	DeviceID              string                          `json:"device_id"`
	ReachabilityStatus    devicestatus.ReachabilityStatus `json:"reachability_status"`
	ConsecutiveFailures   int                             `json:"consecutive_failures"`
	DiagnosticsComplete   bool                            `json:"diagnostics_complete"`
	LastCheckedAt         *time.Time                      `json:"last_checked_at"`
	LastReachableAt       *time.Time                      `json:"last_reachable_at"`
	LastFailureCategory   *string                         `json:"last_failure_category"`
	DeviceReportedStatus  *string                         `json:"device_reported_status"`
	HardwareVersion       *string                         `json:"hardware_version"`
	SoftwareVersion       *string                         `json:"software_version"`
	FirmwareVersion       *string                         `json:"firmware_version"`
	Checksum              *string                         `json:"checksum"`
	DiagnosticsObservedAt *time.Time                      `json:"diagnostics_observed_at"`
}
