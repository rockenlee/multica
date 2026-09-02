-- GitLab issue IIDs are only unique inside a GitLab project. Scope external
-- mirror idempotency and tombstones by an optional source_scope so one Multica
-- workspace can sync multiple GitLab projects without source_id collisions.

ALTER TABLE integration_issue_tombstone
    ADD COLUMN IF NOT EXISTS source_scope TEXT NOT NULL DEFAULT '';

ALTER TABLE integration_issue_tombstone
    DROP CONSTRAINT IF EXISTS integration_issue_tombstone_pkey;

ALTER TABLE integration_issue_tombstone
    ADD CONSTRAINT integration_issue_tombstone_pkey
    PRIMARY KEY (workspace_id, provider, source_scope, source_id);
