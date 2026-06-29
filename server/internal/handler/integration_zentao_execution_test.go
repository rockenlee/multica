package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// TestZentaoTargetFromRef covers the resource_ref / external_ref parser that
// resolves a ZenTao project vs. execution (kanban) target. The execution-kanban
// URL case is the #-bug: a zentao_project resource created from a kanban page
// used to parse to an empty id, so content fetch returned 400 "missing a
// project id" and inbound sync skipped it.
func TestZentaoTargetFromRef(t *testing.T) {
	cases := []struct {
		name     string
		ref      string
		wantID   string
		wantExec bool
	}{
		{
			name:     "execution kanban url (the bug)",
			ref:      `{"url":"https://chandao.huangbaoche.com/zentao/execution-kanban-447.html"}`,
			wantID:   "447",
			wantExec: true,
		},
		{
			name:     "execution task url",
			ref:      `{"url":"https://host/zentao/execution-task-512-0-id_asc-.html"}`,
			wantID:   "512",
			wantExec: true,
		},
		{
			name:     "explicit execution_id key",
			ref:      `{"execution_id":"447"}`,
			wantID:   "447",
			wantExec: true,
		},
		{
			name:     "explicit execution_id wins over project_id",
			ref:      `{"project_id":"9","execution_id":"447"}`,
			wantID:   "447",
			wantExec: true,
		},
		{
			name:     "project_id key (unchanged)",
			ref:      `{"project_id":"446"}`,
			wantID:   "446",
			wantExec: false,
		},
		{
			name:     "numeric project_id (unchanged)",
			ref:      `{"project_id":446}`,
			wantID:   "446",
			wantExec: false,
		},
		{
			name:     "project_key key (unchanged)",
			ref:      `{"project_key":"PROD"}`,
			wantID:   "PROD",
			wantExec: false,
		},
		{
			name:     "project-index url (unchanged)",
			ref:      `{"url":"https://host/zentao/project-index-446.html"}`,
			wantID:   "446",
			wantExec: false,
		},
		{
			name:     "non-zentao url",
			ref:      `{"url":"https://host/zentao/foo.html"}`,
			wantID:   "",
			wantExec: false,
		},
		{
			name:     "empty ref",
			ref:      ``,
			wantID:   "",
			wantExec: false,
		},
		{
			name:     "invalid json",
			ref:      `not-json`,
			wantID:   "",
			wantExec: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := zentaoTargetFromRef([]byte(tc.ref))
			if got.ID != tc.wantID {
				t.Fatalf("zentaoTargetFromRef(%s).ID = %q, want %q", tc.ref, got.ID, tc.wantID)
			}
			if got.IsExecution != tc.wantExec {
				t.Fatalf("zentaoTargetFromRef(%s).IsExecution = %v, want %v", tc.ref, got.IsExecution, tc.wantExec)
			}
			// zentaoProjectIDFromRef (used by inbound/outbound sync) must surface
			// the same id, so the execution id flows into ListTasks/CreateTask.
			if id := zentaoProjectIDFromRef([]byte(tc.ref)); id != tc.wantID {
				t.Fatalf("zentaoProjectIDFromRef(%s) = %q, want %q", tc.ref, id, tc.wantID)
			}
		})
	}
}

