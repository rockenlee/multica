package main

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/agentrun"
)

type agentRunHTTPResponse struct {
	RunID           string            `json:"run_id"`
	ProtocolSHA256  string            `json:"protocol_sha256"`
	Status          string            `json:"status"`
	Contract        agentrun.Contract `json:"contract"`
	Revision        int               `json:"revision"`
	IssueStatusMode string            `json:"issue_status_mode"`
}

func httpAgentRunContract(issueID, runID, agentID, status string) agentrun.Contract {
	stepStatus := "planned"
	activeWorkers := []string{}
	evidence := []agentrun.Evidence{}
	if status == "running" {
		stepStatus = "running"
		activeWorkers = []string{agentID}
	}
	if status == "passed" {
		stepStatus = "passed"
		evidence = []agentrun.Evidence{{
			Kind:           "test",
			Producer:       agentID,
			StepID:         "implement",
			Outcome:        "pass",
			ArtifactRef:    "http-test://agent-run/native-route",
			AcceptanceRefs: []string{"native-route"},
			Gaps:           []string{},
		}}
	}
	return agentrun.Contract{
		Schema:                 agentrun.ProtocolVersion,
		ProtocolPackageVersion: agentrun.ProtocolPackageVersion,
		ProtocolSHA256:         agentrun.ProtocolSHA256,
		RunID:                  runID,
		DispatchAuthority: agentrun.DispatchAuthority{
			System: "multica",
			Actor:  agentID,
		},
		SourceRef:     "issue:" + issueID,
		Tier:          "S",
		Objective:     "prove the shared protocol is enforced by the native HTTP surface",
		WorkspaceMode: "directory",
		Status:        status,
		ActiveWorkers: activeWorkers,
		Steps: []agentrun.Step{{
			StepID:   "implement",
			Role:     "implementer",
			Executor: agentID,
			Status:   stepStatus,
			Scope: agentrun.Scope{
				Workspace:      "/integration-test",
				WritablePaths:  []string{"server/cmd/server/**"},
				ForbiddenPaths: []string{},
				ExternalWrites: false,
			},
			Acceptance: []agentrun.Acceptance{{
				ID:    "native-route",
				Check: "native HTTP create, reject, update, and readback all enforce the protocol",
			}},
			Verification: []string{"go test ./cmd/server"},
			Evidence:     evidence,
		}},
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

func requireAgentRunStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode == want {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Fatalf("agent-run HTTP status = %d, want %d: %s", resp.StatusCode, want, body)
}

func TestAgentRunNativeHTTPEnforcesSharedProtocol(t *testing.T) {
	agentID := getAgentID(t)
	issueID := createIssue(t, "Agent-run native HTTP protocol acceptance")
	t.Cleanup(func() {
		resp := authRequest(t, http.MethodDelete, "/api/issues/"+issueID, nil)
		resp.Body.Close()
	})

	runID := "http-native-shared-protocol"
	runPath := fmt.Sprintf("/api/issues/%s/agent-runs/%s", issueID, runID)
	createPath := fmt.Sprintf("/api/issues/%s/agent-runs", issueID)

	draft := httpAgentRunContract(issueID, runID, agentID, "draft")
	resp := authRequestWithAgent(t, http.MethodPost, createPath, map[string]any{
		"contract":          draft,
		"issue_status_mode": "follow_run",
	}, agentID)
	requireAgentRunStatus(t, resp, http.StatusCreated)
	var created agentRunHTTPResponse
	readJSON(t, resp, &created)
	if created.Revision != 1 || created.Status != "draft" ||
		created.ProtocolSHA256 != agentrun.ProtocolSHA256 {
		t.Fatalf(
			"created run = revision %d status %s protocol %s",
			created.Revision,
			created.Status,
			created.ProtocolSHA256,
		)
	}

	wrongIdentity := draft
	wrongIdentity.ProtocolSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	resp = authRequestWithAgent(t, http.MethodPut, runPath, map[string]any{
		"expected_revision": created.Revision,
		"contract":          wrongIdentity,
	}, agentID)
	requireAgentRunStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()

	resp = authRequest(t, http.MethodGet, runPath, nil)
	requireAgentRunStatus(t, resp, http.StatusOK)
	var afterIdentityReject agentRunHTTPResponse
	readJSON(t, resp, &afterIdentityReject)
	if afterIdentityReject.Revision != 1 || afterIdentityReject.Status != "draft" ||
		afterIdentityReject.ProtocolSHA256 != agentrun.ProtocolSHA256 {
		t.Fatalf(
			"identity rejection mutated run: revision %d status %s protocol %s",
			afterIdentityReject.Revision,
			afterIdentityReject.Status,
			afterIdentityReject.ProtocolSHA256,
		)
	}

	running := httpAgentRunContract(issueID, runID, agentID, "running")
	resp = authRequestWithAgent(t, http.MethodPut, runPath, map[string]any{
		"expected_revision": afterIdentityReject.Revision,
		"contract":          running,
	}, agentID)
	requireAgentRunStatus(t, resp, http.StatusOK)
	var runningResponse agentRunHTTPResponse
	readJSON(t, resp, &runningResponse)
	if runningResponse.Revision != 2 || runningResponse.Status != "running" {
		t.Fatalf(
			"running run = revision %d status %s, want revision 2 running",
			runningResponse.Revision,
			runningResponse.Status,
		)
	}

	falsePass := httpAgentRunContract(issueID, runID, agentID, "passed")
	falsePass.Steps[0].Evidence = nil
	resp = authRequestWithAgent(t, http.MethodPut, runPath, map[string]any{
		"expected_revision": runningResponse.Revision,
		"contract":          falsePass,
	}, agentID)
	requireAgentRunStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()

	resp = authRequest(t, http.MethodGet, runPath, nil)
	requireAgentRunStatus(t, resp, http.StatusOK)
	var afterFalsePass agentRunHTTPResponse
	readJSON(t, resp, &afterFalsePass)
	if afterFalsePass.Revision != 2 || afterFalsePass.Status != "running" {
		t.Fatalf(
			"false PASS mutated run: revision %d status %s",
			afterFalsePass.Revision,
			afterFalsePass.Status,
		)
	}

	passed := httpAgentRunContract(issueID, runID, agentID, "passed")
	resp = authRequestWithAgent(t, http.MethodPut, runPath, map[string]any{
		"expected_revision": afterFalsePass.Revision,
		"contract":          passed,
	}, agentID)
	requireAgentRunStatus(t, resp, http.StatusOK)
	var passedResponse agentRunHTTPResponse
	readJSON(t, resp, &passedResponse)
	if passedResponse.Revision != 3 || passedResponse.Status != "passed" {
		t.Fatalf(
			"passed run = revision %d status %s, want revision 3 passed",
			passedResponse.Revision,
			passedResponse.Status,
		)
	}

	resp = authRequest(t, http.MethodGet, "/api/issues/"+issueID, nil)
	requireAgentRunStatus(t, resp, http.StatusOK)
	var issue map[string]any
	readJSON(t, resp, &issue)
	if issue["status"] != "done" {
		t.Fatalf("reconciled issue status = %v, want done", issue["status"])
	}
}
