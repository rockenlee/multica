CREATE TABLE IF NOT EXISTS agent_run (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    issue_id UUID NOT NULL,
    run_id TEXT NOT NULL,
    protocol_version TEXT NOT NULL,
    protocol_package_version TEXT NOT NULL,
    protocol_sha256 TEXT NOT NULL,
    status TEXT NOT NULL,
    dispatch_authority TEXT NOT NULL,
    contract JSONB NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    issue_status_mode TEXT NOT NULL DEFAULT 'follow_run',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_run_workspace_run_unique UNIQUE (workspace_id, run_id),
    CONSTRAINT agent_run_protocol_version_check CHECK (protocol_version = 'agent-run/v1'),
    CONSTRAINT agent_run_protocol_sha256_check CHECK (protocol_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT agent_run_status_check CHECK (
        status IN ('draft', 'running', 'in_review', 'passed', 'blocked', 'failed', 'cancelled')
    ),
    CONSTRAINT agent_run_issue_status_mode_check CHECK (
        issue_status_mode IN ('follow_run', 'none')
    ),
    CONSTRAINT agent_run_contract_object_check CHECK (jsonb_typeof(contract) = 'object'),
    CONSTRAINT agent_run_contract_size_check CHECK (pg_column_size(contract) <= 65536),
    CONSTRAINT agent_run_revision_check CHECK (revision > 0)
);

ALTER TABLE agent_run
    DROP CONSTRAINT IF EXISTS agent_run_workspace_id_fkey,
    DROP CONSTRAINT IF EXISTS agent_run_issue_id_fkey;
