-- +goose Up
CREATE TABLE device_statuses (
    device_id UUID PRIMARY KEY REFERENCES devices(id),
    reachability_status TEXT NOT NULL DEFAULT 'unknown',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    diagnostics_complete BOOLEAN NOT NULL DEFAULT FALSE,
    last_checked_at TIMESTAMPTZ,
    last_reachable_at TIMESTAMPTZ,
    last_failure_category TEXT,
    device_reported_status TEXT,
    hardware_version TEXT,
    software_version TEXT,
    firmware_version TEXT,
    checksum TEXT,
    diagnostics_observed_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE device_statuses;