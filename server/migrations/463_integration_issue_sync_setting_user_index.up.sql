CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_issue_sync_setting_user
    ON integration_issue_sync_setting(user_id, workspace_id);
