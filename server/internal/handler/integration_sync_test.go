package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestOutboundIssueSyncInFlightStatus(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{
			name:     "queued blocks duplicate outbound request",
			metadata: map[string]any{"sync_out_status": "queued"},
			want:     "queued",
		},
		{
			name:     "processing blocks duplicate outbound request",
			metadata: map[string]any{"sync_out_status": " processing "},
			want:     "processing",
		},
		{
			name:     "failed can be retried",
			metadata: map[string]any{"sync_out_status": "failed"},
			want:     "",
		},
		{
			name:     "missing status can be requested",
			metadata: map[string]any{},
			want:     "",
		},
		{
			name:     "non-string status is ignored",
			metadata: map[string]any{"sync_out_status": true},
			want:     "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := outboundIssueSyncInFlightStatus(tc.metadata); got != tc.want {
				t.Fatalf("outboundIssueSyncInFlightStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpsertIntegrationProjectBindingForcesIssueSyncDefaults(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/integrations/connections", map[string]any{
		"provider":     "gitlab",
		"name":         "gitlab-project-binding-defaults",
		"base_url":     "https://gitlab.example.com",
		"sync_enabled": true,
	})
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.CreateIntegrationConnection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIntegrationConnection: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var connection IntegrationConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&connection); err != nil {
		t.Fatalf("decode connection: %v", err)
	}
	defer func() {
		req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/integrations/connections/"+connection.ID, nil)
		req = withURLParams(req, "id", testWorkspaceID, "connectionId", connection.ID)
		testHandler.DeleteIntegrationConnection(httptest.NewRecorder(), req)
	}()

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Project binding defaults",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	defer func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	}()

	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/workspaces/"+testWorkspaceID+"/integrations/connections/"+connection.ID+"/project-bindings/"+project.ID, map[string]any{
		"external_ref": map[string]any{"path_with_namespace": "group/project"},
		// These are intentionally false: project binding direction is not a
		// user-facing control surface; account sync settings own that decision.
		"inbound_enabled":        false,
		"outbound_enabled":       false,
		"issue_sync_enabled":     false,
		"knowledge_sync_enabled": false,
	})
	req = withURLParams(req, "id", testWorkspaceID, "connectionId", connection.ID, "projectId", project.ID)
	testHandler.UpsertIntegrationProjectBinding(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertIntegrationProjectBinding: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var binding IntegrationProjectBindingResponse
	if err := json.NewDecoder(w.Body).Decode(&binding); err != nil {
		t.Fatalf("decode binding: %v", err)
	}
	if !binding.InboundEnabled || !binding.OutboundEnabled || !binding.IssueSyncEnabled {
		t.Fatalf("project binding sync defaults = inbound:%v outbound:%v issue:%v, want all true",
			binding.InboundEnabled, binding.OutboundEnabled, binding.IssueSyncEnabled)
	}
	if binding.KnowledgeSyncEnabled {
		t.Fatalf("knowledge sync should still follow the request body")
	}
}

