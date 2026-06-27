package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
