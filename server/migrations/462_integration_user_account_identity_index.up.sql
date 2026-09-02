CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_user_account_connection_user_account
    ON integration_user_account(connection_id, user_id, account_key);
