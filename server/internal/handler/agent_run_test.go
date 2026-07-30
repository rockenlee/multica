package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/agentrun"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

func handlerAgentRunContract(runID, leadID, status string) agentrun.Contract {
	stepStatus := "planned"
	activeWorkers := []string{}
	evidence := []agentrun.Evidence{}
	if status == "running" {
		stepStatus = "running"
		activeWorkers = []string{leadID}
	}
	if status == "passed" {
		stepStatus = "passed"
		evidence = []agentrun.Evidence{
			{
				Kind:           "test",
				Producer:       leadID,
				StepID:         "implement",
				Outcome:        "pass",
				ArtifactRef:    "test://agent-run-handler",
				AcceptanceRefs: []string{"tests"},
				Gaps:           []string{},
			},
		}
	}
	return agentrun.Contract{
		Schema:                 agentrun.ProtocolVersion,
		ProtocolPackageVersion: agentrun.ProtocolPackageVersion,
		ProtocolSHA256:         testProtocolSHA,
		RunID:                  runID,
		DispatchAuthority: agentrun.DispatchAuthority{
			System: "multica",
			Actor:  leadID,
		},
		SourceRef:     "issue:test",
		Tier:          "S",
		Objective:     "verify agent run control-plane enforcement",
		WorkspaceMode: "worktree",
		Status:        status,
		ActiveWorkers: activeWorkers,
		Steps: []agentrun.Step{
			{
				StepID:   "implement",
				Role:     "implementer",
				Executor: leadID,
				Status:   stepStatus,
				Scope: agentrun.Scope{
					Workspace:      "/repo",
					WritablePaths:  []string{"server/internal/agentrun/**"},
					ForbiddenPaths: []string{"server/migrations/**"},
					ExternalWrites: false,
				},
				Acceptance: []agentrun.Acceptance{
					{ID: "tests", Check: "handler tests pass"},
				},
				Verification: []string{"go test ./internal/handler"},
				Evidence:     evidence,
			},
		},
		Review: agentrun.Review{
			Required:  false,
			Cycle:     0,
			MaxCycles: 3,
			Verdict: func() string {
				if status == "passed" {
					return "PASS"
				}
				return ""
			}(),
		},
	}
}

const testProtocolSHA = "7326e0ce5cca7258c2ba304c934656811e614b94739d603485d090472bc5bf68"

func agentRunRequest(t *testing.T, method, issueID, runID string, body any, leadID, taskID string) *http.Request {
	t.Helper()
	req := newRequest(method, "/api/issues/"+issueID+"/agent-runs/"+runID, body)
	req = withURLParams(req, "id", issueID, "runId", runID)
	req.Header.Set("X-Agent-ID", leadID)
	req.Header.Set("X-Task-ID", taskID)
	return req
}

