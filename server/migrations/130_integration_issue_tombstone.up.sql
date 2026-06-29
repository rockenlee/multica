-- Tombstones for user-deleted external mirror issues. When a member deletes a
-- mirrored issue (sourced from GitLab / ZenTao / Feishu), we record the external
-- identity here so the next inbound poll does NOT resurrect it. Keyed the same
-- way as the inbound idempotency index: (workspace_id, provider, source_id).
--
-- A later manual re-sync of the same external item clears its tombstone first
-- (an explicit "I want it back" action), so suppression only blocks automatic
-- inbound recreation.

CREATE TABLE integration_issue_tombstone (
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL,
    source_id     TEXT NOT NULL,
    project_id    UUID,
    source_url    TEXT,
    issue_id      UUID,
    deleted_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, provider, source_id)
);

CREATE INDEX idx_integration_issue_tombstone_project
    ON integration_issue_tombstone(workspace_id, project_id);
