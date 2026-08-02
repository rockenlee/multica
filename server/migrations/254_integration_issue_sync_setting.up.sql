-- Per-user issue sync preferences for generic integrations. Workspace
-- connections and project resources define what can be reached; this table
-- records whether a member wants a provider's issues to flow into/out of My
-- Issues.

CREATE TABLE IF NOT EXISTS integration_issue_sync_setting (
    workspace_id         UUID NOT NULL,
    user_id              UUID NOT NULL,
    provider             TEXT NOT NULL
        CHECK (provider IN ('gitlab', 'zentao', 'feishu')),
    inbound_enabled_at   TIMESTAMPTZ,
    outbound_enabled_at  TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id, provider)
);

ALTER TABLE integration_issue_sync_setting
    DROP CONSTRAINT IF EXISTS integration_issue_sync_setting_member_fk;
