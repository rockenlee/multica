package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/agentrun"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const (
	agentRunIssueStatusFollow = "follow_run"
	agentRunIssueStatusNone   = "none"
)

type createAgentRunRequest struct {
	Contract        agentrun.Contract `json:"contract"`
	IssueStatusMode string            `json:"issue_status_mode,omitempty"`
}

type updateAgentRunRequest struct {
	ExpectedRevision int               `json:"expected_revision"`
	Contract         agentrun.Contract `json:"contract"`
}

type agentRunRecord struct {
	ID                     pgtype.UUID
	WorkspaceID            pgtype.UUID
	IssueID                pgtype.UUID
	RunID                  string
	ProtocolVersion        string
	ProtocolPackageVersion string
	ProtocolSHA256         string
	Status                 string
	DispatchAuthority      string
	Contract               agentrun.Contract
	Revision               int
	IssueStatusMode        string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type agentRunResponse struct {
	ID                     string            `json:"id"`
	WorkspaceID            string            `json:"workspace_id"`
	IssueID                string            `json:"issue_id"`
	RunID                  string            `json:"run_id"`
	ProtocolVersion        string            `json:"protocol_version"`
	ProtocolPackageVersion string            `json:"protocol_package_version"`
	ProtocolSHA256         string            `json:"protocol_sha256"`
	Status                 string            `json:"status"`
	DispatchAuthority      string            `json:"dispatch_authority"`
	Contract               agentrun.Contract `json:"contract"`
	Revision               int               `json:"revision"`
	IssueStatusMode        string            `json:"issue_status_mode"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgentRun(row rowScanner) (agentRunRecord, error) {
	var record agentRunRecord
	var contractJSON []byte
	err := row.Scan(
		&record.ID,
		&record.WorkspaceID,
		&record.IssueID,
		&record.RunID,
		&record.ProtocolVersion,
		&record.ProtocolPackageVersion,
		&record.ProtocolSHA256,
		&record.Status,
		&record.DispatchAuthority,
		&contractJSON,
		&record.Revision,
		&record.IssueStatusMode,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return agentRunRecord{}, err
	}
	if err := json.Unmarshal(contractJSON, &record.Contract); err != nil {
		return agentRunRecord{}, fmt.Errorf("decode stored agent run contract: %w", err)
	}
	return record, nil
}

func agentRunToResponse(record agentRunRecord) agentRunResponse {
	return agentRunResponse{
		ID:                     uuidToString(record.ID),
		WorkspaceID:            uuidToString(record.WorkspaceID),
		IssueID:                uuidToString(record.IssueID),
		RunID:                  record.RunID,
		ProtocolVersion:        record.ProtocolVersion,
		ProtocolPackageVersion: record.ProtocolPackageVersion,
		ProtocolSHA256:         record.ProtocolSHA256,
		Status:                 record.Status,
		DispatchAuthority:      record.DispatchAuthority,
		Contract:               record.Contract,
		Revision:               record.Revision,
		IssueStatusMode:        record.IssueStatusMode,
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	}
}

func (h *Handler) CreateAgentRun(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req createAgentRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IssueStatusMode == "" {
		req.IssueStatusMode = agentRunIssueStatusFollow
	}
	if req.IssueStatusMode != agentRunIssueStatusFollow && req.IssueStatusMode != agentRunIssueStatusNone {
		writeError(w, http.StatusBadRequest, "issue_status_mode must be follow_run or none")
		return
	}
	if req.Contract.Status != "draft" {
		writeAgentRunViolation(w, http.StatusUnprocessableEntity, agentrun.Violation{
			Code:    "run_must_start_draft",
			Path:    "contract.status",
			Message: "new agent run must start in draft",
		})
		return
	}
	if err := agentrun.Validate(req.Contract); err != nil {
		writeAgentRunValidationError(w, err)
		return
	}
	if violations := h.validateMulticaAgentRunIdentities(r.Context(), req.Contract, issue.WorkspaceID); len(violations) > 0 {
		writeAgentRunViolations(w, http.StatusUnprocessableEntity, violations)
		return
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if !agentRunActorAllowed(req.Contract, actorType, actorID) {
		writeError(w, http.StatusForbidden, "only the declared Multica dispatch authority may create this run")
		return
	}

	contractJSON, err := json.Marshal(req.Contract)
	if err != nil {
		writeError(w, http.StatusBadRequest, "contract cannot be encoded")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start agent run transaction")
		return
	}
	defer tx.Rollback(r.Context())

	if err := lockAgentRunWorkspace(r, tx, issue.WorkspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock agent run workspace")
		return
	}

	record, err := scanAgentRun(tx.QueryRow(r.Context(), `
INSERT INTO agent_run (
    workspace_id, issue_id, run_id, protocol_version,
    protocol_package_version, protocol_sha256, status,
    dispatch_authority, contract, issue_status_mode
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id, workspace_id, issue_id, run_id, protocol_version,
          protocol_package_version, protocol_sha256, status,
          dispatch_authority, contract, revision, issue_status_mode,
          created_at, updated_at
`,
		issue.WorkspaceID,
		issue.ID,
		req.Contract.RunID,
		req.Contract.Schema,
		req.Contract.ProtocolPackageVersion,
		req.Contract.ProtocolSHA256,
		req.Contract.Status,
		req.Contract.DispatchAuthority.Actor,
		contractJSON,
		req.IssueStatusMode,
	))
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "agent run id already exists in this workspace")
			return
		}
		slog.Warn("create agent run failed", append(logger.RequestAttrs(r), "error", err, "issue_id", uuidToString(issue.ID), "run_id", req.Contract.RunID)...)
		writeError(w, http.StatusInternalServerError, "failed to create agent run")
		return
	}

	if err := syncAgentRunMetadata(r, tx, issue, req.Contract); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit agent run")
		return
	}

	h.publishAgentRunMetadata(issue, req.Contract, actorType, actorID)
	writeJSON(w, http.StatusCreated, agentRunToResponse(record))
}

func (h *Handler) GetAgentRun(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	runID := chi.URLParam(r, "runId")
	record, err := scanAgentRun(h.DB.QueryRow(r.Context(), `
SELECT id, workspace_id, issue_id, run_id, protocol_version,
       protocol_package_version, protocol_sha256, status,
       dispatch_authority, contract, revision, issue_status_mode,
       created_at, updated_at
FROM agent_run
WHERE issue_id=$1 AND workspace_id=$2 AND run_id=$3
`, issue.ID, issue.WorkspaceID, runID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load agent run")
		return
	}
	writeJSON(w, http.StatusOK, agentRunToResponse(record))
}

func (h *Handler) ListAgentRuns(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT id, workspace_id, issue_id, run_id, protocol_version,
       protocol_package_version, protocol_sha256, status,
       dispatch_authority, contract, revision, issue_status_mode,
       created_at, updated_at
FROM agent_run
WHERE issue_id=$1 AND workspace_id=$2
ORDER BY created_at DESC
`, issue.ID, issue.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent runs")
		return
	}
	defer rows.Close()

	response := make([]agentRunResponse, 0)
	for rows.Next() {
		record, scanErr := scanAgentRun(rows)
		if scanErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to decode agent run")
			return
		}
		response = append(response, agentRunToResponse(record))
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent runs")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateAgentRun(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	runID := chi.URLParam(r, "runId")

	var req updateAgentRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ExpectedRevision <= 0 {
		writeError(w, http.StatusBadRequest, "expected_revision must be greater than zero")
		return
	}
	if req.Contract.RunID != runID {
		writeAgentRunViolation(w, http.StatusUnprocessableEntity, agentrun.Violation{
			Code:    "run_id_path_mismatch",
			Path:    "contract.run_id",
			Message: "contract run_id must match the URL",
		})
		return
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if !agentRunActorAllowed(req.Contract, actorType, actorID) {
		writeError(w, http.StatusForbidden, "only the declared Multica dispatch authority may update this run")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start agent run transaction")
		return
	}
	defer tx.Rollback(r.Context())

	if err := lockAgentRunWorkspace(r, tx, issue.WorkspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock agent run workspace")
		return
	}

	previous, err := scanAgentRun(tx.QueryRow(r.Context(), `
SELECT id, workspace_id, issue_id, run_id, protocol_version,
       protocol_package_version, protocol_sha256, status,
       dispatch_authority, contract, revision, issue_status_mode,
       created_at, updated_at
FROM agent_run
WHERE issue_id=$1 AND workspace_id=$2 AND run_id=$3
FOR UPDATE
`, issue.ID, issue.WorkspaceID, runID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load agent run")
		return
	}
	if previous.Revision != req.ExpectedRevision {
		writeError(w, http.StatusConflict, fmt.Sprintf("agent run revision conflict: current revision is %d", previous.Revision))
		return
	}
	if previous.DispatchAuthority != actorID {
		writeError(w, http.StatusForbidden, "only the persisted dispatch authority may update this run")
		return
	}
	if err := agentrun.ValidateTransition(previous.Contract, req.Contract); err != nil {
		writeAgentRunValidationError(w, err)
		return
	}
	if violations := h.validateMulticaAgentRunIdentities(r.Context(), req.Contract, issue.WorkspaceID); len(violations) > 0 {
		writeAgentRunViolations(w, http.StatusUnprocessableEntity, violations)
		return
	}

	otherContracts, err := loadOtherActiveAgentRunContracts(r, tx, issue.WorkspaceID, previous.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate concurrent agent runs")
		return
	}
	if err := agentrun.ValidateConcurrentContracts(append(otherContracts, req.Contract)...); err != nil {
		writeAgentRunValidationError(w, err)
		return
	}
	if terminalAgentRunStatus(req.Contract.Status) {
		activeChildren, checkErr := activeAgentRunChildTaskCount(r, tx, issue, actorID)
		if checkErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate terminal agent run workers")
			return
		}
		if activeChildren > 0 {
			writeAgentRunViolation(w, http.StatusUnprocessableEntity, agentrun.Violation{
				Code:    "terminal_run_has_active_platform_tasks",
				Path:    "status",
				Message: fmt.Sprintf("terminal run rejected while %d non-closure platform task(s) remain active", activeChildren),
			})
			return
		}
	}

	contractJSON, err := json.Marshal(req.Contract)
	if err != nil {
		writeError(w, http.StatusBadRequest, "contract cannot be encoded")
		return
	}

	record, err := scanAgentRun(tx.QueryRow(r.Context(), `
UPDATE agent_run
SET status=$1,
    contract=$2,
    revision=revision+1,
    updated_at=now()
WHERE id=$3 AND revision=$4
RETURNING id, workspace_id, issue_id, run_id, protocol_version,
          protocol_package_version, protocol_sha256, status,
          dispatch_authority, contract, revision, issue_status_mode,
          created_at, updated_at
`, req.Contract.Status, contractJSON, previous.ID, req.ExpectedRevision))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "agent run revision changed during update")
			return
		}
		slog.Warn("update agent run failed", append(logger.RequestAttrs(r), "error", err, "issue_id", uuidToString(issue.ID), "run_id", runID)...)
		writeError(w, http.StatusInternalServerError, "failed to update agent run")
		return
	}

	previousIssueStatus := issue.Status
	if err := syncAgentRunMetadata(r, tx, issue, req.Contract); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nextIssueStatus := previousIssueStatus
	if previous.IssueStatusMode == agentRunIssueStatusFollow {
		if mapped := issueStatusForAgentRun(req.Contract.Status); mapped != "" {
			if err := tx.QueryRow(r.Context(), `
UPDATE issue
SET status=$1, updated_at=now()
WHERE id=$2 AND workspace_id=$3
RETURNING status
`, mapped, issue.ID, issue.WorkspaceID).Scan(&nextIssueStatus); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to reconcile issue status")
				return
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit agent run update")
		return
	}

	h.publishAgentRunMetadata(issue, req.Contract, actorType, actorID)
	h.publishAgentRunIssueStatus(r, issue, previousIssueStatus, nextIssueStatus, actorType, actorID, userID)
	writeJSON(w, http.StatusOK, agentRunToResponse(record))
}

