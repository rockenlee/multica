package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAgentResourceE2E_ClaimSurfacesProjectResources is the live-path Agent E2E
// proof for the "a Project references resources -> an Agent can identify them"
// goal. Unlike the narrower repo-override tests, it asserts the FULL agent
// claim contract against the REAL claim endpoint (the same handler the daemon
// calls):
//
//  1. a usable agent + an ONLINE runtime exist,
//  2. a real queued task is claimable and gets claimed (HTTP 200, task != null),
//  3. the agent-facing claim payload carries EVERY one of the project's
//     referenced resources (all provider types), and
//  4. each referenced resource exposes a concrete read/sync handle
//     (resource_ref url/token/id) — the exact metadata the agent passes to
//     `multica project resource fetch` to read/sync latest info.
//
// It is hermetic: it seeds everything in the test database and cleans up after
// itself, so QA can run it against a throwaway DB without touching real data.
// Run in isolation so the seeded task is the one claimed:
//
//	DATABASE_URL=postgres://multica:<pw>@localhost:5432/multica_e2e?sslmode=disable \
//	  go test ./internal/handler -run TestAgentResourceE2E -count=1 -v
func TestAgentResourceE2E_ClaimSurfacesProjectResources(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	// --- Usable agent + ONLINE runtime (strict standard requires online). ---
	var agentID, runtimeID string
	if err := testPool.QueryRow(ctx,
		`SELECT id, runtime_id FROM agent WHERE workspace_id = $1 LIMIT 1`,
		testWorkspaceID).Scan(&agentID, &runtimeID); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'online', last_seen_at = now() WHERE id = $1`,
		runtimeID); err != nil {
		t.Fatalf("mark runtime online: %v", err)
	}
	var rtStatus string
	var heartbeatAgeSecs float64
	if err := testPool.QueryRow(ctx,
		`SELECT status, EXTRACT(EPOCH FROM (now() - last_seen_at)) FROM agent_runtime WHERE id = $1`,
		runtimeID).Scan(&rtStatus, &heartbeatAgeSecs); err != nil {
		t.Fatalf("read runtime: %v", err)
	}
	if rtStatus != "online" || heartbeatAgeSecs > 60 {
		t.Fatalf("runtime not online: status=%q heartbeatAge=%.0fs", rtStatus, heartbeatAgeSecs)
	}
	t.Logf("RUNTIME ONLINE: id=%s status=%s heartbeat_age=%.1fs", runtimeID, rtStatus, heartbeatAgeSecs)

	// --- Project referencing the same provider resource types QA uses. ---
	var projectID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO project (workspace_id, title) VALUES ($1, $2) RETURNING id`,
		testWorkspaceID, "Agent Resource E2E").Scan(&projectID); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id = $1`, projectID) })

	resources := []struct{ typ, ref string }{
		{"gitlab_repo", `{"url":"https://gitlab.hbc.tech/litianyi/multica-demo"}`},
		{"github_repo", `{"url":"https://github.com/example/agent-e2e-repo"}`},
		{"feishu_drive", `{"drive_url":"https://example.feishu.cn/drive/folder/E2E","label":"E2E Docs"}`},
		{"feishu_wiki", `{"wiki_url":"https://example.feishu.cn/wiki/E2E"}`},
		{"zentao_project", `{"project_id":"447"}`},
	}
	for i, r := range resources {
		if _, err := testPool.Exec(ctx,
			`INSERT INTO project_resource (project_id, workspace_id, resource_type, resource_ref, position)
			 VALUES ($1, $2, $3, $4::jsonb, $5)`,
			projectID, testWorkspaceID, r.typ, r.ref, i); err != nil {
			t.Fatalf("create project_resource %s: %v", r.typ, err)
		}
	}

	// --- A real claimable task: issue assigned to the agent, queued for the runtime. ---
	var issueID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO issue (workspace_id, project_id, title, status, priority, creator_id, creator_type, number, position)
		 VALUES ($1, $2, 'agent resource e2e', 'todo', 'medium', $3, 'member', 88011, 0) RETURNING id`,
		testWorkspaceID, projectID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })

	var taskID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority)
		 VALUES ($1, $2, $3, 'queued', 0) RETURNING id`,
		agentID, runtimeID, issueID).Scan(&taskID); err != nil {
		t.Fatalf("create task: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })

	// --- Drive the REAL claim endpoint (the same handler the daemon calls). ---
	w := httptest.NewRecorder()
	req := newDaemonTokenRequest("POST", "/api/daemon/runtimes/"+runtimeID+"/claim", nil, testWorkspaceID, "agent-resource-e2e")
	req = withURLParams(req, "runtimeId", runtimeID)
	testHandler.ClaimTaskByRuntime(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("claim failed: %d %s", w.Code, w.Body.String())
	}

	var resp struct {
		Task *struct {
			ProjectID        string                `json:"project_id"`
			ProjectResources []ProjectResourceData `json:"project_resources"`
		} `json:"task"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if resp.Task == nil {
		t.Fatal("no task claimed (claim payload had null task)")
	}
	if resp.Task.ProjectID != projectID {
		t.Fatalf("claimed a different task: project_id=%q want %q", resp.Task.ProjectID, projectID)
	}
	t.Logf("TASK CLAIMED: task=%s project=%s", taskID, resp.Task.ProjectID)

	// --- The agent-facing claim payload must identify EVERY project resource... ---
	if len(resp.Task.ProjectResources) != len(resources) {
		t.Fatalf("claim payload surfaced %d project_resources, want %d: %+v",
			len(resp.Task.ProjectResources), len(resources), resp.Task.ProjectResources)
	}

	// --- ...each with a concrete read/sync handle (the metadata the agent uses
	//     with `multica project resource fetch`). ---
	readSyncKeys := []string{"url", "drive_url", "wiki_url", "project_id", "folder_token", "space_id", "product_id"}
	withHandle := 0
	for _, pr := range resp.Task.ProjectResources {
		var ref map[string]any
		if err := json.Unmarshal(pr.ResourceRef, &ref); err != nil {
			t.Fatalf("resource %s ref is not JSON: %v", pr.ResourceType, err)
		}
		handle := ""
		for _, k := range readSyncKeys {
			if v, ok := ref[k].(string); ok && strings.TrimSpace(v) != "" {
				handle = k + "=" + v
				break
			}
		}
		if handle == "" {
			t.Errorf("resource %s exposes no read/sync handle: %s", pr.ResourceType, string(pr.ResourceRef))
			continue
		}
		withHandle++
		t.Logf("AGENT-CONTEXT RESOURCE: type=%-14s handle=%s", pr.ResourceType, handle)
	}
	if withHandle < 1 {
		t.Fatal("no referenced resource exposed read/sync metadata to the agent context")
	}
	t.Logf("PASS: online runtime claimed a task; agent context carries %d/%d project resources, %d with read/sync handles",
		len(resp.Task.ProjectResources), len(resources), withHandle)
}
