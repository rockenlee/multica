package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type WorkspaceResourceResponse struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	ResourceType string          `json:"resource_type"`
	ResourceRef  json.RawMessage `json:"resource_ref"`
	Label        *string         `json:"label"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	CreatedBy    *string         `json:"created_by"`
	CanManage    bool            `json:"can_manage"`
}

type CreateWorkspaceResourceRequest struct {
	ResourceType string          `json:"resource_type"`
	ResourceRef  json.RawMessage `json:"resource_ref"`
	Label        *string         `json:"label"`
}

func workspaceResourceToResponse(r db.WorkspaceResource, canManage bool) WorkspaceResourceResponse {
	ref := json.RawMessage(r.ResourceRef)
	if len(ref) == 0 {
		ref = json.RawMessage("{}")
	}
	return WorkspaceResourceResponse{
		ID:           uuidToString(r.ID),
		WorkspaceID:  uuidToString(r.WorkspaceID),
		ResourceType: r.ResourceType,
		ResourceRef:  ref,
		Label:        textToPtr(r.Label),
		CreatedAt:    timestampToString(r.CreatedAt),
		UpdatedAt:    timestampToString(r.UpdatedAt),
		CreatedBy:    uuidToPtr(r.CreatedBy),
		CanManage:    canManage,
	}
}

func validateWorkspaceResourceType(resourceType string) bool {
	switch resourceType {
	case "github_repo", "gitlab_repo", "feishu_drive", "feishu_wiki", "zentao_project", "zentao_product":
		return true
	default:
		return false
	}
}

func (h *Handler) loadWorkspaceResourceContext(w http.ResponseWriter, r *http.Request) (pgtype.UUID, db.Member, string, bool) {
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return pgtype.UUID{}, db.Member{}, "", false
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return pgtype.UUID{}, db.Member{}, "", false
	}
	userUUID, ok := h.parseUserUUIDOrZero(userID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return pgtype.UUID{}, db.Member{}, "", false
	}
	member, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      userUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusForbidden, "workspace member required")
		return pgtype.UUID{}, db.Member{}, "", false
	}
	return wsUUID, member, userID, true
}

func canManageWorkspaceResource(member db.Member, userID string, resource db.WorkspaceResource) bool {
	if roleAllowed(member.Role, "owner", "admin") {
		return true
	}
	return resource.CreatedBy.Valid && uuidToString(resource.CreatedBy) == userID
}

func canCreateWorkspaceResource(member db.Member) bool {
	return roleAllowed(member.Role, "owner", "admin")
}

func (h *Handler) ListWorkspaceResources(w http.ResponseWriter, r *http.Request) {
	wsUUID, member, userID, ok := h.loadWorkspaceResourceContext(w, r)
	if !ok {
		return
	}
	resources, err := h.Queries.ListWorkspaceResources(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspace resources")
		return
	}
	resp := make([]WorkspaceResourceResponse, len(resources))
	for i, resource := range resources {
		resp[i] = workspaceResourceToResponse(
			resource,
			canManageWorkspaceResource(member, userID, resource),
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": resp, "total": len(resp)})
}

func (h *Handler) CreateWorkspaceResource(w http.ResponseWriter, r *http.Request) {
	wsUUID, member, userID, ok := h.loadWorkspaceResourceContext(w, r)
	if !ok {
		return
	}
	if !canCreateWorkspaceResource(member) {
		writeError(w, http.StatusForbidden, "only workspace owners and admins can add resources")
		return
	}

	var req CreateWorkspaceResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ResourceType = strings.TrimSpace(req.ResourceType)
	if req.ResourceType == "" {
		writeError(w, http.StatusBadRequest, "resource_type is required")
		return
	}
	if !validateWorkspaceResourceType(req.ResourceType) {
		writeError(w, http.StatusBadRequest, "workspace resources support Git, Feishu, and ZenTao resource types only")
		return
	}
	normalizedRef, err := validateAndNormalizeResourceRef(req.ResourceType, req.ResourceRef)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var label pgtype.Text
	if req.Label != nil && strings.TrimSpace(*req.Label) != "" {
		label = pgtype.Text{String: strings.TrimSpace(*req.Label), Valid: true}
	}
	creator, _ := h.parseUserUUIDOrZero(userID)
	resource, err := h.Queries.CreateWorkspaceResource(r.Context(), db.CreateWorkspaceResourceParams{
		WorkspaceID:  wsUUID,
		ResourceType: req.ResourceType,
		ResourceRef:  normalizedRef,
		Label:        label,
		CreatedBy:    creator,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "this resource is already added to the workspace")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create workspace resource")
		return
	}
	writeJSON(w, http.StatusCreated, workspaceResourceToResponse(resource, true))
}

func (h *Handler) DeleteWorkspaceResource(w http.ResponseWriter, r *http.Request) {
	wsUUID, member, userID, ok := h.loadWorkspaceResourceContext(w, r)
	if !ok {
		return
	}
	resourceUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "resourceId"), "resource id")
	if !ok {
		return
	}
	resource, err := h.Queries.GetWorkspaceResourceInWorkspace(r.Context(), db.GetWorkspaceResourceInWorkspaceParams{
		ID:          resourceUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace resource not found")
		return
	}
	if !canManageWorkspaceResource(member, userID, resource) {
		writeError(w, http.StatusForbidden, "only workspace owners, admins, or the resource owner can delete this resource")
		return
	}
	if err := h.Queries.DeleteWorkspaceResource(r.Context(), resource.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace resource")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