func agentRunActorAllowed(contract agentrun.Contract, actorType, actorID string) bool {
	return contract.DispatchAuthority.System == "multica" &&
		actorType == "agent" &&
		actorID != "" &&
		actorID == contract.DispatchAuthority.Actor
}

func (h *Handler) validateMulticaAgentRunIdentities(ctx context.Context, contract agentrun.Contract, workspaceID pgtype.UUID) []agentrun.Violation {
	type identityUse struct {
		id        string
		fieldPath string
	}
	uses := []identityUse{
		{id: contract.DispatchAuthority.Actor, fieldPath: "dispatch_authority.actor"},
	}
	for i, step := range contract.Steps {
		uses = append(uses, identityUse{
			id:        step.Executor,
			fieldPath: fmt.Sprintf("steps[%d].executor", i),
		})
	}
	if contract.Review.Reviewer != "" {
		uses = append(uses, identityUse{
			id:        contract.Review.Reviewer,
			fieldPath: "review.reviewer",
		})
	}

	type identityResult struct {
		valid    bool
		archived bool
	}
	results := make(map[string]identityResult, len(uses))
	violations := make([]agentrun.Violation, 0)
	for _, use := range uses {
		if _, seen := results[use.id]; !seen {
			parsed, err := util.ParseUUID(use.id)
			if err != nil {
				results[use.id] = identityResult{}
			} else {
				agent, loadErr := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{
					ID:          parsed,
					WorkspaceID: workspaceID,
				})
				results[use.id] = identityResult{
					valid:    loadErr == nil,
					archived: loadErr == nil && agent.ArchivedAt.Valid,
				}
			}
		}
		result := results[use.id]
		switch {
		case !result.valid:
			violations = append(violations, agentrun.Violation{
				Code:    "unknown_multica_agent",
				Path:    use.fieldPath,
				Message: "identity must reference an agent in this Multica workspace",
			})
		case result.archived:
			violations = append(violations, agentrun.Violation{
				Code:    "archived_multica_agent",
				Path:    use.fieldPath,
				Message: "archived agent cannot participate in an active run",
			})
		}
	}
	return violations
}

