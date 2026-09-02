CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_connection_workspace
    ON integration_connection(workspace_id, provider);
