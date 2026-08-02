CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_user_account_connection
    ON integration_user_account(connection_id, user_id);
