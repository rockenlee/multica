-- name: ListWorkspaceResources :many
SELECT * FROM workspace_resource
WHERE workspace_id = $1
ORDER BY resource_type ASC, created_at ASC;

-- name: GetWorkspaceResourceInWorkspace :one
SELECT * FROM workspace_resource
WHERE id = $1 AND workspace_id = $2;

-- name: CreateWorkspaceResource :one
INSERT INTO workspace_resource (
    workspace_id, resource_type, resource_ref, label, created_by
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: DeleteWorkspaceResource :exec
DELETE FROM workspace_resource WHERE id = $1;
