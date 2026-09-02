CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_sync_event_connection
    ON integration_sync_event(connection_id, occurred_at DESC);
