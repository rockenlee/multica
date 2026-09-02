CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workspace_resource_workspace
    ON workspace_resource(workspace_id, resource_type, created_at);