func createAgentRunForTest(t *testing.T, issueID, runID, leadID, taskID string) agentRunResponse {
	t.Helper()
	body := createAgentRunRequest{
		Contract:        handlerAgentRunContract(runID, leadID, "draft"),
		IssueStatusMode: agentRunIssueStatusFollow,
	}
	w := httptest.NewRecorder()
	req := agentRunRequest(t, http.MethodPost, issueID, runID, body, leadID, taskID)
	testHandler.CreateAgentRun(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create agent run: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var response agentRunResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_run WHERE issue_id=$1`, issueID)
	})
	return response
}

func updateAgentRunForTest(t *testing.T, issueID, runID, leadID, taskID string, revision int, contract agentrun.Contract) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := agentRunRequest(t, http.MethodPut, issueID, runID, updateAgentRunRequest{
		ExpectedRevision: revision,
		Contract:         contract,
	}, leadID, taskID)
	testHandler.UpdateAgentRun(w, req)
	return w
}

func TestAgentRunCreateAndTransitionReconcilesIssue(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run transition")
	leadID := createHandlerTestAgent(t, "agent-run-lead", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)

	created := createAgentRunForTest(t, issueID, "run-transition", leadID, taskID)
	if created.Revision != 1 || created.Status != "draft" {
		t.Fatalf("created run = revision %d status %s, want revision 1 draft", created.Revision, created.Status)
	}

	running := handlerAgentRunContract(created.RunID, leadID, "running")
	w := updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, created.Revision, running)
	if w.Code != http.StatusOK {
		t.Fatalf("transition running: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var runningResponse agentRunResponse
	if err := json.NewDecoder(w.Body).Decode(&runningResponse); err != nil {
		t.Fatalf("decode running response: %v", err)
	}
	if runningResponse.Revision != 2 {
		t.Fatalf("running revision = %d, want 2", runningResponse.Revision)
	}

	passed := handlerAgentRunContract(created.RunID, leadID, "passed")
	w = updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, runningResponse.Revision, passed)
	if w.Code != http.StatusOK {
		t.Fatalf("transition passed: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var issueStatus string
	var metadata []byte
	if err := testPool.QueryRow(context.Background(), `SELECT status, metadata FROM issue WHERE id=$1`, issueID).Scan(&issueStatus, &metadata); err != nil {
		t.Fatalf("load reconciled issue: %v", err)
	}
	if issueStatus != "done" {
		t.Fatalf("issue status = %s, want done", issueStatus)
	}
	var metadataMap map[string]any
	if err := json.Unmarshal(metadata, &metadataMap); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadataMap["run.status"] != "passed" {
		t.Fatalf("metadata run.status = %v, want passed", metadataMap["run.status"])
	}
	if metadataMap["run.protocol_sha256"] != testProtocolSHA {
		t.Fatalf("metadata protocol hash = %v, want %s", metadataMap["run.protocol_sha256"], testProtocolSHA)
	}
}

func TestAgentRunRejectsMissingEvidencePass(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run missing evidence")
	leadID := createHandlerTestAgent(t, "agent-run-missing-evidence", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)
	created := createAgentRunForTest(t, issueID, "run-missing-evidence", leadID, taskID)

	running := handlerAgentRunContract(created.RunID, leadID, "running")
	w := updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, 1, running)
	if w.Code != http.StatusOK {
		t.Fatalf("transition running: %d %s", w.Code, w.Body.String())
	}

	invalid := handlerAgentRunContract(created.RunID, leadID, "passed")
	invalid.Steps[0].Evidence = nil
	w = updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, 2, invalid)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing evidence: expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if !jsonResponseHasViolation(t, w, "acceptance_without_passing_evidence") {
		t.Fatalf("missing evidence response did not include expected violation: %s", w.Body.String())
	}
}

func TestAgentRunRejectsCrossRunWritableScopeConflict(t *testing.T) {
	leftIssueID := createMetadataTestIssue(t, "Agent run scope left")
	rightIssueID := createMetadataTestIssue(t, "Agent run scope right")
	leadID := createHandlerTestAgent(t, "agent-run-scope-lead", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)

	left := createAgentRunForTest(t, leftIssueID, "run-scope-left", leadID, taskID)
	right := createAgentRunForTest(t, rightIssueID, "run-scope-right", leadID, taskID)

	leftRunning := handlerAgentRunContract(left.RunID, leadID, "running")
	w := updateAgentRunForTest(t, leftIssueID, left.RunID, leadID, taskID, 1, leftRunning)
	if w.Code != http.StatusOK {
		t.Fatalf("left transition: %d %s", w.Code, w.Body.String())
	}

	rightRunning := handlerAgentRunContract(right.RunID, leadID, "running")
	rightRunning.Steps[0].Scope.WritablePaths = []string{"server/internal/agentrun/contract.go"}
	w = updateAgentRunForTest(t, rightIssueID, right.RunID, leadID, taskID, 1, rightRunning)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("scope conflict: expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if !jsonResponseHasViolation(t, w, "writable_scope_conflict") {
		t.Fatalf("scope conflict response did not include expected violation: %s", w.Body.String())
	}
}

func TestAgentRunRejectsStaleRevision(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run stale revision")
	leadID := createHandlerTestAgent(t, "agent-run-stale-revision", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)
	created := createAgentRunForTest(t, issueID, "run-stale-revision", leadID, taskID)

	running := handlerAgentRunContract(created.RunID, leadID, "running")
	w := updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, 1, running)
	if w.Code != http.StatusOK {
		t.Fatalf("first update: %d %s", w.Code, w.Body.String())
	}
	w = updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, 1, running)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale revision: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAgentRunRejectsTerminalStateWithActivePlatformTask(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run active child")
	leadID := createHandlerTestAgent(t, "agent-run-active-child-lead", nil)
	workerID := createHandlerTestAgent(t, "agent-run-active-child-worker", nil)
	leadTaskID := createHandlerTestTaskForAgent(t, leadID)
	createHandlerTestTaskForAgentOnIssue(t, workerID, issueID)
	created := createAgentRunForTest(t, issueID, "run-active-child", leadID, leadTaskID)

	running := handlerAgentRunContract(created.RunID, leadID, "running")
	w := updateAgentRunForTest(t, issueID, created.RunID, leadID, leadTaskID, 1, running)
	if w.Code != http.StatusOK {
		t.Fatalf("transition running: %d %s", w.Code, w.Body.String())
	}

	passed := handlerAgentRunContract(created.RunID, leadID, "passed")
	w = updateAgentRunForTest(t, issueID, created.RunID, leadID, leadTaskID, 2, passed)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("active child: expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if !jsonResponseHasViolation(t, w, "terminal_run_has_active_platform_tasks") {
		t.Fatalf("active child response did not include expected violation: %s", w.Body.String())
	}
}

func TestAgentRunTaskAdmissionRequiresValidatedRunningStep(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run task admission")
	leadID := createHandlerTestAgent(t, "agent-run-admission-lead", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)
	created := createAgentRunForTest(t, issueID, "run-admission", leadID, taskID)

	issue, err := testHandler.Queries.GetIssue(context.Background(), util.MustParseUUID(issueID))
	if err != nil {
		t.Fatalf("load issue: %v", err)
	}
	allowed, reason, err := service.AgentRunTaskAdmission(
		context.Background(),
		testHandler.Queries,
		issue.ID,
		issue.WorkspaceID,
		util.MustParseUUID(leadID),
	)
	if err != nil {
		t.Fatalf("draft admission: %v", err)
	}
	if allowed || reason == "" {
		t.Fatalf("draft admission = allowed %v reason %q, want rejected with reason", allowed, reason)
	}

	running := handlerAgentRunContract(created.RunID, leadID, "running")
	w := updateAgentRunForTest(t, issueID, created.RunID, leadID, taskID, created.Revision, running)
	if w.Code != http.StatusOK {
		t.Fatalf("transition running: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	allowed, reason, err = service.AgentRunTaskAdmission(
		context.Background(),
		testHandler.Queries,
		issue.ID,
		issue.WorkspaceID,
		util.MustParseUUID(leadID),
	)
	if err != nil {
		t.Fatalf("running admission: %v", err)
	}
	if !allowed || reason != "" {
		t.Fatalf("running admission = allowed %v reason %q, want allowed", allowed, reason)
	}
}

func TestAgentRunRejectsSecondActiveRunOnSameIssue(t *testing.T) {
	issueID := createMetadataTestIssue(t, "Agent run single active")
	leadID := createHandlerTestAgent(t, "agent-run-single-active-lead", nil)
	taskID := createHandlerTestTaskForAgent(t, leadID)
	createAgentRunForTest(t, issueID, "run-first", leadID, taskID)

	body := createAgentRunRequest{
		Contract:        handlerAgentRunContract("run-second", leadID, "draft"),
		IssueStatusMode: agentRunIssueStatusFollow,
	}
	w := httptest.NewRecorder()
	req := agentRunRequest(t, http.MethodPost, issueID, "run-second", body, leadID, taskID)
	testHandler.CreateAgentRun(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("second active run: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func jsonResponseHasViolation(t *testing.T, w *httptest.ResponseRecorder, code string) bool {
	t.Helper()
	var response struct {
		Violations []agentrun.Violation `json:"violations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode violation response: %v", err)
	}
	for _, violation := range response.Violations {
		if violation.Code == code {
			return true
		}
	}
	return false
}
