-- +goose Up
CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    digest text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id text PRIMARY KEY,
    name text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS resources (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    kind text NOT NULL,
    id text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    source_system text,
    source_sequence bigint,
    PRIMARY KEY (project_id, kind, id)
);
CREATE INDEX IF NOT EXISTS resources_list_idx
    ON resources (project_id, kind, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS events (
    cursor bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id text NOT NULL UNIQUE,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    event_type text NOT NULL,
    subject_type text NOT NULL,
    subject_id text NOT NULL,
    actor_id text NOT NULL,
    command_id text,
    correlation_id text,
    causation_id text,
    occurred_at timestamptz NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    source_system text,
    source_event_id text,
    source_sequence bigint,
    payload jsonb NOT NULL
);
CREATE INDEX IF NOT EXISTS events_project_cursor_idx ON events (project_id, cursor);
CREATE INDEX IF NOT EXISTS events_subject_idx ON events (project_id, subject_type, subject_id, cursor);
CREATE UNIQUE INDEX IF NOT EXISTS events_source_identity_idx
    ON events(project_id,source_system,source_event_id)
    WHERE source_system IS NOT NULL AND source_event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS event_outbox (
    event_id text PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    cursor bigint NOT NULL,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    event jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX IF NOT EXISTS event_outbox_pending_idx
    ON event_outbox (available_at, cursor) WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS run_control_outbox (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    run_id text NOT NULL,
    actor_id text NOT NULL,
    command_id text NOT NULL UNIQUE,
    action text NOT NULL CHECK (action IN ('pause','resume','cancel','message','redirect')),
    message text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','executing','executed','failed')),
    attempts integer NOT NULL DEFAULT 0,
    requested_at timestamptz NOT NULL,
    claimed_at timestamptz,
    completed_at timestamptz,
    last_error text
);
CREATE INDEX IF NOT EXISTS run_control_outbox_dispatch_idx
    ON run_control_outbox (status, requested_at);

CREATE TABLE IF NOT EXISTS command_results (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    idempotency_key text NOT NULL,
    fingerprint bytea NOT NULL,
    command_id text NOT NULL UNIQUE,
    resource jsonb NOT NULL,
    event jsonb NOT NULL,
    accepted_at timestamptz NOT NULL,
    PRIMARY KEY (project_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS audit_records (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    actor_id text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    command_id text,
    outcome text NOT NULL,
    remote_addr text,
    details jsonb,
    occurred_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_project_time_idx ON audit_records (project_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION agentroom_reject_history_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'append-only history cannot be updated or deleted'
        USING ERRCODE = '42501';
END
$$;

CREATE TRIGGER events_append_only
    BEFORE UPDATE OR DELETE ON events
    FOR EACH ROW EXECUTE FUNCTION agentroom_reject_history_mutation();
CREATE TRIGGER commands_append_only
    BEFORE UPDATE OR DELETE ON command_results
    FOR EACH ROW EXECUTE FUNCTION agentroom_reject_history_mutation();
CREATE TRIGGER audit_append_only
    BEFORE UPDATE OR DELETE ON audit_records
    FOR EACH ROW EXECUTE FUNCTION agentroom_reject_history_mutation();

CREATE TABLE IF NOT EXISTS user_accounts (
    id text PRIMARY KEY,
    username text NOT NULL UNIQUE,
    password_hash text,
    display_name text NOT NULL,
    disabled boolean NOT NULL DEFAULT false,
    capabilities text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_memberships (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES user_accounts(id) ON DELETE CASCADE,
    capabilities text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS service_tokens (
    token_hash bytea PRIMARY KEY,
    id text NOT NULL UNIQUE,
    name text NOT NULL,
    actor_id text NOT NULL,
    project_ids text[] NOT NULL,
    capabilities text[] NOT NULL,
    expires_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS web_sessions (
    token_hash bytea PRIMARY KEY,
    user_id text NOT NULL REFERENCES user_accounts(id),
    csrf_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    oidc_subject text
);
CREATE INDEX IF NOT EXISTS web_sessions_expiry_idx ON web_sessions (expires_at);

CREATE TABLE IF NOT EXISTS oidc_states (
    state_hash bytea PRIMARY KEY,
    browser_hash bytea NOT NULL,
    nonce_hash bytea NOT NULL,
    verifier_hash bytea NOT NULL,
    verifier_ciphertext bytea NOT NULL,
    return_to text NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS brief_cursors (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    actor_id text NOT NULL,
    last_cursor bigint NOT NULL DEFAULT 0 CHECK (last_cursor >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, actor_id)
);

CREATE TABLE IF NOT EXISTS brief_acknowledgements (
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    actor_id text NOT NULL,
    idempotency_key text NOT NULL,
    command_id text NOT NULL UNIQUE,
    expected_cursor bigint NOT NULL,
    through_cursor bigint NOT NULL,
    accepted_at timestamptz NOT NULL,
    PRIMARY KEY (project_id, actor_id, idempotency_key)
);

-- The operator is expected to create one or more projects explicitly. No
-- production sample or synthetic domain data is inserted by migrations.

-- +goose Down
DROP TABLE IF EXISTS brief_acknowledgements;
DROP TABLE IF EXISTS brief_cursors;
DROP TABLE IF EXISTS oidc_states;
DROP TABLE IF EXISTS web_sessions;
DROP TABLE IF EXISTS service_tokens;
DROP TABLE IF EXISTS project_memberships;
DROP TABLE IF EXISTS user_accounts;
DROP FUNCTION IF EXISTS agentroom_reject_history_mutation();
DROP TABLE IF EXISTS audit_records;
DROP TABLE IF EXISTS command_results;
DROP TABLE IF EXISTS run_control_outbox;
DROP TABLE IF EXISTS event_outbox;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS resources;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS schema_migrations;
