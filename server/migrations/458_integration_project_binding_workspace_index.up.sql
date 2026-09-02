CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_integration_project_binding_workspace
    ON integration_project_binding(workspace_id, project_id);
