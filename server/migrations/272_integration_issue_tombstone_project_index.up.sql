CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_issue_tombstone_project
    ON integration_issue_tombstone(workspace_id, project_id);
