-- Generic integration module: workspace-level connections, per-user
-- credentials, and project bindings for issue / knowledge sync.

CREATE TABLE IF NOT EXISTS integration_connection (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL,
    provider           TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    name               TEXT NOT NULL,
    base_url           TEXT,
    config             JSONB NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    sync_enabled_at    TIMESTAMPTZ,
    created_by_user_id UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, workspace_id),
    UNIQUE (workspace_id, provider, name)
);

CREATE TABLE IF NOT EXISTS integration_user_account (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id         UUID NOT NULL,
    connection_id        UUID NOT NULL,
    user_id              UUID NOT NULL,
    account_key          TEXT NOT NULL,
    account_name         TEXT NOT NULL,
    external_user_id     TEXT,
    external_username    TEXT,
    credential_encrypted BYTEA,
    scopes               JSONB NOT NULL DEFAULT '[]',
    config               JSONB NOT NULL DEFAULT '{}',
    status               TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled', 'error')),
    sync_enabled_at      TIMESTAMPTZ,
    expires_at           TIMESTAMPTZ,
    last_used_at         TIMESTAMPTZ,
    last_error           TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS integration_project_binding (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id              UUID NOT NULL,
    project_id                UUID NOT NULL,
    connection_id             UUID NOT NULL,
    external_ref              JSONB NOT NULL DEFAULT '{}',
    inbound_enabled_at        TIMESTAMPTZ,
    outbound_enabled_at       TIMESTAMPTZ,
    issue_sync_enabled_at     TIMESTAMPTZ,
    knowledge_sync_enabled_at TIMESTAMPTZ,
    created_by_user_id        UUID,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, connection_id)
);

-- Existing fork databases may already have the former 122 migration. Remove
-- its database-level relationships so both fresh and upgraded installs follow
-- the current application-layer cleanup policy.
ALTER TABLE integration_connection
    DROP CONSTRAINT IF EXISTS integration_connection_workspace_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_connection_created_by_user_id_fkey;

ALTER TABLE integration_user_account
    DROP CONSTRAINT IF EXISTS integration_user_account_connection_fk,
    DROP CONSTRAINT IF EXISTS integration_user_account_member_fk;

ALTER TABLE integration_project_binding
    DROP CONSTRAINT IF EXISTS integration_project_binding_workspace_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_project_binding_project_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_project_binding_created_by_user_id_fkey,
    DROP CONSTRAINT IF EXISTS integration_project_binding_connection_fk;
