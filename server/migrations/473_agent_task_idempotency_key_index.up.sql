CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_task_idempotency_key
    ON agent_task_queue (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