// TestZentaoExecutionResourceDrivesInboundBinding proves the end-to-end fix:
// creating a zentao_project resource that points at a ZenTao execution kanban,
// in a workspace with exactly one active ZenTao connection, auto-creates an
// inbound-enabled integration_project_binding whose external_ref resolves the
// execution id — so ListInboundSyncTargets (which only reads bindings) now
// includes it. Updating the resource repoints the binding. Content fetch no
// longer fails at the parse step.
func TestZentaoExecutionResourceDrivesInboundBinding(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	// One active ZenTao connection in the workspace, with a base URL that points
	// at a closed port so the content-fetch HTTP call fails fast (we only assert
	// it gets past the parse step).
	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'zentao', 'zt-exec-binding-test', 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID)
	})

	// Project to attach the resource to.
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "ZenTao execution kanban project",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	})

	// Create the zentao_project resource from the execution kanban URL.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/resources", map[string]any{
		"resource_type": "zentao_project",
		"resource_ref": map[string]any{
			"url": "https://chandao.huangbaoche.com/zentao/execution-kanban-447.html",
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

	// The binding must have been auto-created, inbound-enabled, with an
	// external_ref that resolves execution 447.
	bindingRef, inboundEnabled := loadZentaoBinding(t, ctx, project.ID, connID)
	if !inboundEnabled {
		t.Fatalf("auto-created binding is not inbound-enabled")
	}
	if id := zentaoProjectIDFromRef(bindingRef); id != "447" {
		t.Fatalf("binding external_ref %s resolves to %q, want 447", string(bindingRef), id)
	}

	// Seed the issue-sync setting + a stored credential so ListInboundSyncTargets'
	// full join is satisfied, then confirm the binding now surfaces as a target.
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_issue_sync_setting (workspace_id, user_id, provider, inbound_enabled_at)
		 VALUES ($1, $2, 'zentao', now())
		 ON CONFLICT (workspace_id, user_id, provider) DO UPDATE SET inbound_enabled_at = now()`,
		testWorkspaceID, testUserID); err != nil {
		t.Fatalf("seed issue sync setting: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_sync_setting WHERE workspace_id = $1 AND user_id = $2 AND provider = 'zentao'`, testWorkspaceID, testUserID)
	})
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_user_account (workspace_id, connection_id, user_id, account_key, account_name, credential_encrypted, status)
		 VALUES ($1, $2, $3, 'zt-exec-acct', 'zt exec acct', $4, 'active')`,
		testWorkspaceID, connID, testUserID, []byte("sealed-credential")); err != nil {
		t.Fatalf("seed user account: %v", err)
	}

	targets, err := testHandler.ListInboundSyncTargets(ctx)
	if err != nil {
		t.Fatalf("ListInboundSyncTargets: %v", err)
	}
	found := false
	for _, tgt := range targets {
		if uuidToString(tgt.ProjectID) == project.ID && uuidToString(tgt.ConnectionID) == connID {
			found = true
			if id := zentaoProjectIDFromRef(tgt.ExternalRef); id != "447" {
				t.Fatalf("inbound target external_ref resolves to %q, want 447", id)
			}
		}
	}
	if !found {
		t.Fatalf("ListInboundSyncTargets did not include the auto-created execution binding")
	}

	// Updating the resource to a different execution repoints the binding.
	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/projects/"+project.ID+"/resources/"+resource.ID, map[string]any{
		"resource_ref": map[string]any{
			"url": "https://chandao.huangbaoche.com/zentao/execution-kanban-512.html",
		},
	})
	req = withURLParams(req, "id", project.ID, "resourceId", resource.ID)
	testHandler.UpdateProjectResource(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateProjectResource: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	bindingRef, _ = loadZentaoBinding(t, ctx, project.ID, connID)
	if id := zentaoProjectIDFromRef(bindingRef); id != "512" {
		t.Fatalf("after update, binding resolves to %q, want 512", id)
	}

	// Content fetch must get past the parse step. With a real secret box + sealed
	// credential it reaches the execution read against the closed base URL and
	// fails with 502 — never the old 400 "missing a project id".
	key := make([]byte, secretbox.KeySize)
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatalf("secretbox.New: %v", err)
	}
	sealed, err := box.Seal([]byte(`{"token":"t"}`))
	if err != nil {
		t.Fatalf("seal credential: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE integration_user_account SET credential_encrypted = $1 WHERE connection_id = $2 AND user_id = $3`,
		sealed, connID, testUserID); err != nil {
		t.Fatalf("update sealed credential: %v", err)
	}
	prevSecrets := testHandler.IntegrationSecrets
	testHandler.IntegrationSecrets = box
	t.Cleanup(func() { testHandler.IntegrationSecrets = prevSecrets })

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/projects/"+project.ID+"/resources/"+resource.ID+"/content", nil)
	req = withURLParams(req, "id", project.ID, "resourceId", resource.ID)
	testHandler.FetchProjectResourceContent(w, req)
	if w.Code == http.StatusBadRequest {
		t.Fatalf("FetchProjectResourceContent returned 400 (parse step still failing): %s", w.Body.String())
	}
	if w.Code != http.StatusBadGateway {
		t.Fatalf("FetchProjectResourceContent: expected 502 (parse passed, HTTP failed), got %d: %s", w.Code, w.Body.String())
	}
}

