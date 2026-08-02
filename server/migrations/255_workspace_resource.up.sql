-- Workspace resources are the global pool of external resources a workspace can
-- reference from projects: Git repositories, Feishu Drive/Wiki entries, and
-- ZenTao projects/products. Project-level bindings remain separate.

CREATE TABLE IF NOT EXISTS workspace_resource (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL,
    resource_type TEXT NOT NULL,
    resource_ref  JSONB NOT NULL,
    label         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    UUID,
    UNIQUE (workspace_id, resource_type, resource_ref)
);

ALTER TABLE workspace_resource
    DROP CONSTRAINT IF EXISTS workspace_resource_workspace_id_fkey,
    DROP CONSTRAINT IF EXISTS workspace_resource_created_by_fkey;