func TestGitLabResourceReconcileCreatesSingleBindingForDuplicateRepo(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	var connID, firstProjectID, secondProjectID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
VALUES ($1, 'gitlab', 'gitlab-resource-reconcile', 'https://gitlab.dyvip.tech', 'active', now())
RETURNING id`, testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed gitlab connection: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id=$1`, connID) })
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'gitlab first project') RETURNING id`, testWorkspaceID).Scan(&firstProjectID); err != nil {
		t.Fatalf("seed first project: %v", err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'gitlab second project') RETURNING id`, testWorkspaceID).Scan(&secondProjectID); err != nil {
		t.Fatalf("seed second project: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM project WHERE id IN ($1, $2)`, firstProjectID, secondProjectID)
	})
	resourceRef := []byte(`{"url":"https://gitlab.dyvip.tech/chengnengliang/route-match"}`)
	if _, err := testPool.Exec(ctx, `
INSERT INTO project_resource (project_id, workspace_id, resource_type, resource_ref, created_by, created_at)
VALUES ($1, $3, 'github_repo', $4, $5, now() - interval '1 second'),
       ($2, $3, 'github_repo', $4, $5, now())`,
		firstProjectID, secondProjectID, testWorkspaceID, resourceRef, testUserID); err != nil {
		t.Fatalf("seed project resources: %v", err)
	}

	testHandler.reconcileGitLabRepoBindings(ctx)

	var count int
	var projectID string
	if err := testPool.QueryRow(ctx, `
SELECT count(*), min(project_id::text)
FROM integration_project_binding
WHERE connection_id=$1
  AND external_ref->>'path_with_namespace'='chengnengliang/route-match'
  AND inbound_enabled_at IS NOT NULL
  AND issue_sync_enabled_at IS NOT NULL`,
		connID).Scan(&count, &projectID); err != nil {
		t.Fatalf("count gitlab bindings: %v", err)
	}
	if count != 1 {
		t.Fatalf("gitlab binding count = %d, want 1", count)
	}
	if projectID != firstProjectID {
		t.Fatalf("gitlab binding project = %s, want earliest %s", projectID, firstProjectID)
	}
}

func TestGitLabInboundSourceScopeAvoidsIIDCollision(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	var connID, projectA, projectB string
	if err := testPool.QueryRow(ctx, `
INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
VALUES ($1, 'gitlab', 'gitlab-source-scope', 'https://gitlab.example.com', 'active', now())
RETURNING id`, testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed gitlab connection: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id=$1`, connID) })
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'gitlab scope A') RETURNING id`, testWorkspaceID).Scan(&projectA); err != nil {
		t.Fatalf("seed project A: %v", err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'gitlab scope B') RETURNING id`, testWorkspaceID).Scan(&projectB); err != nil {
		t.Fatalf("seed project B: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM project WHERE id IN ($1, $2)`, projectA, projectB)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='gitlab' AND metadata->>'source_id'='1'`, testWorkspaceID)
	})
	conn, err := testHandler.Queries.GetIntegrationConnectionInWorkspace(ctx, db.GetIntegrationConnectionInWorkspaceParams{
		ID:          parseUUID(connID),
		WorkspaceID: parseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("load gitlab connection: %v", err)
	}
	descA, descB := "issue A", "issue B"
	issueA, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, parseUUID(projectA), pgtype.UUID{},
		"GitLab IID 1 A", &descA, "issue", "1", integrationSourceScope(conn.ID, "group/a"), "https://gitlab.example.com/group/a/-/issues/1", "opened", "")
	if err != nil {
		t.Fatalf("upsert issue A: %v", err)
	}
	issueB, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, parseUUID(projectB), pgtype.UUID{},
		"GitLab IID 1 B", &descB, "issue", "1", integrationSourceScope(conn.ID, "group/b"), "https://gitlab.example.com/group/b/-/issues/1", "opened", "")
	if err != nil {
		t.Fatalf("upsert issue B: %v", err)
	}
	if uuidToString(issueA.ID) == uuidToString(issueB.ID) {
		t.Fatalf("scoped GitLab imports reused the same issue id %s", uuidToString(issueA.ID))
	}
	if metadataString(parseIssueMetadata(issueA.Metadata), "source_scope") == "" || metadataString(parseIssueMetadata(issueB.Metadata), "source_scope") == "" {
		t.Fatalf("expected source_scope metadata on both GitLab mirror issues")
	}
}

func TestLinkZentaoTaskParents(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zentao-parent-link")
	var projectID string
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, 'zentao parent link') RETURNING id`, testWorkspaceID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='zentao' AND metadata->>'source_id' IN ('ZT-PARENT-1', 'ZT-CHILD-1')`, testWorkspaceID)
	})

	parent, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, parseUUID(projectID), pgtype.UUID{},
		"ZenTao parent", nil, "task", "ZT-PARENT-1", "", "https://zentao/task-parent", "doing", "")
	if err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	child, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, parseUUID(projectID), pgtype.UUID{},
		"ZenTao child", nil, "task", "ZT-CHILD-1", "", "https://zentao/task-child", "doing", "")
	if err != nil {
		t.Fatalf("upsert child: %v", err)
	}
	testHandler.linkZentaoTaskParents(ctx, conn, map[string]db.Issue{
		"ZT-PARENT-1": parent,
		"ZT-CHILD-1":  child,
	}, map[string]string{"ZT-CHILD-1": "ZT-PARENT-1"})

	got, err := testHandler.Queries.GetIssue(ctx, child.ID)
	if err != nil {
		t.Fatalf("reload child: %v", err)
	}
	if uuidToString(got.ParentIssueID) != uuidToString(parent.ID) {
		t.Fatalf("child parent_issue_id = %s, want %s", uuidToString(got.ParentIssueID), uuidToString(parent.ID))
	}
}

func TestInboundExternalStatusCreatesIssueWithMappedStatus(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zentao-inbound-status-create")
	projectID := seedIntegrationStatusProject(t, ctx, "zentao inbound status create")
	const sourceID = "ZT-STATUS-CREATE-1"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='zentao' AND metadata->>'source_id'=$2`, testWorkspaceID, sourceID)
	})

	issue, status, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao finished task", nil, "task", sourceID, "", "https://zentao/task-status-create", "done", "2026-06-29T08:00:00Z")
	if err != nil {
		t.Fatalf("upsert issue: %v", err)
	}
	if status != "created" {
		t.Fatalf("sync status = %q, want created", status)
	}
	if issue.Status != "done" {
		t.Fatalf("issue status = %q, want done", issue.Status)
	}
	metadata := parseIssueMetadata(issue.Metadata)
	if got := metadataString(metadata, "external_status"); got != "done" {
		t.Fatalf("external_status = %q, want done", got)
	}
	if got := metadataString(metadata, "external_status_updated_at"); got != "2026-06-29T08:00:00Z" {
		t.Fatalf("external_status_updated_at = %q, want 2026-06-29T08:00:00Z", got)
	}
}