// TestValidateZenTaoProjectRefExecutionID covers requirement A: the
// zentao_project resource-ref validator must accept an explicit execution_id
// (a resource pinned at a ZenTao execution/kanban without the page URL), while
// still accepting the legacy project_id/project_key/url forms and rejecting an
// empty ref.
func TestValidateZenTaoProjectRefExecutionID(t *testing.T) {
	cases := []struct {
		name    string
		ref     string
		wantErr bool
		wantID  string // resolved id via zentaoTargetFromRef, when no error
		wantExe bool
	}{
		{name: "explicit execution_id", ref: `{"execution_id":"447"}`, wantID: "447", wantExe: true},
		{name: "execution_id with whitespace", ref: `{"execution_id":" 447 "}`, wantID: "447", wantExe: true},
		{name: "legacy project_id still ok", ref: `{"project_id":"446"}`, wantID: "446"},
		{name: "legacy url still ok", ref: `{"url":"https://host/zentao/project-index-446.html"}`, wantID: "446"},
		{name: "empty object rejected", ref: `{}`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := validateZenTaoProjectRef([]byte(tc.ref))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateZenTaoProjectRef(%s) = %s, want error", tc.ref, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateZenTaoProjectRef(%s) unexpected error: %v", tc.ref, err)
			}
			got := zentaoTargetFromRef(out)
			if got.ID != tc.wantID {
				t.Fatalf("normalized ref %s resolves to id %q, want %q", out, got.ID, tc.wantID)
			}
			if got.IsExecution != tc.wantExe {
				t.Fatalf("normalized ref %s IsExecution = %v, want %v", out, got.IsExecution, tc.wantExe)
			}
		})
	}
}

// TestZentaoBindingClearedWhenRepointedAwayFromExecution covers requirement B:
// when a zentao_project resource is repointed from an execution target to a
// non-execution target, the auto-created inbound flag is cleared so the old
// execution stops being polled — while a manually enabled outbound flag is
// preserved.
func TestZentaoBindingClearedWhenRepointedAwayFromExecution(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'zentao', 'zt-exec-teardown-test', 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "ZenTao execution teardown project",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	})

	// Create the resource from an execution kanban URL — auto-creates the
	// inbound binding.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/resources", map[string]any{
		"resource_type": "zentao_project",
		"resource_ref":  map[string]any{"url": "https://chandao.huangbaoche.com/zentao/execution-kanban-447.html"},
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
	if _, inbound := loadZentaoBinding(t, ctx, project.ID, connID); !inbound {
		t.Fatalf("expected auto-created binding to be inbound-enabled")
	}

	// Simulate a manually enabled outbound flag on the same binding; the
	// tear-down must preserve it.
	if _, err := testPool.Exec(ctx,
		`UPDATE integration_project_binding SET outbound_enabled_at = now() WHERE project_id = $1 AND connection_id = $2`,
		project.ID, connID); err != nil {
		t.Fatalf("seed outbound flag: %v", err)
	}

	// Repoint the resource at a plain (non-execution) project.
	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/projects/"+project.ID+"/resources/"+resource.ID, map[string]any{
		"resource_ref": map[string]any{"project_id": "446"},
	})
	req = withURLParams(req, "id", project.ID, "resourceId", resource.ID)
	testHandler.UpdateProjectResource(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateProjectResource: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Inbound must now be cleared; outbound must remain; external_ref repointed.
	var inboundAt, outboundAt *time.Time
	var ref []byte
	if err := testPool.QueryRow(ctx,
		`SELECT inbound_enabled_at, outbound_enabled_at, external_ref FROM integration_project_binding WHERE project_id = $1 AND connection_id = $2`,
		project.ID, connID,
	).Scan(&inboundAt, &outboundAt, &ref); err != nil {
		t.Fatalf("load binding after repoint: %v", err)
	}
	if inboundAt != nil {
		t.Fatalf("inbound flag was not cleared after repoint away from execution")
	}
	if outboundAt == nil {
		t.Fatalf("manually enabled outbound flag was not preserved")
	}
	if id := zentaoProjectIDFromRef(ref); id != "446" {
		t.Fatalf("binding external_ref %s resolves to %q, want 446", string(ref), id)
	}
}