func lockAgentRunWorkspace(r *http.Request, tx pgx.Tx, workspaceID pgtype.UUID) error {
	_, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, workspaceID)
	return err
}

func terminalAgentRunStatus(status string) bool {
	switch status {
	case "passed", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func activeAgentRunChildTaskCount(r *http.Request, tx pgx.Tx, issue db.Issue, actorID string) (int, error) {
	var closureTaskID pgtype.UUID
	if rawTaskID := r.Header.Get("X-Task-ID"); rawTaskID != "" {
		if parsedTaskID, err := util.ParseUUID(rawTaskID); err == nil {
			var belongs bool
			if err := tx.QueryRow(r.Context(), `
SELECT EXISTS (
    SELECT 1
    FROM agent_task_queue
    WHERE id=$1
      AND issue_id=$2
      AND agent_id=$3
      AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, parsedTaskID, issue.ID, parseUUID(actorID)).Scan(&belongs); err != nil {
				return 0, err
			}
			if belongs {
				closureTaskID = parsedTaskID
			}
		}
	}

	var count int
	err := tx.QueryRow(r.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE issue_id=$1
  AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
  AND ($2::uuid IS NULL OR id<>$2)
`, issue.ID, closureTaskID).Scan(&count)
	return count, err
}

func loadOtherActiveAgentRunContracts(r *http.Request, tx pgx.Tx, workspaceID, excludeRunID pgtype.UUID) ([]agentrun.Contract, error) {
	rows, err := tx.Query(r.Context(), `
SELECT contract
FROM agent_run
WHERE workspace_id=$1
  AND id<>$2
  AND status IN ('draft', 'running', 'in_review')
FOR UPDATE
`, workspaceID, excludeRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contracts := make([]agentrun.Contract, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var contract agentrun.Contract
		if err := json.Unmarshal(raw, &contract); err != nil {
			return nil, err
		}
		contracts = append(contracts, contract)
	}
	return contracts, rows.Err()
}

func syncAgentRunMetadata(r *http.Request, tx pgx.Tx, issue db.Issue, contract agentrun.Contract) error {
	baseRevision := ""
	if contract.BaseRevision != nil {
		baseRevision = *contract.BaseRevision
	}
	evidenceRef := fmt.Sprintf("agent-run:%s#steps.evidence", contract.RunID)
	contractRef := fmt.Sprintf("agent-run:%s", contract.RunID)
	_, err := tx.Exec(r.Context(), `
UPDATE issue
SET metadata = metadata || jsonb_build_object(
        'run.schema', $1::text,
        'run.protocol_package_version', $2::text,
        'run.protocol_sha256', $3::text,
        'run.id', $4::text,
        'run.tier', $5::text,
        'run.status', $6::text,
        'run.base_revision', $7::text,
        'run.dispatch_authority', $8::text,
        'run.contract_ref', $9::text,
        'run.review_cycle', $10::int,
        'run.review_max', $11::int,
        'run.review_verdict', $12::text,
        'run.evidence_ref', $13::text
    ),
    updated_at = now()
WHERE id=$14 AND workspace_id=$15
`,
		contract.Schema,
		contract.ProtocolPackageVersion,
		contract.ProtocolSHA256,
		contract.RunID,
		contract.Tier,
		contract.Status,
		baseRevision,
		contract.DispatchAuthority.Actor,
		contractRef,
		contract.Review.Cycle,
		contract.Review.MaxCycles,
		contract.Review.Verdict,
		evidenceRef,
		issue.ID,
		issue.WorkspaceID,
	)
	if err != nil {
		if isCheckViolation(err) {
			return errors.New("agent run metadata exceeds the issue metadata size limit")
		}
		return fmt.Errorf("persist agent run metadata: %w", err)
	}
	return nil
}

func issueStatusForAgentRun(runStatus string) string {
	switch runStatus {
	case "running":
		return "in_progress"
	case "in_review":
		return "in_review"
	case "passed":
		return "done"
	case "blocked", "failed":
		return "blocked"
	case "cancelled":
		return "cancelled"
	default:
		return ""
	}
}

func (h *Handler) publishAgentRunMetadata(issue db.Issue, contract agentrun.Contract, actorType, actorID string) {
	h.publish(protocol.EventIssueMetadataChanged, uuidToString(issue.WorkspaceID), actorType, actorID, map[string]any{
		"issue_id": uuidToString(issue.ID),
		"agent_run": map[string]any{
			"run_id":           contract.RunID,
			"status":           contract.Status,
			"protocol_version": contract.Schema,
			"protocol_sha256":  contract.ProtocolSHA256,
		},
	})
}

func (h *Handler) publishAgentRunIssueStatus(r *http.Request, previous db.Issue, previousStatus, nextStatus, actorType, actorID, requestUserID string) {
	if previousStatus == nextStatus {
		return
	}
	updated, err := h.Queries.GetIssue(r.Context(), previous.ID)
	if err != nil {
		slog.Warn("reload issue after agent run status reconciliation failed", "issue_id", uuidToString(previous.ID), "error", err)
		return
	}
	h.recordIssueStatusChangedActivity(r.Context(), updated, actorType, actorID, previousStatus, nextStatus)
	if changer, parseErr := parseUUIDLoose(requestUserID); parseErr == nil {
		go h.pushIssueStatusOutbound(updated, nextStatus, changer)
	}
	prefix := h.getIssuePrefix(r.Context(), updated.WorkspaceID)
	h.publish(protocol.EventIssueUpdated, uuidToString(updated.WorkspaceID), actorType, actorID, map[string]any{
		"issue":          issueToResponse(updated, prefix),
		"status_changed": true,
		"prev_status":    previousStatus,
	})
	if nextStatus == "cancelled" {
		h.TaskService.CancelTasksForIssue(r.Context(), updated.ID)
	}
	h.notifyParentOfChildDone(r.Context(), previous, updated)
}

func writeAgentRunValidationError(w http.ResponseWriter, err error) {
	var validationErr *agentrun.ValidationError
	if errors.As(err, &validationErr) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":      "agent run contract rejected",
			"code":       "agent_run_contract_invalid",
			"violations": validationErr.Violations,
		})
		return
	}
	writeError(w, http.StatusUnprocessableEntity, strings.TrimSpace(err.Error()))
}

func writeAgentRunViolation(w http.ResponseWriter, status int, violation agentrun.Violation) {
	writeAgentRunViolations(w, status, []agentrun.Violation{violation})
}

func writeAgentRunViolations(w http.ResponseWriter, status int, violations []agentrun.Violation) {
	writeJSON(w, status, map[string]any{
		"error":      "agent run contract rejected",
		"code":       "agent_run_contract_invalid",
		"violations": violations,
	})
}