func TestInboundExternalStatusNewerThanLocalStatusWins(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zentao-inbound-status-newer")
	projectID := seedIntegrationStatusProject(t, ctx, "zentao inbound status newer")
	const sourceID = "ZT-STATUS-NEWER-1"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='zentao' AND metadata->>'source_id'=$2`, testWorkspaceID, sourceID)
	})

	issue, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao status task", nil, "task", sourceID, "", "https://zentao/task-status-newer", "wait", "2026-06-29T08:00:00Z")
	if err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	setIssueStatusWithActivityAt(t, ctx, issue, "todo", "in_progress", time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))

	updated, _, event, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao status task", nil, "task", sourceID, "", "https://zentao/task-status-newer", "done", "2026-06-29T10:00:00Z")
	if err != nil {
		t.Fatalf("update mirror: %v", err)
	}
	if updated.Status != "done" {
		t.Fatalf("issue status = %q, want done", updated.Status)
	}
	eventMetadata := parseIssueMetadata(event.Metadata)
	if applied, _ := eventMetadata["inbound_status_applied"].(bool); !applied {
		t.Fatalf("inbound_status_applied = %v, want true", eventMetadata["inbound_status_applied"])
	}
}

func TestInboundExternalStatusWinsWithoutLocalStatusActivity(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zentao-inbound-status-no-local-activity")
	projectID := seedIntegrationStatusProject(t, ctx, "zentao inbound no local activity")
	const sourceID = "ZT-STATUS-NO-LOCAL-1"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='zentao' AND metadata->>'source_id'=$2`, testWorkspaceID, sourceID)
	})

	issue, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao no-local-activity task", nil, "task", sourceID, "", "https://zentao/task-status-no-local", "wait", "2026-06-29T08:00:00Z")
	if err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	if _, err := testPool.Exec(ctx, `DELETE FROM activity_log WHERE issue_id=$1 AND action='status_changed'`, issue.ID); err != nil {
		t.Fatalf("clear status activity: %v", err)
	}

	updated, _, event, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao no-local-activity task", nil, "task", sourceID, "", "https://zentao/task-status-no-local", "done", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("update mirror: %v", err)
	}
	if updated.Status != "done" {
		t.Fatalf("issue status = %q, want done", updated.Status)
	}
	eventMetadata := parseIssueMetadata(event.Metadata)
	if applied, _ := eventMetadata["inbound_status_applied"].(bool); !applied {
		t.Fatalf("inbound_status_applied = %v, want true", eventMetadata["inbound_status_applied"])
	}
}

