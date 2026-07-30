package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/agentrun"
)

const cliAgentRunSHA = "7326e0ce5cca7258c2ba304c934656811e614b94739d603485d090472bc5bf68"

func validCLIAgentRunContract() agentrun.Contract {
	return agentrun.Contract{
		Schema:                 agentrun.ProtocolVersion,
		ProtocolPackageVersion: agentrun.ProtocolPackageVersion,
		ProtocolSHA256:         cliAgentRunSHA,
		RunID:                  "run-1",
		DispatchAuthority: agentrun.DispatchAuthority{
			System: "multica",
			Actor:  "22222222-2222-2222-2222-222222222222",
		},
		SourceRef:     "MUL-1",
		Tier:          "S",
		Objective:     "Verify the protocol path",
		WorkspaceMode: "worktree",
		Status:        "draft",
		Steps: []agentrun.Step{
			{
				StepID:   "implement",
				Role:     "implementer",
				Executor: "33333333-3333-3333-3333-333333333333",
				Status:   "planned",
				Scope: agentrun.Scope{
					Workspace:      "/repo",
					WritablePaths:  []string{"server/internal/agentrun/**"},
					ForbiddenPaths: []string{},
					ExternalWrites: false,
				},
				Acceptance: []agentrun.Acceptance{
					{ID: "tests", Check: "unit tests pass"},
				},
				Verification: []string{"go test ./internal/agentrun"},
				Evidence:     []agentrun.Evidence{},
			},
		},
		Review: agentrun.Review{
			Required:  false,
			Cycle:     0,
			MaxCycles: 3,
		},
	}
}

func newAgentRunCreateTestCmd(t *testing.T, contract agentrun.Contract) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "create"}
	addAgentRunContractFlags(cmd)
	cmd.Flags().String("issue-status-mode", "follow_run", "")
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("contract", string(raw)); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func newAgentRunUpdateTestCmd(t *testing.T, contract agentrun.Contract, revision string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "update"}
	addAgentRunContractFlags(cmd)
	cmd.Flags().Int("expected-revision", 0, "")
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("contract", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("expected-revision", revision); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func agentRunCLITestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+testIssueUUID {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         testIssueUUID,
				"identifier": "MUL-1",
				"title":      "protocol test",
			})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	t.Setenv("MULTICA_SERVER_URL", server.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "44444444-4444-4444-4444-444444444444")
	t.Setenv("MULTICA_TOKEN", "mat_test")
	t.Setenv("MULTICA_AGENT_ID", "22222222-2222-2222-2222-222222222222")
	t.Setenv("MULTICA_TASK_ID", "55555555-5555-5555-5555-555555555555")
	return server
}

func TestRunIssueAgentRunCreateSendsValidatedContractAndIdentity(t *testing.T) {
	contract := validCLIAgentRunContract()
	var received map[string]any
	agentRunCLITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/issues/"+testIssueUUID+"/agent-runs" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-Agent-ID"); got != contract.DispatchAuthority.Actor {
			t.Errorf("X-Agent-ID = %q, want %q", got, contract.DispatchAuthority.Actor)
		}
		if got := r.Header.Get("X-Task-ID"); got == "" {
			t.Error("X-Task-ID is empty")
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id":   contract.RunID,
			"revision": 1,
			"status":   contract.Status,
		})
	})

	output, err := captureStdout(t, func() error {
		return runIssueAgentRunCreate(newAgentRunCreateTestCmd(t, contract), []string{testIssueUUID})
	})
	if err != nil {
		t.Fatalf("create agent run: %v", err)
	}
	if received["issue_status_mode"] != "follow_run" {
		t.Fatalf("issue_status_mode = %#v", received["issue_status_mode"])
	}
	if !strings.Contains(output, `"run_id": "run-1"`) {
		t.Fatalf("unexpected stdout: %s", output)
	}
}

func TestRunIssueAgentRunUpdateSendsExpectedRevision(t *testing.T) {
	contract := validCLIAgentRunContract()
	contract.Status = "running"
	contract.Steps[0].Status = "running"
	contract.ActiveWorkers = []string{contract.Steps[0].Executor}

	var received struct {
		ExpectedRevision int               `json:"expected_revision"`
		Contract         agentrun.Contract `json:"contract"`
	}
	agentRunCLITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/api/issues/" + testIssueUUID + "/agent-runs/run-1"
		if r.Method != http.MethodPut || r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id":   contract.RunID,
			"revision": 2,
			"status":   contract.Status,
		})
	})

	if _, err := captureStdout(t, func() error {
		return runIssueAgentRunUpdate(newAgentRunUpdateTestCmd(t, contract, "1"), []string{testIssueUUID})
	}); err != nil {
		t.Fatalf("update agent run: %v", err)
	}
	if received.ExpectedRevision != 1 {
		t.Fatalf("expected_revision = %d, want 1", received.ExpectedRevision)
	}
	if received.Contract.Status != "running" {
		t.Fatalf("contract status = %q, want running", received.Contract.Status)
	}
}

func TestReadAgentRunContractRejectsUnknownField(t *testing.T) {
	cmd := &cobra.Command{Use: "create"}
	addAgentRunContractFlags(cmd)
	if err := cmd.Flags().Set("contract", `{"schema":"agent-run/v1","unknown":true}`); err != nil {
		t.Fatal(err)
	}
	if _, err := readAgentRunContract(cmd); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestRunIssueAgentRunUpdateRequiresRevision(t *testing.T) {
	cmd := newAgentRunUpdateTestCmd(t, validCLIAgentRunContract(), "0")
	err := runIssueAgentRunUpdate(cmd, []string{testIssueUUID})
	if err == nil || !strings.Contains(err.Error(), "expected-revision") {
		t.Fatalf("expected revision error, got %v", err)
	}
}
