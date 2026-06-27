-- Audit trail for integration sync/config events. This is append-only so the
-- integrations can report recent health without depending on external
-- systems being reachable at page-render time.

CREATE TABLE integration_sync_event (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    connection_id UUID REFERENCES integration_connection(id) ON DELETE SET NULL,
    provider      TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    direction     TEXT NOT NULL
        CHECK (direction IN ('inbound', 'outbound', 'internal')),
    object_type   TEXT NOT NULL,
    object_id     TEXT,
    external_id   TEXT,
    external_url  TEXT,
    project_id    UUID REFERENCES project(id) ON DELETE SET NULL,
    status        TEXT NOT NULL
        CHECK (status IN ('success', 'warning', 'error', 'skipped')),
    message       TEXT,
    error         TEXT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_integration_sync_event_workspace
    ON integration_sync_event(workspace_id, occurred_at DESC);

CREATE INDEX idx_integration_sync_event_connection
    ON integration_sync_event(connection_id, occurred_at DESC);

CREATE INDEX idx_integration_sync_event_provider_status
    ON integration_sync_event(workspace_id, provider, status, occurred_at DESC);
