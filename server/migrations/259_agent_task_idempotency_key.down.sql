ALTER TABLE agent_task_queue
    DROP CONSTRAINT IF EXISTS agent_task_idempotency_key_length,
    DROP COLUMN IF EXISTS idempotency_key;
