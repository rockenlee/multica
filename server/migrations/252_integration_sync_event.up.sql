-- Audit trail for integration sync/config events. This is append-only so the
-- integrations can report recent health without depending on external
-- systems being reachable at page-render time.

CREATE TABLE IF NOT EXISTS integration_sync_event (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL,
    connection_id UUID,
    provider      TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    direction     TEXT NOT NULL
        CHECK (direction IN ('inbound', 'outbound', 'internal')),
    object_type   TEXT NOT NULL,
    object_id     TEXT,
    external_id   TEXT,
    external_url  TEXT,
    project_id    UUID,
    status        TEXT NOT NULL
        CHECK (status IN ('success', 'warning', 'error', 'skipped')),
    message       TEXT,
    error         TEXT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE integration_sync_event
    DROP CONSTRAINT IF EXISTS integration_sync_event_workspace_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_sync_event_connection_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_sync_event_project_id_fkey;
