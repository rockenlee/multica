CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_user_account_user
    ON integration_user_account(user_id, workspace_id);