// TestZentaoExecutionResourceReconciledIntoInboundTarget covers the backfill
// path: a zentao_project resource that resolves to a ZenTao execution is
// inserted directly (simulating a row that predates the create/update
// auto-binding) with NO integration_project_binding. Without calling
// CreateProjectResource/UpdateProjectResource, ListInboundSyncTargets must
// reconcile it into an inbound-enabled binding and surface it as a target, in a
// workspace with exactly one active ZenTao connection.
func TestZentaoExecutionResourceReconciledIntoInboundTarget(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'zentao', 'zt-exec-reconcile-test', 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID)
	})

	// Project to attach the resource to (CreateProject is allowed; only
	// CreateProjectResource/UpdateProjectResource are off-limits for this test).
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "ZenTao execution reconcile project",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	})

	// Insert the execution-resolving resource directly — bypassing the handler so
	// no auto-binding runs at insert time. No binding exists yet.
	execRef := []byte(`{"url":"https://chandao.huangbaoche.com/zentao/execution-kanban-447.html"}`)
	if _, err := testPool.Exec(ctx,
		`INSERT INTO project_resource (project_id, workspace_id, resource_type, resource_ref, position, created_by)
		 VALUES ($1, $2, 'zentao_project', $3, 0, $4)`,
		project.ID, testWorkspaceID, execRef, testUserID); err != nil {
		t.Fatalf("seed project_resource: %v", err)
	}

	// Sanity: there is genuinely no binding before reconciliation.
	var pre int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM integration_project_binding WHERE project_id = $1 AND connection_id = $2`,
		project.ID, connID).Scan(&pre); err != nil {
		t.Fatalf("count bindings pre-reconcile: %v", err)
	}
	if pre != 0 {
		t.Fatalf("expected no binding before reconciliation, found %d", pre)
	}

	// Satisfy the rest of the ListInboundSyncTargets join (per-user inbound
	// setting + stored credential) so a reconciled binding becomes a target.
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_issue_sync_setting (workspace_id, user_id, provider, inbound_enabled_at)
		 VALUES ($1, $2, 'zentao', now())
		 ON CONFLICT (workspace_id, user_id, provider) DO UPDATE SET inbound_enabled_at = now()`,
		testWorkspaceID, testUserID); err != nil {
		t.Fatalf("seed issue sync setting: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_issue_sync_setting WHERE workspace_id = $1 AND user_id = $2 AND provider = 'zentao'`, testWorkspaceID, testUserID)
	})
	if _, err := testPool.Exec(ctx,
		`INSERT INTO integration_user_account (workspace_id, connection_id, user_id, account_key, account_name, credential_encrypted, status)
		 VALUES ($1, $2, $3, 'zt-exec-reconcile-acct', 'zt reconcile acct', $4, 'active')`,
		testWorkspaceID, connID, testUserID, []byte("sealed-credential")); err != nil {
		t.Fatalf("seed user account: %v", err)
	}

	// ListInboundSyncTargets must reconcile the resource into an inbound binding
	// and return it — without any CreateProjectResource/UpdateProjectResource call.
	targets, err := testHandler.ListInboundSyncTargets(ctx)
	if err != nil {
		t.Fatalf("ListInboundSyncTargets: %v", err)
	}
	found := false
	for _, tgt := range targets {
		if uuidToString(tgt.ProjectID) == project.ID && uuidToString(tgt.ConnectionID) == connID {
			found = true
			if id := zentaoProjectIDFromRef(tgt.ExternalRef); id != "447" {
				t.Fatalf("reconciled target external_ref resolves to %q, want 447", id)
			}
		}
	}
	if !found {
		t.Fatalf("ListInboundSyncTargets did not reconcile the existing execution resource into a target")
	}

	// The reconciled binding must actually be inbound-enabled in the DB.
	if _, inbound := loadZentaoBinding(t, ctx, project.ID, connID); !inbound {
		t.Fatalf("reconciled binding is not inbound-enabled")
	}
}

