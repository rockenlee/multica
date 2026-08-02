ALTER TABLE integration_issue_tombstone
    DROP CONSTRAINT IF EXISTS integration_issue_tombstone_pkey;

DELETE FROM integration_issue_tombstone t
USING integration_issue_tombstone keep
WHERE t.workspace_id = keep.workspace_id
  AND t.provider = keep.provider
  AND t.source_id = keep.source_id
  AND t.ctid > keep.ctid;

ALTER TABLE integration_issue_tombstone
    ADD CONSTRAINT integration_issue_tombstone_pkey
    PRIMARY KEY (workspace_id, provider, source_id);

ALTER TABLE integration_issue_tombstone
    DROP COLUMN IF EXISTS source_scope;
