package devicestatus

import (
	"errors"
	"time"
)

var ErrDeviceStatusNotFound = errors.New("device status not found")

type ReachabilityStatus string

const (
	ReachabilityUnknown     ReachabilityStatus = "unknown"
	ReachabilityReachable   ReachabilityStatus = "reachable"
	ReachabilityDegraded    ReachabilityStatus = "degraded"
	ReachabilityUnreachable ReachabilityStatus = "unreachable"
)

type DeviceStatus struct {
	DeviceID              string
	ReachabilityStatus    ReachabilityStatus
	ConsecutiveFailures   int
	DiagnosticsComplete   bool
	LastCheckedAt         *time.Time
	LastReachableAt       *time.Time
	LastFailureCategory   *string
	DeviceReportedStatus  *string
	HardwareVersion       *string
	SoftwareVersion       *string
	FirmwareVersion       *string
	Checksum              *string
	DiagnosticsObservedAt *time.Time
}

type DeviceStatusHistory struct {
	ID                   string
	DeviceID             string
	CheckedAt            time.Time
	ReachabilityStatus   ReachabilityStatus
	ConsecutiveFailures  int
	DiagnosticsComplete  bool
	FailureCategory      *string
	DeviceReportedStatus *string
	HardwareVersion      *string
	SoftwareVersion      *string
	FirmwareVersion      *string
	Checksum             *string
}
