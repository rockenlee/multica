CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_run_workspace_status
    ON agent_run (workspace_id, status, updated_at DESC);
