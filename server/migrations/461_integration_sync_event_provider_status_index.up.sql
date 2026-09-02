CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_sync_event_provider_status
    ON integration_sync_event(workspace_id, provider, status, occurred_at DESC);
