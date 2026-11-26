-- +goose Up
CREATE TABLE device_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id),
    checked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reachability_status TEXT NOT NULL,
    consecutive_failures INTEGER NOT NULL,
    diagnostics_complete BOOLEAN NOT NULL,
    failure_category TEXT,
    device_reported_status TEXT,
    hardware_version TEXT,
    software_version TEXT,
    firmware_version TEXT,
    checksum TEXT
);

CREATE INDEX device_status_history_device_checked_at_idx
    ON device_status_history (device_id, checked_at DESC);

CREATE INDEX device_status_history_checked_at_idx
    ON device_status_history (checked_at);


-- +goose Down
DROP TABLE device_status_history;