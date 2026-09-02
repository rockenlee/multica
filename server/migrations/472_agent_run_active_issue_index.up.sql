CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_run_one_active_per_issue
    ON agent_run (issue_id)
    WHERE status IN ('draft', 'running', 'in_review');
