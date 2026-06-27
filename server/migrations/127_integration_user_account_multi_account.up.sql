-- Compatibility migration for databases that applied an earlier
-- 122_integration_module before user accounts supported multiple accounts.

ALTER TABLE integration_user_account
    ADD COLUMN IF NOT EXISTS account_key TEXT,
    ADD COLUMN IF NOT EXISTS account_name TEXT,
    ADD COLUMN IF NOT EXISTS scopes JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_error TEXT;

UPDATE integration_user_account
SET account_key = COALESCE(NULLIF(account_key, ''), 'default'),
    account_name = COALESCE(NULLIF(account_name, ''), NULLIF(external_username, ''), NULLIF(external_user_id, ''), 'Default'),
    scopes = COALESCE(scopes, '[]'::jsonb),
    status = COALESCE(NULLIF(status, ''), 'active');

ALTER TABLE integration_user_account
    ALTER COLUMN account_key SET NOT NULL,
    ALTER COLUMN account_name SET NOT NULL,
    ALTER COLUMN scopes SET NOT NULL,
    ALTER COLUMN status SET NOT NULL,
    ALTER COLUMN scopes SET DEFAULT '[]',
    ALTER COLUMN status SET DEFAULT 'active';

ALTER TABLE integration_user_account
    DROP CONSTRAINT IF EXISTS integration_user_account_connection_id_user_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_integration_user_account_connection_user_account
    ON integration_user_account(connection_id, user_id, account_key);

CREATE INDEX IF NOT EXISTS idx_integration_user_account_connection
    ON integration_user_account(connection_id, user_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'integration_user_account'::regclass
          AND conname = 'integration_user_account_status_check'
    ) THEN
        ALTER TABLE integration_user_account
            ADD CONSTRAINT integration_user_account_status_check
            CHECK (status IN ('active', 'disabled', 'error'));
    END IF;
END $$;
