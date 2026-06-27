-- name: ListIntegrationConnections :many
SELECT * FROM integration_connection
WHERE workspace_id = $1
ORDER BY provider ASC, created_at ASC;

-- name: GetIntegrationConnectionInWorkspace :one
SELECT * FROM integration_connection
WHERE id = $1 AND workspace_id = $2;

-- name: CreateIntegrationConnection :one
INSERT INTO integration_connection (
    workspace_id, provider, name, base_url, config, status,
    sync_enabled_at, created_by_user_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    CASE WHEN sqlc.arg('sync_enabled')::boolean THEN now() ELSE NULL END,
    $7
)
RETURNING *;

-- name: UpdateIntegrationConnection :one
UPDATE integration_connection
SET name            = COALESCE(sqlc.narg('name'), name),
    base_url        = sqlc.narg('base_url'),
    config          = COALESCE(sqlc.narg('config'), config),
    status          = COALESCE(sqlc.narg('status'), status),
    sync_enabled_at = CASE
        WHEN sqlc.narg('sync_enabled')::boolean IS NULL THEN sync_enabled_at
        WHEN sqlc.narg('sync_enabled')::boolean THEN COALESCE(sync_enabled_at, now())
        ELSE NULL
    END,
    updated_at      = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteIntegrationConnection :exec
DELETE FROM integration_connection
WHERE id = $1 AND workspace_id = $2;

-- name: ListIntegrationUserAccountsForUser :many
SELECT * FROM integration_user_account
WHERE workspace_id = $1 AND user_id = $2
ORDER BY connection_id ASC, created_at ASC;

-- name: UpsertIntegrationUserAccount :one
INSERT INTO integration_user_account (
    workspace_id, connection_id, user_id, account_key, account_name,
    external_user_id, external_username, credential_encrypted, scopes,
    config, status, sync_enabled_at, expires_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11,
    CASE WHEN sqlc.arg('sync_enabled')::boolean THEN now() ELSE NULL END,
    $12
)
ON CONFLICT (connection_id, user_id, account_key) DO UPDATE SET
    account_name         = EXCLUDED.account_name,
    external_user_id     = EXCLUDED.external_user_id,
    external_username    = EXCLUDED.external_username,
    credential_encrypted = COALESCE(EXCLUDED.credential_encrypted, integration_user_account.credential_encrypted),
    scopes               = EXCLUDED.scopes,
    config               = EXCLUDED.config,
    status               = EXCLUDED.status,
    sync_enabled_at      = CASE
        WHEN sqlc.arg('sync_enabled')::boolean THEN COALESCE(integration_user_account.sync_enabled_at, now())
        ELSE NULL
    END,
    expires_at           = EXCLUDED.expires_at,
    last_error           = CASE WHEN EXCLUDED.status != 'error' THEN NULL ELSE integration_user_account.last_error END,
    updated_at           = now()
RETURNING *;

-- name: DeleteIntegrationUserAccount :exec
DELETE FROM integration_user_account
WHERE workspace_id = $1 AND connection_id = $2 AND user_id = $3 AND account_key = $4;

-- name: DeleteIntegrationUserAccountByID :exec
DELETE FROM integration_user_account
WHERE workspace_id = $1 AND user_id = $2 AND id = $3;

-- name: ListIntegrationProjectBindings :many
SELECT * FROM integration_project_binding
WHERE workspace_id = $1
ORDER BY created_at ASC;

-- name: ListIntegrationSyncEvents :many
SELECT * FROM integration_sync_event
WHERE workspace_id = $1
ORDER BY occurred_at DESC, created_at DESC
LIMIT $2;

-- name: ListIntegrationIssueSyncSettingsForUser :many
SELECT * FROM integration_issue_sync_setting
WHERE workspace_id = $1 AND user_id = $2
ORDER BY provider ASC;

-- name: GetIntegrationIssueSyncSettingForUser :one
SELECT * FROM integration_issue_sync_setting
WHERE workspace_id = $1 AND user_id = $2 AND provider = $3;

-- name: UpsertIntegrationIssueSyncSetting :one
INSERT INTO integration_issue_sync_setting (
    workspace_id, user_id, provider, inbound_enabled_at, outbound_enabled_at
) VALUES (
    $1, $2, $3,
    CASE WHEN sqlc.arg('inbound_enabled')::boolean THEN now() ELSE NULL END,
    CASE WHEN sqlc.arg('outbound_enabled')::boolean THEN now() ELSE NULL END
)
ON CONFLICT (workspace_id, user_id, provider) DO UPDATE SET
    inbound_enabled_at = CASE
        WHEN sqlc.arg('inbound_enabled')::boolean THEN COALESCE(integration_issue_sync_setting.inbound_enabled_at, now())
        ELSE NULL
    END,
    outbound_enabled_at = CASE
        WHEN sqlc.arg('outbound_enabled')::boolean THEN COALESCE(integration_issue_sync_setting.outbound_enabled_at, now())
        ELSE NULL
    END,
    updated_at = now()
RETURNING *;

-- name: CreateIntegrationSyncEvent :one
INSERT INTO integration_sync_event (
    workspace_id, connection_id, provider, direction, object_type,
    object_id, external_id, external_url, project_id, status,
    message, error, metadata
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13
)
RETURNING *;

-- name: UpsertIntegrationProjectBinding :one
INSERT INTO integration_project_binding (
    workspace_id, project_id, connection_id, external_ref,
    inbound_enabled_at, outbound_enabled_at,
    issue_sync_enabled_at, knowledge_sync_enabled_at,
    created_by_user_id
) VALUES (
    $1, $2, $3, $4,
    CASE WHEN sqlc.arg('inbound_enabled')::boolean THEN now() ELSE NULL END,
    CASE WHEN sqlc.arg('outbound_enabled')::boolean THEN now() ELSE NULL END,
    CASE WHEN sqlc.arg('issue_sync_enabled')::boolean THEN now() ELSE NULL END,
    CASE WHEN sqlc.arg('knowledge_sync_enabled')::boolean THEN now() ELSE NULL END,
    $5
)
ON CONFLICT (project_id, connection_id) DO UPDATE SET
    external_ref              = EXCLUDED.external_ref,
    inbound_enabled_at        = CASE
        WHEN sqlc.arg('inbound_enabled')::boolean THEN COALESCE(integration_project_binding.inbound_enabled_at, now())
        ELSE NULL
    END,
    outbound_enabled_at       = CASE
        WHEN sqlc.arg('outbound_enabled')::boolean THEN COALESCE(integration_project_binding.outbound_enabled_at, now())
        ELSE NULL
    END,
    issue_sync_enabled_at     = CASE
        WHEN sqlc.arg('issue_sync_enabled')::boolean THEN COALESCE(integration_project_binding.issue_sync_enabled_at, now())
        ELSE NULL
    END,
    knowledge_sync_enabled_at = CASE
        WHEN sqlc.arg('knowledge_sync_enabled')::boolean THEN COALESCE(integration_project_binding.knowledge_sync_enabled_at, now())
        ELSE NULL
    END,
    updated_at                = now()
RETURNING *;
