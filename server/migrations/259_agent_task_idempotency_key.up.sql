ALTER TABLE agent_task_queue
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'agent_task_queue'::regclass
          AND conname = 'agent_task_idempotency_key_length'
    ) THEN
        ALTER TABLE agent_task_queue
            ADD CONSTRAINT agent_task_idempotency_key_length
            CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 255);
    END IF;
END $$;
