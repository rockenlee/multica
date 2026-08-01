ALTER TABLE agent_task_queue
    ADD COLUMN idempotency_key TEXT;

ALTER TABLE agent_task_queue
    ADD CONSTRAINT agent_task_idempotency_key_length
    CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 255);

CREATE UNIQUE INDEX idx_agent_task_idempotency_key
    ON agent_task_queue (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
