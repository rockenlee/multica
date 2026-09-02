CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_sync_event_workspace
    ON integration_sync_event(workspace_id, occurred_at DESC);
