-- Generic integration module: workspace-level connections, per-user
-- credentials, and project bindings for issue / knowledge sync.

CREATE TABLE integration_connection (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    provider           TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    name               TEXT NOT NULL,
    base_url           TEXT,
    config             JSONB NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    sync_enabled_at    TIMESTAMPTZ,
    created_by_user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, workspace_id),
    UNIQUE (workspace_id, provider, name)
);

CREATE INDEX idx_integration_connection_workspace
    ON integration_connection(workspace_id, provider);

CREATE TABLE integration_user_account (
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
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, user_id, account_key),
    CONSTRAINT integration_user_account_connection_fk
        FOREIGN KEY (connection_id, workspace_id)
        REFERENCES integration_connection(id, workspace_id)
        ON DELETE CASCADE,
    CONSTRAINT integration_user_account_member_fk
        FOREIGN KEY (workspace_id, user_id)
        REFERENCES member(workspace_id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_integration_user_account_user
    ON integration_user_account(user_id, workspace_id);

CREATE INDEX idx_integration_user_account_connection
    ON integration_user_account(connection_id, user_id);

CREATE TABLE integration_project_binding (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id              UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    project_id                UUID NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    connection_id             UUID NOT NULL,
    external_ref              JSONB NOT NULL DEFAULT '{}',
    inbound_enabled_at        TIMESTAMPTZ,
    outbound_enabled_at       TIMESTAMPTZ,
    issue_sync_enabled_at     TIMESTAMPTZ,
    knowledge_sync_enabled_at TIMESTAMPTZ,
    created_by_user_id        UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, connection_id),
    CONSTRAINT integration_project_binding_connection_fk
        FOREIGN KEY (connection_id, workspace_id)
        REFERENCES integration_connection(id, workspace_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_integration_project_binding_workspace
    ON integration_project_binding(workspace_id, project_id);
