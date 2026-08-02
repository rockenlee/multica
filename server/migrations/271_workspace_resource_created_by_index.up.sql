CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workspace_resource_created_by
    ON workspace_resource(created_by)
    WHERE created_by IS NOT NULL;
