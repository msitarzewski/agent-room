-- +goose Up
CREATE INDEX events_project_occurred_cursor_idx
    ON events (project_id, occurred_at DESC, cursor DESC);

-- +goose Down
DROP INDEX events_project_occurred_cursor_idx;
