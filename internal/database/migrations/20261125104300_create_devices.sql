-- +goose Up
CREATE TABLE devices (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    health_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE devices;