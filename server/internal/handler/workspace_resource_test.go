package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func createWorkspaceResourceTestMember(t *testing.T, role string) string {
	t.Helper()
	var userID string
	if err := testPool.QueryRow(t.Context(), `
		INSERT INTO "user" (name, email)
		VALUES ($1, $2)
		RETURNING id
	`, "Workspace Resource Test "+role, "workspace-resource-"+role+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := testPool.Exec(t.Context(), `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
	`, testWorkspaceID, userID, role); err != nil {
		t.Fatalf("create member: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(t.Context(), `DELETE FROM "user" WHERE id = $1`, userID)
	})
	return userID
}

func TestWorkspaceResourcesPermissionModel(t *testing.T) {
	if testHandler == nil {
		t.Skip("test handler not initialized")
	}
	memberID := createWorkspaceResourceTestMember(t, "member")
	adminID := createWorkspaceResourceTestMember(t, "admin")

	createBody := map[string]any{
		"resource_type": "feishu_drive",
		"resource_ref": map[string]any{
			"folder_token": "folder-permission-test",
		},
		"label": "Permission test drive",
	}

	w := httptest.NewRecorder()
	req := newRequestAs(memberID, "POST", "/api/workspaces/"+testWorkspaceID+"/resources", createBody)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.CreateWorkspaceResource(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member create: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequestAs(adminID, "POST", "/api/workspaces/"+testWorkspaceID+"/resources", createBody)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.CreateWorkspaceResource(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("admin create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created WorkspaceResourceResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode created resource: %v", err)
	}
	defer func() {
		req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/resources/"+created.ID, nil)
		req = withURLParams(req, "id", testWorkspaceID, "resourceId", created.ID)
		testHandler.DeleteWorkspaceResource(httptest.NewRecorder(), req)
	}()
	if !created.CanManage {
		t.Fatalf("creator response should be manageable")
	}

	w = httptest.NewRecorder()
	req = newRequestAs(memberID, "GET", "/api/workspaces/"+testWorkspaceID+"/resources", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListWorkspaceResources(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("member list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Resources []WorkspaceResourceResponse `json:"resources"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var memberView *WorkspaceResourceResponse
	for i := range listResp.Resources {
		if listResp.Resources[i].ID == created.ID {
			memberView = &listResp.Resources[i]
			break
		}
	}
	if memberView == nil {
		t.Fatalf("created resource missing from member-visible list")
	}
	if memberView.CanManage {
		t.Fatalf("plain member should see can_manage=false for another owner's resource")
	}

	w = httptest.NewRecorder()
	req = newRequestAs(memberID, "DELETE", "/api/workspaces/"+testWorkspaceID+"/resources/"+created.ID, nil)
	req = withURLParams(req, "id", testWorkspaceID, "resourceId", created.ID)
	testHandler.DeleteWorkspaceResource(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member delete other's resource: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequestAs(adminID, "DELETE", "/api/workspaces/"+testWorkspaceID+"/resources/"+created.ID, nil)
	req = withURLParams(req, "id", testWorkspaceID, "resourceId", created.ID)
	testHandler.DeleteWorkspaceResource(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("creator delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}