func TestInboundExternalStatusOlderThanLocalStatusDoesNotWin(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	ctx := context.Background()
	conn := seedZentaoConnection(t, ctx, "zentao-inbound-status-local-newer")
	projectID := seedIntegrationStatusProject(t, ctx, "zentao inbound status local newer")
	const sourceID = "ZT-STATUS-LOCAL-NEWER-1"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id=$1 AND metadata->>'source_system'='zentao' AND metadata->>'source_id'=$2`, testWorkspaceID, sourceID)
	})

	issue, _, _, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao local-newer task", nil, "task", sourceID, "", "https://zentao/task-status-local-newer", "wait", "2026-06-29T08:00:00Z")
	if err != nil {
		t.Fatalf("create mirror: %v", err)
	}
	setIssueStatusWithActivityAt(t, ctx, issue, "todo", "in_progress", time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC))

	updated, _, event, err := testHandler.upsertInboundIntegrationIssue(
		ctx, conn, parseUUID(testUserID), pgtype.UUID{}, projectID, pgtype.UUID{},
		"ZenTao local-newer task", nil, "task", sourceID, "", "https://zentao/task-status-local-newer", "done", "2026-06-29T09:00:00Z")
	if err != nil {
		t.Fatalf("update mirror: %v", err)
	}
	if updated.Status != "in_progress" {
		t.Fatalf("issue status = %q, want in_progress", updated.Status)
	}
	metadata := parseIssueMetadata(updated.Metadata)
	if got := metadataString(metadata, "external_status"); got != "done" {
		t.Fatalf("external_status = %q, want done", got)
	}
	eventMetadata := parseIssueMetadata(event.Metadata)
	if applied, _ := eventMetadata["inbound_status_applied"].(bool); applied {
		t.Fatalf("inbound_status_applied = true, want false")
	}
}

func seedIntegrationStatusProject(t *testing.T, ctx context.Context, title string) pgtype.UUID {
	t.Helper()
	var projectID string
	if err := testPool.QueryRow(ctx, `INSERT INTO project (workspace_id, title) VALUES ($1, $2) RETURNING id`, testWorkspaceID, title).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM project WHERE id=$1`, projectID) })
	return parseUUID(projectID)
}

func setIssueStatusWithActivityAt(t *testing.T, ctx context.Context, issue db.Issue, fromStatus, toStatus string, changedAt time.Time) {
	t.Helper()
	if _, err := testPool.Exec(ctx, `UPDATE issue SET status=$1, updated_at=$2 WHERE id=$3 AND workspace_id=$4`, toStatus, changedAt, issue.ID, issue.WorkspaceID); err != nil {
		t.Fatalf("set issue status: %v", err)
	}
	details, _ := json.Marshal(map[string]string{"from": fromStatus, "to": toStatus})
	if _, err := testPool.Exec(ctx, `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details, created_at)
VALUES ($1, $2, 'member', $3, 'status_changed', $4, $5)`,
		issue.WorkspaceID, issue.ID, parseUUID(testUserID), details, changedAt); err != nil {
		t.Fatalf("insert status activity: %v", err)
	}
}

