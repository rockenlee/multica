-- name: GetActiveAgentRunContractForIssue :one
SELECT contract
FROM agent_run
WHERE issue_id = @issue_id
  AND workspace_id = @workspace_id
  AND status IN ('draft', 'running', 'in_review')
ORDER BY created_at DESC
LIMIT 1;
