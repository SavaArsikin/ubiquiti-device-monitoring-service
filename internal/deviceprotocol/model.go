package deviceprotocol

type Capabilities struct {
	RESTDiagnosticsPath string `json:"rest_diagnostics_path,omitempty"`
	GRPCTarget          string `json:"grpc_target,omitempty"`
}

type Diagnostics struct {
	HardwareVersion string  `json:"hardware_version"`
	SoftwareVersion string  `json:"software_version"`
	FirmwareVersion string  `json:"firmware_version"`
	Status          string  `json:"status"`
	Checksum        *string `json:"checksum"`
	FailureCategory string  `json:"failure_category,omitempty"`
}
