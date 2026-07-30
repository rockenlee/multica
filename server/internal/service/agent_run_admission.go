package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/agentrun"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AgentRunTaskAdmission connects the agent-run contract to the real task
// queue. Issues without an active contract keep normal Multica behavior.
// Once a contract exists, only an executor whose step is already validated as
// running may receive a new platform task.
func AgentRunTaskAdmission(ctx context.Context, q *db.Queries, issueID, workspaceID, agentID pgtype.UUID) (allowed bool, reason string, err error) {
	raw, err := q.GetActiveAgentRunContractForIssue(ctx, db.GetActiveAgentRunContractForIssueParams{
		IssueID:     issueID,
		WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return true, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("load active agent run: %w", err)
	}

	var contract agentrun.Contract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return false, "", fmt.Errorf("decode active agent run: %w", err)
	}
	if err := agentrun.Validate(contract); err != nil {
		return false, "", fmt.Errorf("validate active agent run: %w", err)
	}

	agentIDString := util.UUIDToString(agentID)
	active := false
	for _, worker := range contract.ActiveWorkers {
		if worker == agentIDString {
			active = true
			break
		}
	}
	if !active {
		return false, "agent is not listed in active_workers for the active agent run", nil
	}
	for _, step := range contract.Steps {
		if step.Executor == agentIDString && step.Status == "running" {
			return true, "", nil
		}
	}
	return false, "agent has no running step in the active agent run", nil
}
