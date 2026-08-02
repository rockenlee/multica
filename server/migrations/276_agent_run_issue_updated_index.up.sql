CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_run_issue_updated
    ON agent_run (issue_id, updated_at DESC);
