-- +goose Up
CREATE TABLE IF NOT EXISTS source_receipts (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_system text NOT NULL,
    last_received_at timestamptz NOT NULL,
    last_collected_at timestamptz NOT NULL,
    last_event_id text NOT NULL DEFAULT '',
    last_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (project_id, source_system)
);