func TestSyncInboundIntegrationIssueCreatesAndUpdatesExternalMirror(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	suffix := time.Now().UnixNano()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/integrations/connections", map[string]any{
		"provider":     "zentao",
		"name":         "zentao-sync-test",
		"base_url":     "https://zentao.example.com",
		"sync_enabled": true,
	})
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.CreateIntegrationConnection(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIntegrationConnection: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var connection IntegrationConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&connection); err != nil {
		t.Fatalf("decode connection: %v", err)
	}
	defer func() {
		req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/integrations/connections/"+connection.ID, nil)
		req = withURLParams(req, "id", testWorkspaceID, "connectionId", connection.ID)
		testHandler.DeleteIntegrationConnection(httptest.NewRecorder(), req)
	}()

	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/workspaces/"+testWorkspaceID+"/integrations/issue-sync/zentao", map[string]any{
		"inbound_enabled":  true,
		"outbound_enabled": false,
	})
	req = withURLParams(req, "id", testWorkspaceID, "provider", "zentao")
	testHandler.UpsertIntegrationIssueSyncSetting(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertIntegrationIssueSyncSetting: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "ZenTao sync project",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	defer func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	}()

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/resources", map[string]any{
		"resource_type": "zentao_project",
		"resource_ref": map[string]any{
			"project_id": "zt-project-1",
			"url":        "https://zentao.example.com/project/1",
		},
	})
	req = withURLParam(req, "id", project.ID)
	testHandler.CreateProjectResource(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProjectResource: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resource ProjectResourceResponse
	if err := json.NewDecoder(w.Body).Decode(&resource); err != nil {
		t.Fatalf("decode resource: %v", err)
	}

	sourceID := "zentao-bug-" + time.Unix(0, suffix).Format("150405.000000000")
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/integrations/connections/"+connection.ID+"/issues/inbound", map[string]any{
		"title":           "ZenTao mirrored bug",
		"description":     "Imported from ZenTao",
		"source_type":     "bug",
		"source_id":       sourceID,
		"source_url":      "https://zentao.example.com/bug/" + sourceID,
		"external_status": "active",
		"project_id":      project.ID,
		"resource_id":     resource.ID,
	})
	req = withURLParams(req, "id", testWorkspaceID, "connectionId", connection.ID)
	testHandler.SyncInboundIntegrationIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SyncInboundIntegrationIssue create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var created SyncInboundIntegrationIssueResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode created sync response: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("created.Status = %q, want created", created.Status)
	}
	if created.Issue.AssigneeID == nil || *created.Issue.AssigneeID != testUserID {
		t.Fatalf("created issue assignee = %v, want test user", created.Issue.AssigneeID)
	}
	if got := created.Metadata["source_system"]; got != "zentao" {
		t.Fatalf("source_system = %v, want zentao", got)
	}
	defer func() {
		req := newRequest("DELETE", "/api/issues/"+created.Issue.ID, nil)
		req = withURLParam(req, "id", created.Issue.ID)
		testHandler.DeleteIssue(httptest.NewRecorder(), req)
	}()

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/integrations/connections/"+connection.ID+"/issues/inbound", map[string]any{
		"title":           "ZenTao mirrored bug updated",
		"description":     "Imported from ZenTao again",
		"source_type":     "bug",
		"source_id":       sourceID,
		"source_url":      "https://zentao.example.com/bug/" + sourceID,
		"external_status": "resolved",
		"project_id":      project.ID,
		"resource_id":     resource.ID,
	})
	req = withURLParams(req, "id", testWorkspaceID, "connectionId", connection.ID)
	testHandler.SyncInboundIntegrationIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SyncInboundIntegrationIssue update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated SyncInboundIntegrationIssueResponse
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated sync response: %v", err)
	}
	if updated.Status != "updated" {
		t.Fatalf("updated.Status = %q, want updated", updated.Status)
	}
	if updated.Issue.ID != created.Issue.ID {
		t.Fatalf("updated issue id = %s, want %s", updated.Issue.ID, created.Issue.ID)
	}
	if updated.Issue.Title != "ZenTao mirrored bug updated" {
		t.Fatalf("updated title = %q", updated.Issue.Title)
	}
	if got := updated.Metadata["external_status"]; got != "resolved" {
		t.Fatalf("external_status = %v, want resolved", got)
	}
}
