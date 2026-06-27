-- Workspace resources are the global pool of external resources a workspace can
-- reference from projects: Git repositories, Feishu Drive/Wiki entries, and
-- ZenTao projects/products. Project-level bindings remain separate.

CREATE TABLE workspace_resource (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_ref  JSONB NOT NULL,
    label         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    UUID REFERENCES "user"(id) ON DELETE SET NULL,
    UNIQUE (workspace_id, resource_type, resource_ref)
);

CREATE INDEX idx_workspace_resource_workspace
    ON workspace_resource(workspace_id, resource_type, created_at);

CREATE INDEX idx_workspace_resource_created_by
    ON workspace_resource(created_by)
    WHERE created_by IS NOT NULL;