// TestNonZentaoResourceDoesNotCreateZentaoBinding guards the residual bug: the
// create/update auto-binding must fire only for zentao_project resources. A
// non-ZenTao resource (here a github_repo) whose URL merely contains an
// "execution-kanban-447" substring would otherwise parse to a ZenTao execution
// and create a binding — even in a workspace with exactly one active ZenTao
// connection. The resource_type guard must prevent any integration_project_binding.
func TestNonZentaoResourceDoesNotCreateZentaoBinding(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	// Exactly one active ZenTao connection — the condition under which
	// syncZentaoExecutionBinding would single it out and bind to it.
	var connID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO integration_connection (workspace_id, provider, name, base_url, status, sync_enabled_at)
		 VALUES ($1, 'zentao', 'zt-exec-guard-test', 'http://127.0.0.1:1', 'active', now()) RETURNING id`,
		testWorkspaceID).Scan(&connID); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM integration_connection WHERE id = $1`, connID)
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "non-zentao execution-substring project",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		req := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		req = withURLParam(req, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), req)
	})

	// A github_repo whose URL contains "execution-kanban-447": validated by
	// validateAndNormalizeResourceRef, and zentaoTargetFromRef would resolve it to
	// execution 447 if the guard were missing.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/resources", map[string]any{
		"resource_type": "github_repo",
		"resource_ref": map[string]any{
			"url": "https://github.com/acme/execution-kanban-447",
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

	// No binding may exist after create.
	var afterCreate int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM integration_project_binding WHERE project_id = $1 AND connection_id = $2`,
		project.ID, connID).Scan(&afterCreate); err != nil {
		t.Fatalf("count bindings after create: %v", err)
	}
	if afterCreate != 0 {
		t.Fatalf("non-zentao resource create produced %d ZenTao binding(s), want 0", afterCreate)
	}

	// Updating the same resource (still github_repo) must likewise not bind.
	w = httptest.NewRecorder()
	req = newRequest("PUT", "/api/projects/"+project.ID+"/resources/"+resource.ID, map[string]any{
		"resource_ref": map[string]any{
			"url": "https://github.com/acme/execution-kanban-512",
		},
	})
	req = withURLParams(req, "id", project.ID, "resourceId", resource.ID)
	testHandler.UpdateProjectResource(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateProjectResource: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var afterUpdate int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM integration_project_binding WHERE project_id = $1 AND connection_id = $2`,
		project.ID, connID).Scan(&afterUpdate); err != nil {
		t.Fatalf("count bindings after update: %v", err)
	}
	if afterUpdate != 0 {
		t.Fatalf("non-zentao resource update produced %d ZenTao binding(s), want 0", afterUpdate)
	}
}

func loadZentaoBinding(t *testing.T, ctx context.Context, projectID, connID string) ([]byte, bool) {
	t.Helper()
	var ref []byte
	var inboundAt *time.Time
	if err := testPool.QueryRow(ctx,
		`SELECT external_ref, inbound_enabled_at FROM integration_project_binding WHERE project_id = $1 AND connection_id = $2`,
		projectID, connID,
	).Scan(&ref, &inboundAt); err != nil {
		t.Fatalf("load binding: %v", err)
	}
	return ref, inboundAt != nil
}
