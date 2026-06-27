-- Per-user issue sync preferences for generic integrations. Workspace
-- connections and project resources define what can be reached; this table
-- records whether a member wants a provider's issues to flow into/out of My
-- Issues.

CREATE TABLE integration_issue_sync_setting (
    workspace_id         UUID NOT NULL,
    user_id              UUID NOT NULL,
    provider             TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    inbound_enabled_at   TIMESTAMPTZ,
    outbound_enabled_at  TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id, provider),
    CONSTRAINT integration_issue_sync_setting_member_fk
        FOREIGN KEY (workspace_id, user_id)
        REFERENCES member(workspace_id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_integration_issue_sync_setting_user
    ON integration_issue_sync_setting(user_id, workspace_id);

-- Source metadata is the idempotency key for external issue mirrors.
CREATE UNIQUE INDEX idx_issue_external_source_unique
    ON issue (workspace_id, (metadata->>'source_system'), (metadata->>'source_id'))
    WHERE metadata ? 'source_system'
      AND metadata ? 'source_id'
      AND COALESCE(metadata->>'source_system', '') <> ''
      AND COALESCE(metadata->>'source_id', '') <> '';
