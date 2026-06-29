package handler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/integrations/feishu"
	"github.com/multica-ai/multica/server/internal/integrations/gitlab"
	"github.com/multica-ai/multica/server/internal/integrations/zentao"
	"github.com/multica-ai/multica/server/internal/issueposition"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var integrationProviders = map[string]struct{}{
	"gitlab": {},
	"zentao": {},
	"feishu": {},
}

type IntegrationConnectionResponse struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	Provider        string          `json:"provider"`
	Name            string          `json:"name"`
	BaseURL         *string         `json:"base_url"`
	Config          json.RawMessage `json:"config"`
	Status          string          `json:"status"`
	SyncEnabled     bool            `json:"sync_enabled"`
	CreatedByUserID *string         `json:"created_by_user_id"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type IntegrationUserAccountResponse struct {
	ID                   string          `json:"id"`
	WorkspaceID          string          `json:"workspace_id"`
	ConnectionID         string          `json:"connection_id"`
	UserID               string          `json:"user_id"`
	AccountKey           string          `json:"account_key"`
	AccountName          string          `json:"account_name"`
	ExternalUserID       *string         `json:"external_user_id"`
	ExternalUsername     *string         `json:"external_username"`
	CredentialConfigured bool            `json:"credential_configured"`
	Scopes               []string        `json:"scopes"`
	Config               json.RawMessage `json:"config"`
	Status               string          `json:"status"`
	SyncEnabled          bool            `json:"sync_enabled"`
	ExpiresAt            *string         `json:"expires_at"`
	LastUsedAt           *string         `json:"last_used_at"`
	LastError            *string         `json:"last_error"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

type IntegrationProjectBindingResponse struct {
	ID                   string          `json:"id"`
	WorkspaceID          string          `json:"workspace_id"`
	ProjectID            string          `json:"project_id"`
	ConnectionID         string          `json:"connection_id"`
	ExternalRef          json.RawMessage `json:"external_ref"`
	InboundEnabled       bool            `json:"inbound_enabled"`
	OutboundEnabled      bool            `json:"outbound_enabled"`
	IssueSyncEnabled     bool            `json:"issue_sync_enabled"`
	KnowledgeSyncEnabled bool            `json:"knowledge_sync_enabled"`
	CreatedByUserID      *string         `json:"created_by_user_id"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

type IntegrationSyncEventResponse struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	ConnectionID *string         `json:"connection_id"`
	Provider     string          `json:"provider"`
	Direction    string          `json:"direction"`
	ObjectType   string          `json:"object_type"`
	ObjectID     *string         `json:"object_id"`
	ExternalID   *string         `json:"external_id"`
	ExternalURL  *string         `json:"external_url"`
	ProjectID    *string         `json:"project_id"`
	Status       string          `json:"status"`
	Message      *string         `json:"message"`
	Error        *string         `json:"error"`
	Metadata     json.RawMessage `json:"metadata"`
	OccurredAt   string          `json:"occurred_at"`
	CreatedAt    string          `json:"created_at"`
}

type IntegrationIssueSyncSettingResponse struct {
	WorkspaceID     string `json:"workspace_id"`
	UserID          string `json:"user_id"`
	Provider        string `json:"provider"`
	InboundEnabled  bool   `json:"inbound_enabled"`
	OutboundEnabled bool   `json:"outbound_enabled"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type ListIntegrationsResponse struct {
	Connections              []IntegrationConnectionResponse       `json:"connections"`
	Accounts                 []IntegrationUserAccountResponse      `json:"accounts"`
	ProjectBindings          []IntegrationProjectBindingResponse   `json:"project_bindings"`
	IssueSyncSettings        []IntegrationIssueSyncSettingResponse `json:"issue_sync_settings"`
	SyncEvents               []IntegrationSyncEventResponse        `json:"sync_events"`
	CredentialStorageEnabled bool                                  `json:"credential_storage_enabled"`
}

type UpsertIntegrationConnectionRequest struct {
	Provider    string          `json:"provider"`
	Name        string          `json:"name"`
	BaseURL     *string         `json:"base_url"`
	Config      json.RawMessage `json:"config"`
	Status      *string         `json:"status"`
	SyncEnabled *bool           `json:"sync_enabled"`
}

type UpsertIntegrationUserAccountRequest struct {
	AccountKey       *string         `json:"account_key"`
	AccountName      *string         `json:"account_name"`
	ExternalUserID   *string         `json:"external_user_id"`
	ExternalUsername *string         `json:"external_username"`
	Credential       *string         `json:"credential"`
	Scopes           []string        `json:"scopes"`
	Config           json.RawMessage `json:"config"`
	Status           *string         `json:"status"`
	SyncEnabled      bool            `json:"sync_enabled"`
	ExpiresAt        *string         `json:"expires_at"`
}

type UpsertIntegrationProjectBindingRequest struct {
	ExternalRef          json.RawMessage `json:"external_ref"`
	InboundEnabled       bool            `json:"inbound_enabled"`
	OutboundEnabled      bool            `json:"outbound_enabled"`
	IssueSyncEnabled     bool            `json:"issue_sync_enabled"`
	KnowledgeSyncEnabled bool            `json:"knowledge_sync_enabled"`
}

type UpsertIntegrationIssueSyncSettingRequest struct {
	InboundEnabled  bool `json:"inbound_enabled"`
	OutboundEnabled bool `json:"outbound_enabled"`
}

type SyncInboundIntegrationIssueRequest struct {
	Title          string  `json:"title"`
	Description    *string `json:"description"`
	SourceType     *string `json:"source_type"`
	SourceID       string  `json:"source_id"`
	SourceURL      *string `json:"source_url"`
	ExternalStatus *string `json:"external_status"`
	ProjectID      *string `json:"project_id"`
	ResourceID     *string `json:"resource_id"`
	AssigneeUserID *string `json:"assignee_user_id"`
}

type SyncInboundIntegrationIssueResponse struct {
	Status   string                       `json:"status"`
	Issue    IssueResponse                `json:"issue"`
	Metadata map[string]any               `json:"metadata"`
	Event    IntegrationSyncEventResponse `json:"event"`
}

type RequestIssueOutboundSyncRequest struct {
	Provider   *string `json:"provider"`
	SourceType *string `json:"source_type"`
}

type IssueOutboundSyncResponse struct {
	IssueID      string                        `json:"issue_id"`
	ProjectID    *string                       `json:"project_id"`
	Status       string                        `json:"status"`
	Provider     *string                       `json:"provider"`
	ConnectionID *string                       `json:"connection_id"`
	Message      string                        `json:"message"`
	Metadata     map[string]any                `json:"metadata"`
	Event        *IntegrationSyncEventResponse `json:"event,omitempty"`
}

func (h *Handler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}

	connections, err := h.Queries.ListIntegrationConnections(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integration connections")
		return
	}
	accounts, err := h.Queries.ListIntegrationUserAccountsForUser(r.Context(), db.ListIntegrationUserAccountsForUserParams{
		WorkspaceID: wsUUID,
		UserID:      userUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integration accounts")
		return
	}
	bindings, err := h.Queries.ListIntegrationProjectBindings(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integration project bindings")
		return
	}
	syncSettings, err := h.Queries.ListIntegrationIssueSyncSettingsForUser(r.Context(), db.ListIntegrationIssueSyncSettingsForUserParams{
		WorkspaceID: wsUUID,
		UserID:      userUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integration issue sync settings")
		return
	}
	events, err := h.Queries.ListIntegrationSyncEvents(r.Context(), db.ListIntegrationSyncEventsParams{
		WorkspaceID: wsUUID,
		Limit:       20,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integration sync events")
		return
	}

	// The recent window (LIMIT 20) can be filled entirely by high-frequency
	// providers, hiding a lower-frequency provider's latest sync from its card.
	// Append each provider's latest event (if not already in the window) so every
	// provider card reflects its own most recent sync. These are older than the
	// window, so appending keeps the list time-descending for Recent sync events.
	latestPerProvider, err := h.latestSyncEventPerProvider(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list latest sync events")
		return
	}
	seenEventIDs := make(map[string]struct{}, len(events))
	for _, e := range events {
		seenEventIDs[uuidToString(e.ID)] = struct{}{}
	}
	for _, e := range latestPerProvider {
		if _, ok := seenEventIDs[uuidToString(e.ID)]; !ok {
			events = append(events, e)
		}
	}

	resp := ListIntegrationsResponse{
		Connections:              make([]IntegrationConnectionResponse, 0, len(connections)),
		Accounts:                 make([]IntegrationUserAccountResponse, 0, len(accounts)),
		ProjectBindings:          make([]IntegrationProjectBindingResponse, 0, len(bindings)),
		IssueSyncSettings:        make([]IntegrationIssueSyncSettingResponse, 0, len(syncSettings)),
		SyncEvents:               make([]IntegrationSyncEventResponse, 0, len(events)),
		CredentialStorageEnabled: h.IntegrationSecrets != nil,
	}
	for _, row := range connections {
		resp.Connections = append(resp.Connections, integrationConnectionToResponse(row))
	}
	for _, row := range accounts {
		resp.Accounts = append(resp.Accounts, integrationUserAccountToResponse(row))
	}
	for _, row := range bindings {
		resp.ProjectBindings = append(resp.ProjectBindings, integrationProjectBindingToResponse(row))
	}
	for _, row := range syncSettings {
		resp.IssueSyncSettings = append(resp.IssueSyncSettings, integrationIssueSyncSettingToResponse(row))
	}
	for _, row := range events {
		resp.SyncEvents = append(resp.SyncEvents, integrationSyncEventToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	var req UpsertIntegrationConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	provider, name, baseURL, config, status, syncEnabled, ok := normalizeIntegrationConnectionRequest(w, req, true)
	if !ok {
		return
	}
	// One connection per provider per workspace — dedup so the same app/App ID
	// can't be added twice (edit the existing connection instead).
	existingConns, _ := h.Queries.ListIntegrationConnections(r.Context(), wsUUID)
	for _, c := range existingConns {
		if c.Provider == provider {
			writeError(w, http.StatusConflict, "a "+provider+" connection already exists in this workspace; edit it instead of adding another")
			return
		}
	}
	config, err := h.sealConnectionConfig(config, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.Queries.CreateIntegrationConnection(r.Context(), db.CreateIntegrationConnectionParams{
		WorkspaceID:     wsUUID,
		Provider:        provider,
		Name:            name,
		BaseUrl:         ptrToText(baseURL),
		Config:          config,
		Status:          status,
		SyncEnabled:     syncEnabled,
		CreatedByUserID: userUUID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "integration connection already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create integration connection")
		return
	}
	h.recordIntegrationInternalEvent(r, wsUUID, row.ID, row.Provider, "connection", uuidToString(row.ID), "Connection saved", pgtype.UUID{})
	writeJSON(w, http.StatusCreated, integrationConnectionToResponse(row))
}

func (h *Handler) UpdateIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	var req UpsertIntegrationConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, name, baseURL, config, status, _, ok := normalizeIntegrationConnectionRequest(w, req, false)
	if !ok {
		return
	}
	existing, _ := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	config, err := h.sealConnectionConfig(config, existing.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	syncEnabled := pgtype.Bool{}
	if req.SyncEnabled != nil {
		syncEnabled = pgtype.Bool{Bool: *req.SyncEnabled, Valid: true}
	}
	row, err := h.Queries.UpdateIntegrationConnection(r.Context(), db.UpdateIntegrationConnectionParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
		Name:        ptrToText(&name),
		BaseUrl:     ptrToText(baseURL),
		Config:      config,
		Status:      ptrToText(&status),
		SyncEnabled: syncEnabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "integration connection not found")
			return
		}
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "integration connection already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update integration connection")
		return
	}
	h.recordIntegrationInternalEvent(r, wsUUID, row.ID, row.Provider, "connection", uuidToString(row.ID), "Connection saved", pgtype.UUID{})
	writeJSON(w, http.StatusOK, integrationConnectionToResponse(row))
}

func (h *Handler) DeleteIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	if err := h.Queries.DeleteIntegrationConnection(r.Context(), db.DeleteIntegrationConnectionParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete integration connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpsertIntegrationUserAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	connection, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "integration connection not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load integration connection")
		return
	}
	var req UpsertIntegrationUserAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	config, ok := normalizeJSONObject(w, req.Config, "config")
	if !ok {
		return
	}
	accountKey, accountName, scopes, status, expiresAt, ok := normalizeIntegrationAccountRequest(w, req)
	if !ok {
		return
	}
	var sealed []byte
	if req.Credential != nil && strings.TrimSpace(*req.Credential) != "" {
		if h.IntegrationSecrets == nil {
			writeError(w, http.StatusServiceUnavailable, "integration credential storage is not configured")
			return
		}
		ciphertext, err := h.IntegrationSecrets.Seal([]byte(strings.TrimSpace(*req.Credential)))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt integration credential")
			return
		}
		sealed = ciphertext
	}
	row, err := h.Queries.UpsertIntegrationUserAccount(r.Context(), db.UpsertIntegrationUserAccountParams{
		WorkspaceID:         wsUUID,
		ConnectionID:        connUUID,
		UserID:              userUUID,
		AccountKey:          accountKey,
		AccountName:         accountName,
		ExternalUserID:      ptrToText(trimStringPtr(req.ExternalUserID)),
		ExternalUsername:    ptrToText(trimStringPtr(req.ExternalUsername)),
		CredentialEncrypted: sealed,
		Scopes:              scopes,
		Config:              config,
		Status:              status,
		ExpiresAt:           expiresAt,
		SyncEnabled:         req.SyncEnabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save integration account")
		return
	}
	h.recordIntegrationInternalEvent(r, wsUUID, connUUID, connection.Provider, "account", uuidToString(row.ID), "Account saved", pgtype.UUID{})
	writeJSON(w, http.StatusOK, integrationUserAccountToResponse(row))
}

func (h *Handler) DeleteIntegrationUserAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	if err := h.Queries.DeleteIntegrationUserAccount(r.Context(), db.DeleteIntegrationUserAccountParams{
		WorkspaceID:  wsUUID,
		ConnectionID: connUUID,
		UserID:       userUUID,
		AccountKey:   normalizeAccountKey(r.URL.Query().Get("account_key"), "default"),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete integration account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteIntegrationUserAccountByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	accountUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "accountId"), "integration account id")
	if !ok {
		return
	}
	if err := h.Queries.DeleteIntegrationUserAccountByID(r.Context(), db.DeleteIntegrationUserAccountByIDParams{
		WorkspaceID: wsUUID,
		UserID:      userUUID,
		ID:          accountUUID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete integration account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpsertIntegrationProjectBinding(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	projectUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "projectId"), "project id")
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	project, err := h.Queries.GetProject(r.Context(), projectUUID)
	if err != nil || uuidToString(project.WorkspaceID) != uuidToString(wsUUID) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	connection, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "integration connection not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load integration connection")
		return
	}
	var req UpsertIntegrationProjectBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	externalRef, ok := normalizeJSONObject(w, req.ExternalRef, "external_ref")
	if !ok {
		return
	}
	row, err := h.Queries.UpsertIntegrationProjectBinding(r.Context(), db.UpsertIntegrationProjectBindingParams{
		WorkspaceID:          wsUUID,
		ProjectID:            projectUUID,
		ConnectionID:         connUUID,
		ExternalRef:          externalRef,
		CreatedByUserID:      userUUID,
		InboundEnabled:       req.InboundEnabled,
		OutboundEnabled:      req.OutboundEnabled,
		IssueSyncEnabled:     req.IssueSyncEnabled,
		KnowledgeSyncEnabled: req.KnowledgeSyncEnabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save integration project binding")
		return
	}
	h.recordIntegrationInternalEvent(r, wsUUID, connUUID, connection.Provider, "project_binding", uuidToString(row.ID), "Project binding saved", projectUUID)
	writeJSON(w, http.StatusOK, integrationProjectBindingToResponse(row))
}

func (h *Handler) UpsertIntegrationIssueSyncSetting(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if _, ok := integrationProviders[provider]; !ok {
		writeError(w, http.StatusBadRequest, "provider must be gitlab, zentao, or feishu")
		return
	}
	var req UpsertIntegrationIssueSyncSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	row, err := h.Queries.UpsertIntegrationIssueSyncSetting(r.Context(), db.UpsertIntegrationIssueSyncSettingParams{
		WorkspaceID:     wsUUID,
		UserID:          userUUID,
		Provider:        provider,
		InboundEnabled:  req.InboundEnabled,
		OutboundEnabled: req.OutboundEnabled,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save integration issue sync setting")
		return
	}
	writeJSON(w, http.StatusOK, integrationIssueSyncSettingToResponse(row))
}

func (h *Handler) SyncInboundIntegrationIssue(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, connUUID, ok := integrationConnectionPath(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	connection, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "integration connection not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load integration connection")
		return
	}
	if connection.Status != "active" || !connection.SyncEnabledAt.Valid {
		writeError(w, http.StatusConflict, "integration connection sync is disabled")
		return
	}
	setting, err := h.Queries.GetIntegrationIssueSyncSettingForUser(r.Context(), db.GetIntegrationIssueSyncSettingForUserParams{
		WorkspaceID: wsUUID,
		UserID:      userUUID,
		Provider:    connection.Provider,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "inbound issue sync is disabled for "+connection.Provider)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load integration issue sync setting")
		return
	}
	if !setting.InboundEnabledAt.Valid {
		writeError(w, http.StatusConflict, "inbound issue sync is disabled for "+connection.Provider)
		return
	}

	var req SyncInboundIntegrationIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	sourceID := strings.TrimSpace(req.SourceID)
	if sourceID == "" {
		writeError(w, http.StatusBadRequest, "source_id is required")
		return
	}
	sourceType := strings.ToLower(strings.TrimSpace(firstNonEmptyStringPtr(req.SourceType, "issue")))
	sourceURL := strings.TrimSpace(firstNonEmptyStringPtr(req.SourceURL))
	if sourceURL != "" && !isValidHTTPURL(sourceURL) {
		writeError(w, http.StatusBadRequest, "source_url must be a valid http(s) URL")
		return
	}
	externalStatus := strings.TrimSpace(firstNonEmptyStringPtr(req.ExternalStatus))

	projectID, resourceID, ok := h.resolveInboundIssueProjectResource(w, r, wsUUID, connection.Provider, req.ProjectID, req.ResourceID)
	if !ok {
		return
	}
	assigneeID := userUUID
	if req.AssigneeUserID != nil && strings.TrimSpace(*req.AssigneeUserID) != "" {
		parsed, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(*req.AssigneeUserID), "assignee_user_id")
		if !ok {
			return
		}
		assigneeID = parsed
	}

	// A user-initiated inbound (re-)sync is an explicit "I want this item" action
	// — clear any prior deletion tombstone so the mirror can be recreated.
	_, _ = h.DB.Exec(r.Context(), `DELETE FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider=$2 AND source_id=$3`, wsUUID, connection.Provider, sourceID)

	issue, status, event, err := h.upsertInboundIntegrationIssue(r.Context(), connection, userUUID, assigneeID, projectID, resourceID, title, req.Description, sourceType, sourceID, sourceURL, externalStatus)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "external issue is already being synced")
			return
		}
		slog.Error("failed to sync inbound integration issue", "workspace_id", uuidToString(wsUUID), "connection_id", uuidToString(connUUID), "provider", connection.Provider, "source_id", sourceID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to sync inbound issue")
		return
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	issueID := uuidToString(issue.ID)
	resp := issueToResponse(issue, h.getIssuePrefix(r.Context(), issue.WorkspaceID))
	h.publish(protocol.EventIssueUpdated, workspaceID, "member", userID, map[string]any{"issue": resp})
	h.publish(protocol.EventIssueMetadataChanged, workspaceID, "member", userID, map[string]any{
		"issue_id": issueID,
		"metadata": parseIssueMetadata(issue.Metadata),
	})
	writeJSON(w, http.StatusOK, SyncInboundIntegrationIssueResponse{
		Status:   status,
		Issue:    resp,
		Metadata: parseIssueMetadata(issue.Metadata),
		Event:    integrationSyncEventToResponse(event),
	})
}

func (h *Handler) resolveInboundIssueProjectResource(
	w http.ResponseWriter,
	r *http.Request,
	workspaceID pgtype.UUID,
	provider string,
	projectIDInput *string,
	resourceIDInput *string,
) (pgtype.UUID, pgtype.UUID, bool) {
	var projectID pgtype.UUID
	var resourceID pgtype.UUID

	if projectIDInput != nil && strings.TrimSpace(*projectIDInput) != "" {
		parsed, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(*projectIDInput), "project_id")
		if !ok {
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
			ID:          parsed,
			WorkspaceID: workspaceID,
		}); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		projectID = parsed
	}

	if resourceIDInput != nil && strings.TrimSpace(*resourceIDInput) != "" {
		parsed, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(*resourceIDInput), "resource_id")
		if !ok {
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		resource, err := h.Queries.GetProjectResourceInWorkspace(r.Context(), db.GetProjectResourceInWorkspaceParams{
			ID:          parsed,
			WorkspaceID: workspaceID,
		})
		if err != nil {
			writeError(w, http.StatusNotFound, "project resource not found")
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		if resourceProvider(resource.ResourceType) != provider {
			writeError(w, http.StatusConflict, "project resource does not match integration provider")
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		if projectID.Valid && uuidToString(projectID) != uuidToString(resource.ProjectID) {
			writeError(w, http.StatusConflict, "project resource does not belong to project_id")
			return pgtype.UUID{}, pgtype.UUID{}, false
		}
		resourceID = parsed
		if !projectID.Valid {
			projectID = resource.ProjectID
		}
	}

	return projectID, resourceID, true
}

func (h *Handler) upsertInboundIntegrationIssue(
	ctx context.Context,
	connection db.IntegrationConnection,
	actorID pgtype.UUID,
	assigneeID pgtype.UUID,
	projectID pgtype.UUID,
	resourceID pgtype.UUID,
	title string,
	description *string,
	sourceType string,
	sourceID string,
	sourceURL string,
	externalStatus string,
) (db.Issue, string, db.IntegrationSyncEvent, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.Issue{}, "", db.IntegrationSyncEvent{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)

	existing, err := qtx.GetIssueByExternalSource(ctx, db.GetIssueByExternalSourceParams{
		WorkspaceID:  connection.WorkspaceID,
		SourceSystem: connection.Provider,
		SourceID:     sourceID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("get issue by external source: %w", err)
	}
	isNewIssue := errors.Is(err, pgx.ErrNoRows)

	status := "created"
	var issue db.Issue
	var eventMessage string
	metadata, err := buildInboundIssueMetadata(existing.Metadata, connection, resourceID, sourceType, sourceID, sourceURL, externalStatus, title, description)
	if err != nil {
		return db.Issue{}, "", db.IntegrationSyncEvent{}, err
	}
	descriptionText := ptrToText(trimStringPtr(description))

	if isNewIssue {
		// Suppression: if a user previously deleted this external mirror, a
		// tombstone exists — do NOT resurrect it on the next inbound poll. Record
		// a skipped inbound event via the pool (not qtx) so it survives this tx's
		// rollback. Manual re-sync clears the tombstone first, so this only blocks
		// automatic recreation.
		var tombstoned bool
		if e := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM integration_issue_tombstone WHERE workspace_id=$1 AND provider=$2 AND source_id=$3)`,
			connection.WorkspaceID, connection.Provider, sourceID).Scan(&tombstoned); e == nil && tombstoned {
			msg := "Inbound suppressed: mirror was deleted by a user (tombstoned)"
			_, _ = h.Queries.CreateIntegrationSyncEvent(ctx, db.CreateIntegrationSyncEventParams{
				WorkspaceID:  connection.WorkspaceID,
				ConnectionID: connection.ID,
				Provider:     connection.Provider,
				Direction:    "inbound",
				ObjectType:   sourceType,
				ObjectID:     ptrToText(&sourceID),
				ExternalID:   ptrToText(&sourceID),
				ProjectID:    projectID,
				Status:       "skipped",
				Message:      ptrToText(&msg),
				Metadata:     []byte("{}"),
			})
			return db.Issue{}, "skipped", db.IntegrationSyncEvent{}, nil
		}
		issueNumber, err := qtx.IncrementIssueCounter(ctx, connection.WorkspaceID)
		if err != nil {
			return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("increment issue counter: %w", err)
		}
		position, err := issueposition.NextTopPosition(ctx, tx, connection.WorkspaceID, "todo")
		if err != nil {
			return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("compute issue position: %w", err)
		}
		issue, err = qtx.CreateIssueExternalMirror(ctx, db.CreateIssueExternalMirrorParams{
			WorkspaceID:   connection.WorkspaceID,
			Title:         title,
			Description:   descriptionText,
			Status:        "todo",
			Priority:      "none",
			AssigneeType:  pgtype.Text{String: "member", Valid: true},
			AssigneeID:    assigneeID,
			CreatorType:   "member",
			CreatorID:     actorID,
			ParentIssueID: pgtype.UUID{},
			Position:      position,
			Number:        issueNumber,
			ProjectID:     projectID,
			Metadata:      metadata,
		})
		if err != nil {
			return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("create issue external mirror: %w", err)
		}
		eventMessage = "Inbound issue created"
	} else {
		status = "updated"
		issue, err = qtx.UpdateIssueExternalMirror(ctx, db.UpdateIssueExternalMirrorParams{
			ID:          existing.ID,
			WorkspaceID: existing.WorkspaceID,
			Title:       title,
			Description: descriptionText,
			Metadata:    metadata,
			ProjectID:   projectID,
		})
		if err != nil {
			return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("update issue external mirror: %w", err)
		}
		eventMessage = "Inbound issue updated"
	}

	// Backfill the assignee on an unassigned mirror (e.g. personal tasks imported
	// before they were assignable) without clobbering a manual reassignment.
	if assigneeID.Valid && !issue.AssigneeID.Valid {
		if _, e := tx.Exec(ctx, `UPDATE issue SET assignee_id=$1, assignee_type='member', updated_at=now() WHERE id=$2 AND assignee_id IS NULL`, assigneeID, issue.ID); e == nil {
			issue.AssigneeID = assigneeID
			issue.AssigneeType = pgtype.Text{String: "member", Valid: true}
		}
	}

	issueID := uuidToString(issue.ID)
	eventMetadata, _ := json.Marshal(map[string]any{
		"source_type": sourceType,
		"sync_hash":   metadataString(parseIssueMetadata(metadata), "sync_hash"),
	})
	event, err := qtx.CreateIntegrationSyncEvent(ctx, db.CreateIntegrationSyncEventParams{
		WorkspaceID:  connection.WorkspaceID,
		ConnectionID: connection.ID,
		Provider:     connection.Provider,
		Direction:    "inbound",
		ObjectType:   "issue",
		ObjectID:     ptrToText(&issueID),
		ExternalID:   ptrToText(&sourceID),
		ExternalUrl:  ptrToText(trimStringPtr(&sourceURL)),
		ProjectID:    issue.ProjectID,
		Status:       "success",
		Message:      ptrToText(&eventMessage),
		Metadata:     eventMetadata,
	})
	if err != nil {
		return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("create integration sync event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Issue{}, "", db.IntegrationSyncEvent{}, fmt.Errorf("commit inbound issue sync: %w", err)
	}
	return issue, status, event, nil
}

func buildInboundIssueMetadata(
	existingRaw []byte,
	connection db.IntegrationConnection,
	resourceID pgtype.UUID,
	sourceType string,
	sourceID string,
	sourceURL string,
	externalStatus string,
	title string,
	description *string,
) ([]byte, error) {
	metadata := parseIssueMetadata(existingRaw)
	metadata["source_system"] = connection.Provider
	metadata["source_type"] = sourceType
	metadata["source_id"] = sourceID
	metadata["source_connection_id"] = uuidToString(connection.ID)
	if resourceID.Valid {
		metadata["source_resource_id"] = uuidToString(resourceID)
	}
	if sourceURL != "" {
		metadata["source_url"] = sourceURL
	}
	if externalStatus != "" {
		metadata["external_status"] = externalStatus
	}
	metadata["sync_hash"] = integrationIssueSyncHash(title, description, sourceType, sourceID, sourceURL, externalStatus)
	metadata["last_synced_at"] = time.Now().UTC().Format(time.RFC3339)
	if len(metadata) > maxIssueMetadataKeys {
		return nil, errors.New("metadata cannot exceed 50 keys")
	}
	return json.Marshal(metadata)
}

func integrationIssueSyncHash(title string, description *string, sourceType string, sourceID string, sourceURL string, externalStatus string) string {
	descriptionValue := ""
	if description != nil {
		descriptionValue = strings.TrimSpace(*description)
	}
	payload, _ := json.Marshal(map[string]string{
		"title":           title,
		"description":     descriptionValue,
		"source_type":     sourceType,
		"source_id":       sourceID,
		"source_url":      sourceURL,
		"external_status": externalStatus,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func resourceProvider(resourceType string) string {
	switch resourceType {
	case "gitlab_repo":
		return "gitlab"
	case "zentao_project", "zentao_product":
		return "zentao"
	case "feishu_drive", "feishu_wiki":
		return "feishu"
	default:
		return ""
	}
}

func (h *Handler) RequestIssueOutboundSync(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, issueID)
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req RequestIssueOutboundSyncRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	metadata := parseIssueMetadata(issue.Metadata)
	if _, hasSystem := metadata["source_system"]; hasSystem {
		writeJSON(w, http.StatusOK, IssueOutboundSyncResponse{
			IssueID:   uuidToString(issue.ID),
			ProjectID: uuidToPtr(issue.ProjectID),
			Status:    "skipped",
			Message:   "issue is already linked to an external source",
			Metadata:  metadata,
		})
		return
	}
	if _, hasSourceID := metadata["source_id"]; hasSourceID {
		writeJSON(w, http.StatusOK, IssueOutboundSyncResponse{
			IssueID:   uuidToString(issue.ID),
			ProjectID: uuidToPtr(issue.ProjectID),
			Status:    "skipped",
			Message:   "issue is already linked to an external source",
			Metadata:  metadata,
		})
		return
	}
	if syncOutStatus := outboundIssueSyncInFlightStatus(metadata); syncOutStatus != "" {
		writeJSON(w, http.StatusOK, IssueOutboundSyncResponse{
			IssueID:   uuidToString(issue.ID),
			ProjectID: uuidToPtr(issue.ProjectID),
			Status:    "skipped",
			Message:   "outbound issue sync is already " + syncOutStatus,
			Metadata:  metadata,
		})
		return
	}
	if !issue.ProjectID.Valid {
		writeError(w, http.StatusBadRequest, "issue must belong to a project before outbound sync")
		return
	}

	candidates, err := h.outboundIssueSyncCandidates(r, issue.WorkspaceID, issue.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load integration project bindings")
		return
	}
	providerHint := ""
	if req.Provider != nil {
		providerHint = strings.ToLower(strings.TrimSpace(*req.Provider))
		if _, ok := integrationProviders[providerHint]; !ok {
			writeError(w, http.StatusBadRequest, "provider must be gitlab, zentao, or feishu")
			return
		}
	}
	sourceType := strings.ToLower(strings.TrimSpace(firstNonEmptyStringPtr(req.SourceType, metadataString(metadata, "sync_out_type"), metadataString(metadata, "issue_type"))))
	candidate, routeMessage, ok := selectOutboundIssueSyncCandidate(candidates, providerHint, sourceType)
	if !ok {
		writeError(w, http.StatusConflict, routeMessage)
		return
	}

	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}

	issueIDString := uuidToString(issue.ID)

	// Create the external issue for real (network call, outside the DB tx) so
	// the recorded status reflects reality instead of a perpetual "queued".
	extSourceID, extSourceURL, extCreated, extErr := h.createExternalIssueForOutbound(r.Context(), candidate, issue, userUUID)
	notImplemented := !extCreated && strings.Contains(extErr, "not yet implemented")

	syncStatus := "failed"
	eventStatus := "error"
	eventMessage := "Outbound issue creation failed: " + extErr
	switch {
	case extCreated:
		syncStatus = "created"
		eventStatus = "success"
		eventMessage = "Outbound issue created in " + candidate.connection.Provider
	case notImplemented:
		syncStatus = "queued"
		eventStatus = "success"
		eventMessage = "Outbound issue sync queued"
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start outbound sync transaction")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)

	eventMetadata, _ := json.Marshal(map[string]any{
		"requested_by_user_id": userID,
		"source_type":          sourceType,
		"route_message":        routeMessage,
		"external_source_id":   extSourceID,
		"external_source_url":  extSourceURL,
		"error":                strings.TrimSpace(extErr),
	})
	event, err := qtx.CreateIntegrationSyncEvent(r.Context(), db.CreateIntegrationSyncEventParams{
		WorkspaceID:  issue.WorkspaceID,
		ConnectionID: candidate.connection.ID,
		Provider:     candidate.connection.Provider,
		Direction:    "outbound",
		ObjectType:   "issue",
		ObjectID:     ptrToText(&issueIDString),
		ProjectID:    issue.ProjectID,
		Status:       eventStatus,
		Message:      ptrToText(&eventMessage),
		Metadata:     eventMetadata,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record outbound sync event")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updates := map[string]json.RawMessage{
		"sync_out":               json.RawMessage(`true`),
		"sync_out_provider":      jsonString(candidate.connection.Provider),
		"sync_out_connection_id": jsonString(uuidToString(candidate.connection.ID)),
		"sync_out_status":        jsonString(syncStatus),
		"sync_out_requested_at":  jsonString(now),
		"sync_out_event_id":      jsonString(uuidToString(event.ID)),
	}
	if sourceType != "" {
		updates["sync_out_type"] = jsonString(sourceType)
	}
	if extCreated {
		updates["source_system"] = jsonString(candidate.connection.Provider)
		updates["source_id"] = jsonString(extSourceID)
		updates["source_url"] = jsonString(extSourceURL)
		updates["external_status"] = jsonString("opened")
		updates["last_synced_at"] = jsonString(now)
	} else if !notImplemented && strings.TrimSpace(extErr) != "" {
		errText := extErr
		if len(errText) > 300 {
			errText = errText[:300]
		}
		updates["sync_out_error"] = jsonString(errText)
	}
	newMetadataKeys := 0
	for key := range updates {
		if _, present := metadata[key]; !present {
			newMetadataKeys++
		}
	}
	if len(metadata)+newMetadataKeys > maxIssueMetadataKeys {
		writeError(w, http.StatusBadRequest, "metadata cannot exceed 50 keys")
		return
	}

	updated := issue
	for key, value := range updates {
		updated, err = qtx.SetIssueMetadataKey(r.Context(), db.SetIssueMetadataKeyParams{
			ID:          issue.ID,
			WorkspaceID: issue.WorkspaceID,
			Key:         key,
			Value:       []byte(value),
		})
		if err != nil {
			if isCheckViolation(err) {
				writeError(w, http.StatusBadRequest, "metadata exceeds the 8KB size limit")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to mark issue for outbound sync")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit outbound sync request")
		return
	}

	workspaceID := uuidToString(updated.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	updatedMetadata := parseIssueMetadata(updated.Metadata)
	h.publish(protocol.EventIssueMetadataChanged, workspaceID, actorType, actorID, map[string]any{
		"issue_id": issueIDString,
		"metadata": updatedMetadata,
	})

	provider := candidate.connection.Provider
	connectionID := uuidToString(candidate.connection.ID)
	respStatus := http.StatusAccepted
	if extCreated {
		respStatus = http.StatusOK
	}
	respMessage := routeMessage
	if !extCreated && !notImplemented && strings.TrimSpace(extErr) != "" {
		respMessage = extErr
	}
	writeJSON(w, respStatus, IssueOutboundSyncResponse{
		IssueID:      issueIDString,
		ProjectID:    uuidToPtr(issue.ProjectID),
		Status:       syncStatus,
		Provider:     &provider,
		ConnectionID: &connectionID,
		Message:      respMessage,
		Metadata:     updatedMetadata,
		Event:        ptrToIntegrationSyncEventResponse(integrationSyncEventToResponse(event)),
	})
}

type outboundIssueSyncCandidate struct {
	binding    db.IntegrationProjectBinding
	connection db.IntegrationConnection
}

func (h *Handler) outboundIssueSyncCandidates(r *http.Request, workspaceID pgtype.UUID, projectID pgtype.UUID) ([]outboundIssueSyncCandidate, error) {
	bindings, err := h.Queries.ListIntegrationProjectBindings(r.Context(), workspaceID)
	if err != nil {
		return nil, err
	}
	candidates := make([]outboundIssueSyncCandidate, 0, len(bindings))
	for _, binding := range bindings {
		if uuidToString(binding.ProjectID) != uuidToString(projectID) ||
			!binding.OutboundEnabledAt.Valid ||
			!binding.IssueSyncEnabledAt.Valid {
			continue
		}
		connection, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
			ID:          binding.ConnectionID,
			WorkspaceID: workspaceID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		if connection.Status != "active" || !connection.SyncEnabledAt.Valid {
			continue
		}
		candidates = append(candidates, outboundIssueSyncCandidate{
			binding:    binding,
			connection: connection,
		})
	}
	return candidates, nil
}

func selectOutboundIssueSyncCandidate(candidates []outboundIssueSyncCandidate, providerHint string, sourceType string) (outboundIssueSyncCandidate, string, bool) {
	if providerHint != "" {
		for _, candidate := range candidates {
			if candidate.connection.Provider == providerHint {
				return candidate, "outbound issue sync requested for " + providerHint, true
			}
		}
		return outboundIssueSyncCandidate{}, "project has no active outbound issue binding for " + providerHint, false
	}
	if len(candidates) == 0 {
		return outboundIssueSyncCandidate{}, "project has no active outbound issue sync binding", false
	}
	if len(candidates) == 1 {
		provider := candidates[0].connection.Provider
		return candidates[0], "outbound issue sync requested for " + provider, true
	}

	preferred := outboundProviderForSourceType(sourceType)
	if preferred == "" {
		return outboundIssueSyncCandidate{}, "multiple outbound bindings are available; specify provider or source_type", false
	}
	for _, candidate := range candidates {
		if candidate.connection.Provider == preferred {
			return candidate, "outbound issue sync routed to " + preferred + " for source_type=" + sourceType, true
		}
	}
	return outboundIssueSyncCandidate{}, "no outbound binding matches source_type=" + sourceType, false
}

func outboundIssueSyncInFlightStatus(metadata map[string]any) string {
	status := strings.ToLower(strings.TrimSpace(metadataString(metadata, "sync_out_status")))
	switch status {
	case "queued", "processing":
		return status
	default:
		return ""
	}
}

func outboundProviderForSourceType(sourceType string) string {
	switch sourceType {
	case "bug", "task", "story", "requirement", "demand", "zentao":
		return "zentao"
	case "code", "repo", "repository", "merge_request", "mr", "gitlab":
		return "gitlab"
	case "im", "message", "doc", "drive", "wiki", "feishu":
		return "feishu"
	default:
		return ""
	}
}

// createExternalIssueForOutbound creates a real external issue for the selected
// outbound binding using the triggering user's stored, decrypted credential.
// Returns the external source id/url on success. created=false with errMsg
// otherwise; a "not yet implemented" errMsg means the request should degrade to
// a queued state rather than a hard failure.
func (h *Handler) createExternalIssueForOutbound(ctx context.Context, candidate outboundIssueSyncCandidate, issue db.Issue, userUUID pgtype.UUID) (sourceID, sourceURL string, created bool, errMsg string) {
	conn := candidate.connection
	if h.IntegrationSecrets == nil {
		return "", "", false, "credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)"
	}
	accounts, err := h.Queries.ListIntegrationUserAccountsForUser(ctx, db.ListIntegrationUserAccountsForUserParams{
		WorkspaceID: issue.WorkspaceID,
		UserID:      userUUID,
	})
	if err != nil {
		return "", "", false, "failed to load integration account"
	}
	var credEnc []byte
	for _, a := range accounts {
		if uuidToString(a.ConnectionID) == uuidToString(conn.ID) && len(a.CredentialEncrypted) > 0 {
			credEnc = a.CredentialEncrypted
			break
		}
	}
	if len(credEnc) == 0 {
		return "", "", false, "no stored credential for this connection; add an account under Tokens"
	}
	credential, err := h.IntegrationSecrets.Open(credEnc)
	if err != nil {
		return "", "", false, "failed to decrypt stored credential"
	}
	description := ""
	if issue.Description.Valid {
		description = issue.Description.String
	}
	switch conn.Provider {
	case "gitlab":
		ref := gitlabProjectRefFromExternalRef(candidate.binding.ExternalRef)
		if ref == "" {
			return "", "", false, "gitlab binding is missing a project reference"
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		sid, surl, _, cerr := gitlab.New(baseURL, string(credential), nil).CreateIssue(ctx, ref, issue.Title, description)
		if cerr != nil {
			return "", "", false, cerr.Error()
		}
		return sid, surl, true, ""
	case "zentao":
		execID := zentaoProjectIDFromRef(candidate.binding.ExternalRef)
		if execID == "" {
			return "", "", false, "zentao binding is missing a project/execution id"
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		est := time.Now().UTC().Format("2006-01-02")
		due := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		taskID, msg2, cerr := zentao.New(baseURL, zentaoParseCredential(credential).Token, nil).CreateTask(ctx, execID, issue.Title, description, est, due)
		if cerr != nil {
			return "", "", false, cerr.Error()
		}
		if taskID == "" {
			return "", "", false, msg2
		}
		surl := strings.TrimRight(baseURL, "/") + "/task-view-" + taskID + ".html"
		return taskID, surl, true, ""
	case "feishu":
		appID := jsonStringField(conn.Config, "app_id")
		if appID == "" {
			return "", "", false, "feishu connection is missing app_id in config"
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		guid, turl, cerr := feishu.New(baseURL, appID, string(credential), nil).CreateTask(ctx, issue.Title, description)
		if cerr != nil {
			return "", "", false, cerr.Error()
		}
		return guid, turl, true, ""
	default:
		return "", "", false, "outbound creation for " + conn.Provider + " is not yet implemented"
	}
}

// jsonStringField extracts a top-level string field from a JSON blob.
func jsonStringField(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// feishuFolderTokenFromRef pulls a Drive folder token from a resource_ref
// ({"folder_token":...} or a /drive/folder/<token> URL).
func feishuFolderTokenFromRef(raw []byte) string {
	if t := jsonStringField(raw, "folder_token"); t != "" {
		return t
	}
	if u := jsonStringField(raw, "drive_url"); u != "" {
		if i := strings.Index(u, "/folder/"); i >= 0 {
			rest := u[i+len("/folder/"):]
			if cut := strings.IndexAny(rest, "?/"); cut >= 0 {
				rest = rest[:cut]
			}
			return rest
		}
	}
	return ""
}

// zentaoCredential is a stored ZenTao credential. The login flow stores all
// three (enabling silent re-login when the ~24h session token expires); the
// legacy paste flow stores only a bare token.
type zentaoCredential struct {
	Token    string `json:"token"`
	Account  string `json:"account"`
	Password string `json:"password"`
}

// zentaoParseCredential interprets a stored credential as either a JSON blob
// (login flow) or a bare token string (paste flow).
func zentaoParseCredential(raw []byte) zentaoCredential {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, "{") {
		var z zentaoCredential
		if json.Unmarshal(raw, &z) == nil && z.Token != "" {
			return z
		}
	}
	return zentaoCredential{Token: s}
}

// persistZentaoToken writes a refreshed ZenTao token back to the account so the
// next tick reuses it instead of logging in again.
func (h *Handler) persistZentaoToken(ctx context.Context, t InboundSyncTarget, zc zentaoCredential) {
	if h.IntegrationSecrets == nil {
		return
	}
	b, err := json.Marshal(zc)
	if err != nil {
		return
	}
	sealed, err := h.IntegrationSecrets.Seal(b)
	if err != nil {
		return
	}
	_, _ = h.DB.Exec(ctx, `UPDATE integration_user_account SET credential_encrypted=$1, updated_at=now() WHERE connection_id=$2 AND user_id=$3 AND account_key=$4`,
		sealed, t.ConnectionID, t.UserID, t.AccountKey)
}

// InboundSyncTarget is one (connection, user, project binding) unit the inbound
// worker polls: a user who enabled inbound for a provider, has a stored
// credential for the connection, and an inbound-enabled project binding.
type InboundSyncTarget struct {
	ConnectionID        pgtype.UUID
	WorkspaceID         pgtype.UUID
	Provider            string
	BaseURL             pgtype.Text
	UserID              pgtype.UUID
	ProjectID           pgtype.UUID
	ExternalRef         []byte
	CredentialEncrypted []byte
	AccountKey          string
	Config              []byte
}

// ListInboundSyncTargets enumerates, across all workspaces, the inbound polling
// units. Joins connection + per-user inbound setting + per-user credential +
// inbound-enabled project binding.
func (h *Handler) ListInboundSyncTargets(ctx context.Context) ([]InboundSyncTarget, error) {
	// Backfill: existing zentao_project resources that resolve to a ZenTao
	// execution but predate the create/update auto-binding still have no inbound
	// binding, so they'd be invisible to the query below until re-saved. Reconcile
	// them here (best-effort) so they're picked up without user action.
	h.reconcileZentaoExecutionBindings(ctx)

	const q = `
SELECT c.id, c.workspace_id, c.provider, c.base_url, s.user_id, b.project_id, b.external_ref, a.credential_encrypted, a.account_key, c.config
FROM integration_connection c
JOIN integration_issue_sync_setting s
  ON s.workspace_id = c.workspace_id AND s.provider = c.provider AND s.inbound_enabled_at IS NOT NULL
JOIN integration_user_account a
  ON a.workspace_id = c.workspace_id AND a.connection_id = c.id AND a.user_id = s.user_id AND a.credential_encrypted IS NOT NULL
JOIN integration_project_binding b
  ON b.connection_id = c.id AND b.inbound_enabled_at IS NOT NULL
WHERE c.status = 'active' AND c.sync_enabled_at IS NOT NULL
ORDER BY c.provider, c.workspace_id, s.user_id`
	rows, err := h.DB.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboundSyncTarget
	for rows.Next() {
		var t InboundSyncTarget
		if err := rows.Scan(&t.ConnectionID, &t.WorkspaceID, &t.Provider, &t.BaseURL, &t.UserID, &t.ProjectID, &t.ExternalRef, &t.CredentialEncrypted, &t.AccountKey, &t.Config); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SyncInboundTarget pulls external items for one target and upserts issue
// mirrors (idempotent by source_system+source_id). Returns how many it imported.
func (h *Handler) SyncInboundTarget(ctx context.Context, t InboundSyncTarget) (count int, retErr error) {
	// Reflect auth failures (expired/invalid token) onto the account's health so
	// the UI/API can show "action needed" instead of a misleading "active". A
	// later successful sync clears it. Non-auth (transient) errors don't flip it.
	defer func() { h.recordInboundAccountHealth(ctx, t, retErr) }()
	if h.IntegrationSecrets == nil {
		return 0, nil
	}
	cred, err := h.IntegrationSecrets.Open(t.CredentialEncrypted)
	if err != nil {
		return 0, fmt.Errorf("decrypt credential: %w", err)
	}
	conn := db.IntegrationConnection{
		ID: t.ConnectionID, WorkspaceID: t.WorkspaceID, Provider: t.Provider,
		BaseUrl: t.BaseURL, Status: "active",
	}
	baseURL := ""
	if t.BaseURL.Valid {
		baseURL = t.BaseURL.String
	}
	actor := t.UserID
	// Inbound mirrors are unassigned by default; personal-task providers (feishu)
	// assign to the connecting user so the items surface in that user's My Issues.
	assignee := pgtype.UUID{}
	if t.Provider == "feishu" {
		assignee = actor
	}
	count = 0
	upsert := func(title string, desc *string, sourceType, sourceID, sourceURL, externalStatus string) {
		if sourceID == "" {
			return
		}
		if _, _, _, e := h.upsertInboundIntegrationIssue(ctx, conn, actor, assignee, t.ProjectID, pgtype.UUID{}, title, desc, sourceType, sourceID, sourceURL, externalStatus); e == nil {
			count++
		} else {
			slog.Warn("inbound sync: upsert failed", "provider", t.Provider, "source_id", sourceID, "error", e)
		}
	}

	switch t.Provider {
	case "gitlab":
		ref := gitlabProjectRefFromExternalRef(t.ExternalRef)
		if ref == "" {
			return 0, nil
		}
		issues, err := gitlab.New(baseURL, string(cred), nil).ListIssues(ctx, ref, "all", "", 50)
		if err != nil {
			return 0, err
		}
		for _, is := range issues {
			desc := is.Description
			upsert(is.Title, &desc, "issue", strconv.FormatInt(is.IID, 10), is.WebURL, is.State)
		}
	case "zentao":
		exec := zentaoProjectIDFromRef(t.ExternalRef)
		if exec == "" {
			return 0, nil
		}
		zc := zentaoParseCredential(cred)
		tasks, err := zentao.New(baseURL, zc.Token, nil).ListTasks(ctx, exec)
		if err != nil && strings.Contains(err.Error(), "HTTP 401") && zc.Account != "" && zc.Password != "" {
			// 24h session token expired — re-login, persist, retry once.
			if newTok, lErr := zentao.New(baseURL, "", nil).Login(ctx, zc.Account, zc.Password); lErr == nil {
				zc.Token = newTok
				h.persistZentaoToken(ctx, t, zc)
				tasks, err = zentao.New(baseURL, newTok, nil).ListTasks(ctx, exec)
			}
		}
		if err != nil {
			return 0, err
		}
		for _, tk := range tasks {
			desc := tk.Desc
			url := strings.TrimRight(baseURL, "/") + "/task-view-" + tk.ID + ".html"
			upsert(tk.Name, &desc, "task", tk.ID, url, tk.Status)
		}
	case "feishu":
		// Personal Feishu tasks are only visible to a user_access_token. Only the
		// OAuth account holds the user's refresh token (the other feishu account
		// holds the app_secret for outbound); skip everything else.
		if t.AccountKey != feishuOAuthAccountKey {
			return 0, nil
		}
		var blob struct {
			RefreshToken    string `json:"refresh_token"`
			AccessToken     string `json:"access_token"`
			AccessExpiresAt int64  `json:"access_expires_at"`
		}
		if err := json.Unmarshal(cred, &blob); err != nil || blob.RefreshToken == "" {
			return 0, nil
		}
		appSecret, secErr := h.connectionAppSecretFromConfig(t.Config)
		if secErr != nil {
			// Don't silently skip: surface the misconfiguration so it shows as
			// "action needed" (recordInboundAccountHealth) instead of looking
			// healthy while personal-task inbound never actually runs.
			return 0, fmt.Errorf("feishu inbound disabled — connection is missing App ID/Secret (set them in Integrations): %w", secErr)
		}
		appID := jsonStringField(t.Config, "app_id")
		if appID == "" {
			return 0, fmt.Errorf("feishu inbound disabled — connection is missing App ID (set it in Integrations)")
		}
		fc := feishu.New(baseURL, appID, appSecret, nil)
		accessToken := blob.AccessToken
		// Feishu refresh tokens are SINGLE-USE (each refresh rotates them), so reuse
		// the cached ~2h access token and only refresh when it's missing or within
		// 5 min of expiry. Persist the rotated refresh token + new access token.
		if accessToken == "" || blob.AccessExpiresAt-time.Now().Unix() < 300 {
			tok, err := fc.RefreshUserToken(ctx, appID, appSecret, blob.RefreshToken)
			if err != nil {
				return 0, err
			}
			accessToken = tok.AccessToken
			newRefresh := blob.RefreshToken
			if tok.RefreshToken != "" {
				newRefresh = tok.RefreshToken
			}
			if nb, mErr := json.Marshal(map[string]any{
				"refresh_token":     newRefresh,
				"access_token":      tok.AccessToken,
				"access_expires_at": time.Now().Unix() + int64(tok.ExpiresIn),
			}); mErr == nil {
				if sealed, sErr := h.IntegrationSecrets.Seal(nb); sErr == nil {
					_, _ = h.DB.Exec(ctx, `UPDATE integration_user_account SET credential_encrypted=$1, updated_at=now() WHERE connection_id=$2 AND user_id=$3 AND account_key=$4`,
						sealed, t.ConnectionID, t.UserID, feishuOAuthAccountKey)
				}
			}
		}
		tasks, err := fc.ListTasksWithUserToken(ctx, accessToken)
		if err != nil {
			return 0, err
		}
		for _, tk := range tasks {
			upsert(tk.Summary, nil, "feishu_task", tk.GUID, tk.URL, "todo")
		}
	}
	return count, nil
}

// isIntegrationAuthError reports whether a sync error is a credential/auth
// failure (expired or invalid token) — the kind a user must resolve by
// reconnecting — versus a transient network/server error.
func isIntegrationAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sig := range []string{
		"401", "unauthorized", "e1004", "invalid_grant",
		"invalid token", "token revoked", "token expired", "token has expired",
		"missing app id", "no app_secret", // connection not fully configured → user must act
	} {
		if strings.Contains(s, sig) {
			return true
		}
	}
	return false
}

// recordInboundAccountHealth writes the result of an inbound sync onto the
// account's health fields. An auth failure flips the account to status='error'
// with last_error set (surfaced as "action needed" in the UI/API); a later
// success clears that flag. Transient errors are left as-is so a blip doesn't
// nag the user to reconnect.
func (h *Handler) recordInboundAccountHealth(ctx context.Context, t InboundSyncTarget, syncErr error) {
	if syncErr == nil {
		_, _ = h.DB.Exec(ctx,
			`UPDATE integration_user_account SET status='active', last_error=NULL, last_used_at=now(), updated_at=now()
			 WHERE connection_id=$1 AND user_id=$2 AND account_key=$3 AND status='error'`,
			t.ConnectionID, t.UserID, t.AccountKey)
		return
	}
	if !isIntegrationAuthError(syncErr) {
		return
	}
	msg := syncErr.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = h.DB.Exec(ctx,
		`UPDATE integration_user_account SET status='error', last_error=$1, updated_at=now()
		 WHERE connection_id=$2 AND user_id=$3 AND account_key=$4`,
		msg, t.ConnectionID, t.UserID, t.AccountKey)
}

// providerConnectionCredential finds the workspace connection for a provider
// plus the triggering user's decrypted credential for it. errMsg is non-empty
// when no usable connection/credential is available.
func (h *Handler) providerConnectionCredential(ctx context.Context, workspaceID, userUUID pgtype.UUID, provider string) (db.IntegrationConnection, string, string) {
	conns, err := h.Queries.ListIntegrationConnections(ctx, workspaceID)
	if err != nil {
		return db.IntegrationConnection{}, "", "failed to load integration connections"
	}
	var conn db.IntegrationConnection
	found := false
	for _, c := range conns {
		if c.Provider == provider {
			conn = c
			found = true
			break
		}
	}
	if !found {
		return db.IntegrationConnection{}, "", "no " + provider + " connection configured"
	}
	if h.IntegrationSecrets == nil {
		return conn, "", "credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)"
	}
	accounts, err := h.Queries.ListIntegrationUserAccountsForUser(ctx, db.ListIntegrationUserAccountsForUserParams{
		WorkspaceID: workspaceID,
		UserID:      userUUID,
	})
	if err != nil {
		return conn, "", "failed to load integration account"
	}
	for _, a := range accounts {
		if uuidToString(a.ConnectionID) == uuidToString(conn.ID) && len(a.CredentialEncrypted) > 0 {
			plain, derr := h.IntegrationSecrets.Open(a.CredentialEncrypted)
			if derr != nil {
				return conn, "", "failed to decrypt stored credential"
			}
			return conn, string(plain), ""
		}
	}
	return conn, "", "no stored credential for " + provider + "; add an account under Tokens"
}

// zentaoStatusFor maps a Multica status to a ZenTao task status (per the
// approved mapping). ZenTao has the richest status model of the three.
func zentaoStatusFor(status string) string {
	switch status {
	case "done":
		// "closed" instead of "done" to bypass ZenTao's finish-requires-effort
		// (consumed > 0) rule. User-chosen mapping.
		return "closed"
	case "cancelled":
		return "cancel"
	case "in_progress", "in_review":
		return "doing"
	case "blocked":
		return "pause"
	default: // backlog, todo
		return "wait"
	}
}

// feishuUserAccessToken returns a usable user_access_token for the member's feishu
// OAuth account, preferring the cached token (refresh tokens are single-use, so
// it only refreshes within 5 min of expiry — the inbound worker keeps it warm).
func (h *Handler) feishuUserAccessToken(ctx context.Context, workspaceID, userID pgtype.UUID, config []byte) (string, error) {
	var credEnc []byte
	if err := h.DB.QueryRow(ctx,
		`SELECT credential_encrypted FROM integration_user_account WHERE workspace_id=$1 AND user_id=$2 AND account_key=$3 AND credential_encrypted IS NOT NULL`,
		workspaceID, userID, feishuOAuthAccountKey).Scan(&credEnc); err != nil {
		return "", err
	}
	cred, err := h.IntegrationSecrets.Open(credEnc)
	if err != nil {
		return "", err
	}
	var blob struct {
		RefreshToken    string `json:"refresh_token"`
		AccessToken     string `json:"access_token"`
		AccessExpiresAt int64  `json:"access_expires_at"`
	}
	_ = json.Unmarshal(cred, &blob)
	if blob.AccessToken != "" && blob.AccessExpiresAt-time.Now().Unix() > 300 {
		return blob.AccessToken, nil
	}
	if blob.RefreshToken == "" {
		return "", fmt.Errorf("no feishu refresh token")
	}
	appID := jsonStringField(config, "app_id")
	appSecret, secErr := h.connectionAppSecretFromConfig(config)
	if secErr != nil {
		return "", secErr
	}
	tok, err := feishu.New("", appID, appSecret, nil).RefreshUserToken(ctx, appID, appSecret, blob.RefreshToken)
	if err != nil {
		return "", err
	}
	newRefresh := blob.RefreshToken
	if tok.RefreshToken != "" {
		newRefresh = tok.RefreshToken
	}
	if nb, e := json.Marshal(map[string]any{"refresh_token": newRefresh, "access_token": tok.AccessToken, "access_expires_at": time.Now().Unix() + int64(tok.ExpiresIn)}); e == nil {
		if sealed, se := h.IntegrationSecrets.Seal(nb); se == nil {
			_, _ = h.DB.Exec(ctx, `UPDATE integration_user_account SET credential_encrypted=$1, updated_at=now() WHERE workspace_id=$2 AND user_id=$3 AND account_key=$4`, sealed, workspaceID, userID, feishuOAuthAccountKey)
		}
	}
	return tok.AccessToken, nil
}

// pushIssueStatusOutbound (run async, goroutine after the local commit) syncs a
// mirrored issue's new status to its external source. Hybrid-gated: the project
// binding's outbound flag (admin) AND the changer's per-user outbound setting
// must both be on. Native (non-mirrored) issues are ignored. Idempotent — it
// sets the external state to match Multica, so the next inbound tick sees no
// change. Identity = the changer's own provider credential.
func (h *Handler) pushIssueStatusOutbound(issue db.Issue, status string, actorUserID pgtype.UUID) {
	defer func() { _ = recover() }()
	if h.IntegrationSecrets == nil {
		return
	}
	meta := parseIssueMetadata(issue.Metadata)
	provider := metadataString(meta, "source_system")
	sourceID := metadataString(meta, "source_id")
	if provider == "" || sourceID == "" {
		return // not a mirror → never push native issues out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// One query enforces ALL hybrid gates AND fetches creds/refs.
	const q = `
SELECT c.base_url, c.config, a.credential_encrypted, b.external_ref
FROM integration_connection c
JOIN integration_project_binding b
  ON b.connection_id = c.id AND b.project_id = $3 AND b.outbound_enabled_at IS NOT NULL
JOIN integration_issue_sync_setting s
  ON s.workspace_id = c.workspace_id AND s.provider = c.provider AND s.user_id = $2 AND s.outbound_enabled_at IS NOT NULL
JOIN integration_user_account a
  ON a.connection_id = c.id AND a.user_id = $2 AND a.credential_encrypted IS NOT NULL
WHERE c.workspace_id = $1 AND c.provider = $4 AND c.status = 'active' AND c.sync_enabled_at IS NOT NULL
ORDER BY a.account_key LIMIT 1`
	var baseURL pgtype.Text
	var config, credEnc, externalRef []byte
	if err := h.DB.QueryRow(ctx, q, issue.WorkspaceID, actorUserID, issue.ProjectID, provider).Scan(&baseURL, &config, &credEnc, &externalRef); err != nil {
		return // a gate is off, or the changer has no credential → skip silently
	}
	base := ""
	if baseURL.Valid {
		base = baseURL.String
	}
	cred, err := h.IntegrationSecrets.Open(credEnc)
	if err != nil {
		return
	}
	switch provider {
	case "gitlab":
		ref := gitlabProjectRefFromExternalRef(externalRef)
		iid, perr := strconv.ParseInt(sourceID, 10, 64)
		if ref == "" || perr != nil {
			return
		}
		stateEvent := "reopen"
		if status == "done" || status == "cancelled" {
			stateEvent = "close"
		}
		if e := gitlab.New(base, string(cred), nil).UpdateIssueState(ctx, ref, iid, stateEvent); e != nil {
			slog.Warn("outbound status sync failed", "provider", "gitlab", "issue", uuidToString(issue.ID), "error", e)
		}
	case "zentao":
		zc := zentaoParseCredential(cred)
		if e := zentao.New(base, zc.Token, nil).UpdateTaskStatus(ctx, sourceID, zentaoStatusFor(status)); e != nil {
			slog.Warn("outbound status sync failed", "provider", "zentao", "issue", uuidToString(issue.ID), "error", e)
		}
	case "feishu":
		at, te := h.feishuUserAccessToken(ctx, issue.WorkspaceID, actorUserID, config)
		if te != nil {
			return
		}
		done := status == "done" || status == "cancelled"
		if e := feishu.New(base, "", "", nil).CompleteTask(ctx, at, sourceID, done); e != nil {
			slog.Warn("outbound status sync failed", "provider", "feishu", "issue", uuidToString(issue.ID), "error", e)
		}
	}
}

// pushIssueCommentOutbound (run async, goroutine after the comment commits) posts
// a comment added to a MIRRORED issue back to its external source. Same hybrid
// gating + changer identity as the status path. Native issues are ignored.
func (h *Handler) pushIssueCommentOutbound(issue db.Issue, body string, actorUserID pgtype.UUID) {
	defer func() { _ = recover() }()
	if h.IntegrationSecrets == nil || strings.TrimSpace(body) == "" {
		return
	}
	meta := parseIssueMetadata(issue.Metadata)
	provider := metadataString(meta, "source_system")
	sourceID := metadataString(meta, "source_id")
	if provider == "" || sourceID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if provider == "zentao" {
		// ZenTao 16.5 v1 REST has NO standalone "add comment" endpoint. Per the
		// open-source server (github.com/easysoft/zentaopms): comments live in the
		// ACTION table and are only written by action->create(), which v1 reaches
		// solely through status-change entries (taskclose/taskfinish/… whose
		// batchSetPost('comment') records the comment) — those mutate task status
		// as a side effect, so they cannot carry a plain comment. The generic
		// PUT /tasks/{id} entry setPosts only status/consumed/left (comment is
		// dropped — previously confirmed as a no-op). There is therefore no way
		// to deliver a comment without altering the task, so this is a hard
		// FAILURE, recorded explicitly (status=error) rather than faked as OK.
		h.recordIntegrationOutboundEvent(ctx, issue.WorkspaceID,
			h.lookupActiveConnectionID(ctx, issue.WorkspaceID, "zentao"), issue.ProjectID,
			"zentao", "comment", uuidToString(issue.ID), sourceID, "error",
			"ZenTao comment outbound FAILED: ZenTao v1 REST has no comment endpoint; comment was NOT written to the task",
			"unsupported: zentao v1 has no standalone comment endpoint")
		return
	}
	const q = `
SELECT c.base_url, c.config, a.credential_encrypted, b.external_ref
FROM integration_connection c
JOIN integration_project_binding b
  ON b.connection_id = c.id AND b.project_id = $3 AND b.outbound_enabled_at IS NOT NULL
JOIN integration_issue_sync_setting s
  ON s.workspace_id = c.workspace_id AND s.provider = c.provider AND s.user_id = $2 AND s.outbound_enabled_at IS NOT NULL
JOIN integration_user_account a
  ON a.connection_id = c.id AND a.user_id = $2 AND a.credential_encrypted IS NOT NULL
WHERE c.workspace_id = $1 AND c.provider = $4 AND c.status = 'active' AND c.sync_enabled_at IS NOT NULL
ORDER BY a.account_key LIMIT 1`
	var baseURL pgtype.Text
	var config, credEnc, externalRef []byte
	if err := h.DB.QueryRow(ctx, q, issue.WorkspaceID, actorUserID, issue.ProjectID, provider).Scan(&baseURL, &config, &credEnc, &externalRef); err != nil {
		return // a gate is off, or the commenter has no credential → skip silently
	}
	base := ""
	if baseURL.Valid {
		base = baseURL.String
	}
	cred, err := h.IntegrationSecrets.Open(credEnc)
	if err != nil {
		return
	}
	switch provider {
	case "gitlab":
		ref := gitlabProjectRefFromExternalRef(externalRef)
		iid, perr := strconv.ParseInt(sourceID, 10, 64)
		if ref == "" || perr != nil {
			return
		}
		if e := gitlab.New(base, string(cred), nil).CreateIssueNote(ctx, ref, iid, body); e != nil {
			slog.Warn("outbound comment sync failed", "provider", "gitlab", "issue", uuidToString(issue.ID), "error", e)
		}
	case "feishu":
		at, te := h.feishuUserAccessToken(ctx, issue.WorkspaceID, actorUserID, config)
		if te != nil {
			return
		}
		if e := feishu.New(base, "", "", nil).CreateTaskComment(ctx, at, sourceID, body); e != nil {
			slog.Warn("outbound comment sync failed", "provider", "feishu", "issue", uuidToString(issue.ID), "error", e)
		}
	}
}

// recordIntegrationOutboundEvent appends an outbound sync event from an async
// (no *http.Request) context, so outbound attempts stay visible/auditable even
// when skipped, unsupported, or failed.
func (h *Handler) recordIntegrationOutboundEvent(ctx context.Context, workspaceID, connectionID, projectID pgtype.UUID, provider, objectType, objectID, externalID, status, message, errMsg string) {
	if _, err := h.Queries.CreateIntegrationSyncEvent(ctx, db.CreateIntegrationSyncEventParams{
		WorkspaceID:  workspaceID,
		ConnectionID: connectionID,
		Provider:     provider,
		Direction:    "outbound",
		ObjectType:   objectType,
		ObjectID:     ptrToText(&objectID),
		ExternalID:   ptrToText(&externalID),
		ProjectID:    projectID,
		Status:       status,
		Message:      ptrToText(&message),
		Error:        ptrToText(&errMsg),
		Metadata:     []byte("{}"),
	}); err != nil {
		slog.Warn("integration outbound event write failed", "provider", provider, "status", status, "error", err)
	}
}

// lookupActiveConnectionID returns the workspace's active connection id for a
// provider (oldest first), or an invalid UUID if none. Best-effort, for event
// attribution where the gate query didn't already resolve the connection.
func (h *Handler) lookupActiveConnectionID(ctx context.Context, workspaceID pgtype.UUID, provider string) pgtype.UUID {
	var id pgtype.UUID
	_ = h.DB.QueryRow(ctx, `SELECT id FROM integration_connection WHERE workspace_id=$1 AND provider=$2 AND status='active' ORDER BY created_at LIMIT 1`, workspaceID, provider).Scan(&id)
	return id
}

// handleMirrorIssueDeleted (run async after a mirror issue is deleted) ALWAYS
// writes a tombstone — ungated — so the next inbound poll does not resurrect the
// deleted mirror. It then best-effort cancels/closes the external item, gated by
// the same hybrid outbound gate (binding + per-user) as status/comment sync.
// Native (non-mirror) issues are ignored.
func (h *Handler) handleMirrorIssueDeleted(issue db.Issue, actorUserID pgtype.UUID) {
	defer func() { _ = recover() }()
	meta := parseIssueMetadata(issue.Metadata)
	provider := metadataString(meta, "source_system")
	sourceID := metadataString(meta, "source_id")
	if provider == "" || sourceID == "" {
		return // not a mirror → nothing to suppress or cancel
	}
	sourceURL := metadataString(meta, "source_url")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1) Tombstone — UNCONDITIONAL. Independent of outbound gates so a deleted
	//    mirror never reappears, even when the workspace never enabled outbound.
	if _, err := h.DB.Exec(ctx, `
INSERT INTO integration_issue_tombstone (workspace_id, provider, source_id, project_id, source_url, issue_id, deleted_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (workspace_id, provider, source_id) DO UPDATE
  SET project_id = EXCLUDED.project_id, source_url = EXCLUDED.source_url,
      issue_id = EXCLUDED.issue_id, deleted_by = EXCLUDED.deleted_by, created_at = now()`,
		issue.WorkspaceID, provider, sourceID, issue.ProjectID, ptrToText(&sourceURL), issue.ID, actorUserID); err != nil {
		slog.Warn("delete tombstone write failed", "provider", provider, "issue", uuidToString(issue.ID), "error", err)
	}

	// 2) Outbound cancel/close — gated exactly like status/comment sync. The
	//    tombstone above is already durable regardless of what follows.
	if h.IntegrationSecrets == nil {
		return
	}
	if !actorUserID.Valid {
		// The external cancel uses the actor's own per-user credential; with no
		// valid actor (agent/system/internal delete, or a non-UUID token) we
		// can't gate it to a user, so skip the external mutation and leave an
		// audit trail. The tombstone is already written.
		h.recordIntegrationOutboundEvent(ctx, issue.WorkspaceID,
			h.lookupActiveConnectionID(ctx, issue.WorkspaceID, provider), issue.ProjectID,
			provider, "issue", uuidToString(issue.ID), sourceID, "skipped",
			"Mirror tombstoned; external cancel skipped (no valid actor for per-user credential)", "")
		return
	}
	const q = `
SELECT c.id, c.base_url, c.config, a.credential_encrypted, b.external_ref
FROM integration_connection c
JOIN integration_project_binding b
  ON b.connection_id = c.id AND b.project_id = $3 AND b.outbound_enabled_at IS NOT NULL
JOIN integration_issue_sync_setting s
  ON s.workspace_id = c.workspace_id AND s.provider = c.provider AND s.user_id = $2 AND s.outbound_enabled_at IS NOT NULL
JOIN integration_user_account a
  ON a.connection_id = c.id AND a.user_id = $2 AND a.credential_encrypted IS NOT NULL
WHERE c.workspace_id = $1 AND c.provider = $4 AND c.status = 'active' AND c.sync_enabled_at IS NOT NULL
ORDER BY a.account_key LIMIT 1`
	var connID pgtype.UUID
	var baseURL pgtype.Text
	var config, credEnc, externalRef []byte
	if err := h.DB.QueryRow(ctx, q, issue.WorkspaceID, actorUserID, issue.ProjectID, provider).Scan(&connID, &baseURL, &config, &credEnc, &externalRef); err != nil {
		return // outbound gate off / no credential — tombstone already written
	}
	base := ""
	if baseURL.Valid {
		base = baseURL.String
	}
	cred, err := h.IntegrationSecrets.Open(credEnc)
	if err != nil {
		return
	}
	var actErr error
	switch provider {
	case "gitlab":
		ref := gitlabProjectRefFromExternalRef(externalRef)
		iid, perr := strconv.ParseInt(sourceID, 10, 64)
		if ref == "" || perr != nil {
			return
		}
		actErr = gitlab.New(base, string(cred), nil).UpdateIssueState(ctx, ref, iid, "close")
	case "zentao":
		// No verified hard-delete via REST; cancel is the safe, reversible
		// equivalent and the tombstone already prevents resurrection.
		zc := zentaoParseCredential(cred)
		actErr = zentao.New(base, zc.Token, nil).UpdateTaskStatus(ctx, sourceID, "cancel")
	case "feishu":
		at, te := h.feishuUserAccessToken(ctx, issue.WorkspaceID, actorUserID, config)
		if te != nil {
			return
		}
		actErr = feishu.New(base, "", "", nil).CompleteTask(ctx, at, sourceID, true)
	default:
		return
	}
	if actErr != nil {
		slog.Warn("outbound delete sync failed", "provider", provider, "issue", uuidToString(issue.ID), "error", actErr)
		h.recordIntegrationOutboundEvent(ctx, issue.WorkspaceID, connID, issue.ProjectID, provider, "issue", uuidToString(issue.ID), sourceID, "error", "Outbound cancel on delete failed", actErr.Error())
		return
	}
	h.recordIntegrationOutboundEvent(ctx, issue.WorkspaceID, connID, issue.ProjectID, provider, "issue", uuidToString(issue.ID), sourceID, "success", "Mirror deleted; external item cancelled/closed", "")
}

// FetchProjectResourceContent returns real content from a project resource so an
// agent can read it. Dispatches by resource_type. For gitlab_repo it returns
// the file inventory, or a specific file's content when ?path= is supplied.
func (h *Handler) FetchProjectResourceContent(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProjectForResource(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	resourceUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "resourceId"), "resource id")
	if !ok {
		return
	}
	resource, err := h.Queries.GetProjectResourceInWorkspace(r.Context(), db.GetProjectResourceInWorkspaceParams{
		ID:          resourceUUID,
		WorkspaceID: project.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "project resource not found")
		return
	}

	switch resource.ResourceType {
	case "gitlab_repo":
		conn, credential, cerr := h.providerConnectionCredential(r.Context(), project.WorkspaceID, userUUID, "gitlab")
		if cerr != "" {
			writeError(w, http.StatusBadRequest, cerr)
			return
		}
		ref := gitlabProjectRefFromExternalRef(resource.ResourceRef)
		if ref == "" {
			writeError(w, http.StatusBadRequest, "gitlab resource is missing a project reference")
			return
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		gl := gitlab.New(baseURL, credential, nil)
		if path := strings.TrimSpace(r.URL.Query().Get("path")); path != "" {
			content, ferr := gl.GetFile(r.Context(), ref, path, "")
			if ferr != nil {
				writeError(w, http.StatusBadGateway, ferr.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"resource_type": "gitlab_repo",
				"project":       ref,
				"path":          path,
				"content":       content,
			})
			return
		}
		files, ferr := gl.ListFiles(r.Context(), ref, 200)
		if ferr != nil {
			writeError(w, http.StatusBadGateway, ferr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"resource_type": "gitlab_repo",
			"project":       ref,
			"files":         files,
		})
		return
	case "zentao_project":
		conn, credential, cerr := h.providerConnectionCredential(r.Context(), project.WorkspaceID, userUUID, "zentao")
		if cerr != "" {
			writeError(w, http.StatusBadRequest, cerr)
			return
		}
		target := zentaoTargetFromRef(resource.ResourceRef)
		if target.ID == "" {
			writeError(w, http.StatusBadRequest, "zentao resource is missing a project or execution id")
			return
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		client := zentao.New(baseURL, zentaoParseCredential([]byte(credential)).Token, nil)
		if target.IsExecution {
			// An execution (kanban) ref points at /executions/{id}, not a
			// project — return its task list via the execution-specific read
			// method rather than pretending it's a project.
			tasks, ferr := client.ListTasks(r.Context(), target.ID)
			if ferr != nil {
				writeError(w, http.StatusBadGateway, ferr.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"resource_type": "zentao_project",
				"execution_id":  target.ID,
				"tasks":         tasks,
			})
			return
		}
		info, ferr := client.GetProject(r.Context(), target.ID)
		if ferr != nil {
			writeError(w, http.StatusBadGateway, ferr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"resource_type": "zentao_project",
			"project_id":    target.ID,
			"info": map[string]any{
				"id": info["id"], "name": info["name"], "status": info["status"],
				"desc": info["desc"], "model": info["model"],
				"begin": info["begin"], "end": info["end"],
			},
		})
		return
	case "feishu_drive":
		conn, credential, cerr := h.providerConnectionCredential(r.Context(), project.WorkspaceID, userUUID, "feishu")
		if cerr != "" {
			writeError(w, http.StatusBadRequest, cerr)
			return
		}
		appID := jsonStringField(conn.Config, "app_id")
		if appID == "" {
			writeError(w, http.StatusBadRequest, "feishu connection is missing app_id in config")
			return
		}
		folderToken := feishuFolderTokenFromRef(resource.ResourceRef)
		if folderToken == "" {
			writeError(w, http.StatusBadRequest, "feishu resource is missing a folder token")
			return
		}
		baseURL := ""
		if conn.BaseUrl.Valid {
			baseURL = conn.BaseUrl.String
		}
		files, ferr := feishu.New(baseURL, appID, credential, nil).ListDriveFiles(r.Context(), folderToken)
		if ferr != nil {
			writeError(w, http.StatusBadGateway, ferr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"resource_type": "feishu_drive",
			"folder_token":  folderToken,
			"files":         files,
		})
		return
	default:
		writeError(w, http.StatusNotImplemented, "content fetch for "+resource.ResourceType+" is not yet implemented")
		return
	}
}

// zentaoRefTarget is the ZenTao read/poll target resolved from a resource_ref or
// a binding external_ref. ID is the numeric id; IsExecution distinguishes an
// execution (kanban) target — which is read via /executions/{id}/tasks — from a
// plain project target, read via /projects/{id}.
type zentaoRefTarget struct {
	ID          string
	IsExecution bool
}

// zentaoTargetFromRef resolves a ZenTao target from a resource_ref / external_ref
// JSON object. It understands:
//   - explicit keys: execution_id/execution_key/execution (→ execution),
//     project_id/project_key/id (→ project);
//   - URL forms: execution-kanban-447.html / execution-task-447.html etc.
//     (→ execution 447) and project-index-446.html (→ project 446).
//
// An explicit execution key wins over a project key, and an execution URL wins
// over a project-index URL, because the execution endpoint is the one that
// actually carries the tasks an inbound sync mirrors.
func zentaoTargetFromRef(raw []byte) zentaoRefTarget {
	if len(raw) == 0 {
		return zentaoRefTarget{}
	}
	var ref map[string]any
	if err := json.Unmarshal(raw, &ref); err != nil {
		return zentaoRefTarget{}
	}
	if id := zentaoRefIDValue(ref, "execution_id", "execution_key", "execution"); id != "" {
		return zentaoRefTarget{ID: id, IsExecution: true}
	}
	if id := zentaoRefIDValue(ref, "project_id", "project_key", "id"); id != "" {
		return zentaoRefTarget{ID: id}
	}
	if u, ok := ref["url"].(string); ok {
		// Execution URLs look like .../execution-<view>-<id>.html, so the id is
		// the first digit run after the "execution-" marker.
		if id := zentaoFirstDigitRunAfter(u, "execution-"); id != "" {
			return zentaoRefTarget{ID: id, IsExecution: true}
		}
		// Project URLs look like .../project-index-<id>.html. Preserve the
		// original up-to-delimiter extraction (ZenTao ids are numeric, but a
		// non-numeric project key here should still round-trip).
		if i := strings.Index(u, "project-index-"); i >= 0 {
			rest := u[i+len("project-index-"):]
			if dot := strings.IndexAny(rest, ".?/"); dot >= 0 {
				rest = rest[:dot]
			}
			if rest != "" {
				return zentaoRefTarget{ID: rest}
			}
		}
	}
	return zentaoRefTarget{}
}

// zentaoRefIDValue returns the first non-empty string/number value among keys.
func zentaoRefIDValue(ref map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := ref[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

// zentaoFirstDigitRunAfter returns the first run of digits following marker in
// s, e.g. ("…/execution-kanban-447.html", "execution-") → "447". Returns "" when
// the marker is absent or no digits follow it.
func zentaoFirstDigitRunAfter(s, marker string) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	start := -1
	for j := 0; j < len(rest); j++ {
		if rest[j] >= '0' && rest[j] <= '9' {
			if start < 0 {
				start = j
			}
		} else if start >= 0 {
			return rest[start:j]
		}
	}
	if start >= 0 {
		return rest[start:]
	}
	return ""
}

// zentaoProjectIDFromRef extracts the ZenTao id from a resource_ref / external_ref
// JSON, treating an execution target's id the same as a project id for callers
// (inbound/outbound task sync) that operate on whichever id the ref points at.
func zentaoProjectIDFromRef(raw []byte) string {
	return zentaoTargetFromRef(raw).ID
}

// syncZentaoExecutionBinding mirrors a zentao_project resource that resolves to a
// ZenTao execution into an inbound-enabled integration_project_binding, so the
// scheduler's ListInboundSyncTargets picks it up automatically (a project
// resource alone is otherwise invisible to inbound sync). Invoked best-effort
// from resource create/update.
//
// It acts only when the ref resolves to an execution and the workspace has
// exactly one active ZenTao connection. With zero active connections there is
// nothing to bind to; with several it is ambiguous which one to target, so it
// stays conservative and does nothing. Existing outbound/issue/knowledge flags
// on an already-present binding are preserved — only inbound is force-enabled.
//
// prevRef is the resource's ref before this change (nil on create). When a
// resource is repointed from an execution to a non-execution target, the
// previously auto-created inbound flag is cleared so the old execution stops
// being polled — but only the inbound flag, leaving any manually enabled
// outbound/issue/knowledge sync intact.
func (h *Handler) syncZentaoExecutionBinding(ctx context.Context, project db.Project, resourceRef, prevRef []byte, creator pgtype.UUID) {
	target := zentaoTargetFromRef(resourceRef)
	prev := zentaoTargetFromRef(prevRef)

	enabling := target.IsExecution && target.ID != ""
	// Tear-down case: the new ref is no longer an execution, but the previous
	// ref was — so an auto-created inbound binding may be left polling a stale
	// execution. Stay narrow: only react to this specific transition.
	disabling := !enabling && prev.IsExecution && prev.ID != ""
	if !enabling && !disabling {
		return
	}

	conns, err := h.Queries.ListIntegrationConnections(ctx, project.WorkspaceID)
	if err != nil {
		slog.Warn("zentao auto-binding: list connections failed", "workspace_id", uuidToString(project.WorkspaceID), "error", err)
		return
	}
	var active []db.IntegrationConnection
	for _, c := range conns {
		if c.Provider == "zentao" && c.Status == "active" {
			active = append(active, c)
		}
	}
	if len(active) != 1 {
		return
	}
	conn := active[0]

	// Preserve any non-inbound flags a user may already have set on this binding;
	// the upsert clears unset flags to NULL otherwise.
	outbound, issueSync, knowledge, inbound, hasBinding := false, false, false, false, false
	var existingRef []byte
	if bindings, berr := h.Queries.ListIntegrationProjectBindings(ctx, project.WorkspaceID); berr == nil {
		for _, b := range bindings {
			if uuidToString(b.ProjectID) == uuidToString(project.ID) && uuidToString(b.ConnectionID) == uuidToString(conn.ID) {
				outbound = b.OutboundEnabledAt.Valid
				issueSync = b.IssueSyncEnabledAt.Valid
				knowledge = b.KnowledgeSyncEnabledAt.Valid
				inbound = b.InboundEnabledAt.Valid
				existingRef = b.ExternalRef
				hasBinding = true
				break
			}
		}
	}

	// In the tear-down case there is nothing to do unless an inbound-enabled
	// binding actually exists; never create a binding just to disable it.
	if disabling && (!hasBinding || !inbound) {
		return
	}

	// Determine the external_ref to store. When enabling, normalize to the
	// execution id so a later inbound poll resolves it without re-parsing the
	// page URL. When disabling, repoint at the new (non-execution) target id if
	// resolvable, otherwise leave the previously stored ref untouched.
	var externalRef []byte
	switch {
	case enabling:
		externalRef, _ = json.Marshal(map[string]string{"execution_id": target.ID})
	case target.ID != "":
		externalRef, _ = json.Marshal(map[string]string{"project_id": target.ID})
	default:
		externalRef = existingRef
	}

	if _, err := h.Queries.UpsertIntegrationProjectBinding(ctx, db.UpsertIntegrationProjectBindingParams{
		WorkspaceID:     project.WorkspaceID,
		ProjectID:       project.ID,
		ConnectionID:    conn.ID,
		ExternalRef:     externalRef,
		CreatedByUserID: creator,
		InboundEnabled:  enabling,
		// Execution bindings are bidirectional issue-sync targets: when the
		// execution resource is in use (enabling), default outbound + issue_sync
		// ON so the binding isn't stuck inbound-only. Outbound delivery is still
		// gated per-user; tear-down (disabling) only clears inbound, preserving
		// whatever outbound/issue_sync was already set.
		OutboundEnabled:      outbound || enabling,
		IssueSyncEnabled:     issueSync || enabling,
		KnowledgeSyncEnabled: knowledge,
	}); err != nil {
		slog.Warn("zentao auto-binding: upsert failed", "workspace_id", uuidToString(project.WorkspaceID), "connection_id", uuidToString(conn.ID), "error", err)
	}
}

// reconcileZentaoExecutionBindings backfills inbound bindings for zentao_project
// resources that resolve to a ZenTao execution but predate the create/update
// auto-binding (so they never triggered syncZentaoExecutionBinding). It scans all
// such resources, then routes each execution-resolving one through the same
// conservative syncZentaoExecutionBinding path — which only binds when the
// workspace has exactly one active ZenTao connection and preserves any existing
// outbound/issue/knowledge flags. Best-effort and idempotent: re-running upserts
// the same binding. prevRef is nil here, so this only enables (never tears down).
func (h *Handler) reconcileZentaoExecutionBindings(ctx context.Context) {
	const q = `SELECT project_id, workspace_id, resource_ref, created_by FROM project_resource WHERE resource_type = 'zentao_project'`
	rows, err := h.DB.Query(ctx, q)
	if err != nil {
		slog.Warn("zentao reconcile: list resources failed", "error", err)
		return
	}
	type execResource struct {
		projectID, workspaceID, creator pgtype.UUID
		ref                             []byte
	}
	var execs []execResource
	for rows.Next() {
		var rr execResource
		if err := rows.Scan(&rr.projectID, &rr.workspaceID, &rr.ref, &rr.creator); err != nil {
			rows.Close()
			slog.Warn("zentao reconcile: scan failed", "error", err)
			return
		}
		// Only execution targets need a backfilled inbound binding; skip plain
		// project resources (and anything that doesn't resolve) without touching
		// the DB further. This bounds the per-resource binding work below to the
		// rare execution resources.
		if t := zentaoTargetFromRef(rr.ref); t.IsExecution && t.ID != "" {
			execs = append(execs, rr)
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		slog.Warn("zentao reconcile: iterate resources failed", "error", rowsErr)
		return
	}

	for _, rr := range execs {
		h.syncZentaoExecutionBinding(ctx, db.Project{ID: rr.projectID, WorkspaceID: rr.workspaceID}, rr.ref, nil, rr.creator)
	}
}

// gitlabProjectRefFromExternalRef extracts a GitLab project reference (path
// "group/repo" or a numeric id) from a binding's external_ref JSON.
func gitlabProjectRefFromExternalRef(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var ref map[string]any
	if err := json.Unmarshal(raw, &ref); err != nil {
		return ""
	}
	for _, key := range []string{"project", "project_path", "path_with_namespace"} {
		if v, ok := ref[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if v, ok := ref["project_id"]; ok {
		switch n := v.(type) {
		case string:
			if strings.TrimSpace(n) != "" {
				return strings.TrimSpace(n)
			}
		case float64:
			return strconv.FormatInt(int64(n), 10)
		}
	}
	if u, ok := ref["url"].(string); ok && strings.TrimSpace(u) != "" {
		if parsed, perr := url.Parse(strings.TrimSpace(u)); perr == nil {
			return strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
		}
	}
	return ""
}

func metadataString(metadata map[string]any, key string) string {
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return ""
}

func firstNonEmptyStringPtr(ptr *string, rest ...string) string {
	if ptr != nil && strings.TrimSpace(*ptr) != "" {
		return *ptr
	}
	for _, value := range rest {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func jsonString(value string) json.RawMessage {
	out, _ := json.Marshal(value)
	return out
}

func ptrToIntegrationSyncEventResponse(value IntegrationSyncEventResponse) *IntegrationSyncEventResponse {
	return &value
}

func (h *Handler) recordIntegrationInternalEvent(r *http.Request, workspaceID pgtype.UUID, connectionID pgtype.UUID, provider string, objectType string, objectID string, message string, projectID pgtype.UUID) {
	if _, err := h.Queries.CreateIntegrationSyncEvent(r.Context(), db.CreateIntegrationSyncEventParams{
		WorkspaceID:  workspaceID,
		ConnectionID: connectionID,
		Provider:     provider,
		Direction:    "internal",
		ObjectType:   objectType,
		ObjectID:     ptrToText(&objectID),
		ProjectID:    projectID,
		Status:       "success",
		Message:      ptrToText(&message),
		Metadata:     []byte("{}"),
	}); err != nil {
		slog.Warn("integration sync event write failed",
			"workspace_id", uuidToString(workspaceID),
			"connection_id", uuidToString(connectionID),
			"provider", provider,
			"object_type", objectType,
			"error", err,
		)
	}
}

func integrationConnectionPath(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	connUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "connectionId"), "integration connection id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	return wsUUID, connUUID, true
}

func normalizeIntegrationConnectionRequest(w http.ResponseWriter, req UpsertIntegrationConnectionRequest, creating bool) (string, string, *string, []byte, string, bool, bool) {
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if creating {
		if _, ok := integrationProviders[provider]; !ok {
			writeError(w, http.StatusBadRequest, "provider must be gitlab, zentao, or feishu")
			return "", "", nil, nil, "", false, false
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" && creating {
		name = provider
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", "", nil, nil, "", false, false
	}
	baseURL := trimStringPtr(req.BaseURL)
	if baseURL != nil {
		// Normalize a hand-entered base URL: drop a trailing slash and an
		// accidental /api.php(/v1) suffix (the ZenTao client appends that itself).
		// Prevents the most common misconfig that surfaces as a 404.
		b := strings.TrimRight(strings.TrimSpace(*baseURL), "/")
		b = strings.TrimSuffix(b, "/api.php/v1")
		b = strings.TrimSuffix(b, "/api.php")
		b = strings.TrimRight(b, "/")
		baseURL = &b
	}
	config, ok := normalizeJSONObject(w, req.Config, "config")
	if !ok {
		return "", "", nil, nil, "", false, false
	}
	status := "active"
	if req.Status != nil {
		status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	if status != "active" && status != "disabled" {
		writeError(w, http.StatusBadRequest, "status must be active or disabled")
		return "", "", nil, nil, "", false, false
	}
	syncEnabled := false
	if req.SyncEnabled != nil {
		syncEnabled = *req.SyncEnabled
	}
	return provider, name, baseURL, config, status, syncEnabled, true
}

func normalizeJSONObject(w http.ResponseWriter, raw json.RawMessage, fieldName string) ([]byte, bool) {
	if len(raw) == 0 {
		return []byte("{}"), true
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		writeError(w, http.StatusBadRequest, fieldName+" must be valid JSON")
		return nil, false
	}
	if _, ok := value.(map[string]any); !ok {
		writeError(w, http.StatusBadRequest, fieldName+" must be a JSON object")
		return nil, false
	}
	out, err := json.Marshal(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to normalize "+fieldName)
		return nil, false
	}
	return out, true
}

func normalizeIntegrationAccountRequest(w http.ResponseWriter, req UpsertIntegrationUserAccountRequest) (string, string, []byte, string, pgtype.Timestamptz, bool) {
	name := ""
	if req.AccountName != nil {
		name = strings.TrimSpace(*req.AccountName)
	}
	if name == "" && req.ExternalUsername != nil {
		name = strings.TrimSpace(*req.ExternalUsername)
	}
	if name == "" && req.ExternalUserID != nil {
		name = strings.TrimSpace(*req.ExternalUserID)
	}
	if name == "" {
		name = "Default"
	}

	keySource := name
	if req.AccountKey != nil && strings.TrimSpace(*req.AccountKey) != "" {
		keySource = *req.AccountKey
	}
	key := normalizeAccountKey(keySource, "default")

	scopes, err := json.Marshal(normalizeScopes(req.Scopes))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to normalize account scopes")
		return "", "", nil, "", pgtype.Timestamptz{}, false
	}

	status := "active"
	if req.Status != nil {
		status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	if status != "active" && status != "disabled" && status != "error" {
		writeError(w, http.StatusBadRequest, "account status must be active, disabled, or error")
		return "", "", nil, "", pgtype.Timestamptz{}, false
	}

	expiresAt := pgtype.Timestamptz{}
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" {
		parsed, err := parseIntegrationAccountTime(strings.TrimSpace(*req.ExpiresAt))
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_at must be an RFC3339 timestamp or YYYY-MM-DD date")
			return "", "", nil, "", pgtype.Timestamptz{}, false
		}
		expiresAt = pgtype.Timestamptz{Time: parsed, Valid: true}
	}

	return key, name, scopes, status, expiresAt, true
}

func normalizeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeAccountKey(value string, fallback string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	key := strings.Trim(b.String(), "-")
	if key == "" {
		return fallback
	}
	return key
}

func parseIntegrationAccountTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func stringSliceFromJSON(raw []byte) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return []string{}
	}
	return normalizeScopes(values)
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func integrationConnectionToResponse(row db.IntegrationConnection) IntegrationConnectionResponse {
	return IntegrationConnectionResponse{
		ID:              uuidToString(row.ID),
		WorkspaceID:     uuidToString(row.WorkspaceID),
		Provider:        row.Provider,
		Name:            row.Name,
		BaseURL:         textToPtr(row.BaseUrl),
		Config:          maskConnectionConfig(row.Config),
		Status:          row.Status,
		SyncEnabled:     row.SyncEnabledAt.Valid,
		CreatedByUserID: uuidToPtr(row.CreatedByUserID),
		CreatedAt:       timestampToString(row.CreatedAt),
		UpdatedAt:       timestampToString(row.UpdatedAt),
	}
}

func integrationUserAccountToResponse(row db.IntegrationUserAccount) IntegrationUserAccountResponse {
	return IntegrationUserAccountResponse{
		ID:                   uuidToString(row.ID),
		WorkspaceID:          uuidToString(row.WorkspaceID),
		ConnectionID:         uuidToString(row.ConnectionID),
		UserID:               uuidToString(row.UserID),
		AccountKey:           row.AccountKey,
		AccountName:          row.AccountName,
		ExternalUserID:       textToPtr(row.ExternalUserID),
		ExternalUsername:     textToPtr(row.ExternalUsername),
		CredentialConfigured: len(row.CredentialEncrypted) > 0,
		Scopes:               stringSliceFromJSON(row.Scopes),
		Config:               jsonObjectOrEmpty(row.Config),
		Status:               row.Status,
		SyncEnabled:          row.SyncEnabledAt.Valid,
		ExpiresAt:            timestampToPtr(row.ExpiresAt),
		LastUsedAt:           timestampToPtr(row.LastUsedAt),
		LastError:            textToPtr(row.LastError),
		CreatedAt:            timestampToString(row.CreatedAt),
		UpdatedAt:            timestampToString(row.UpdatedAt),
	}
}

func integrationProjectBindingToResponse(row db.IntegrationProjectBinding) IntegrationProjectBindingResponse {
	return IntegrationProjectBindingResponse{
		ID:                   uuidToString(row.ID),
		WorkspaceID:          uuidToString(row.WorkspaceID),
		ProjectID:            uuidToString(row.ProjectID),
		ConnectionID:         uuidToString(row.ConnectionID),
		ExternalRef:          jsonObjectOrEmpty(row.ExternalRef),
		InboundEnabled:       row.InboundEnabledAt.Valid,
		OutboundEnabled:      row.OutboundEnabledAt.Valid,
		IssueSyncEnabled:     row.IssueSyncEnabledAt.Valid,
		KnowledgeSyncEnabled: row.KnowledgeSyncEnabledAt.Valid,
		CreatedByUserID:      uuidToPtr(row.CreatedByUserID),
		CreatedAt:            timestampToString(row.CreatedAt),
		UpdatedAt:            timestampToString(row.UpdatedAt),
	}
}

func integrationIssueSyncSettingToResponse(row db.IntegrationIssueSyncSetting) IntegrationIssueSyncSettingResponse {
	return IntegrationIssueSyncSettingResponse{
		WorkspaceID:     uuidToString(row.WorkspaceID),
		UserID:          uuidToString(row.UserID),
		Provider:        row.Provider,
		InboundEnabled:  row.InboundEnabledAt.Valid,
		OutboundEnabled: row.OutboundEnabledAt.Valid,
		CreatedAt:       timestampToString(row.CreatedAt),
		UpdatedAt:       timestampToString(row.UpdatedAt),
	}
}

func integrationSyncEventToResponse(row db.IntegrationSyncEvent) IntegrationSyncEventResponse {
	return IntegrationSyncEventResponse{
		ID:           uuidToString(row.ID),
		WorkspaceID:  uuidToString(row.WorkspaceID),
		ConnectionID: uuidToPtr(row.ConnectionID),
		Provider:     row.Provider,
		Direction:    row.Direction,
		ObjectType:   row.ObjectType,
		ObjectID:     textToPtr(row.ObjectID),
		ExternalID:   textToPtr(row.ExternalID),
		ExternalURL:  textToPtr(row.ExternalUrl),
		ProjectID:    uuidToPtr(row.ProjectID),
		Status:       row.Status,
		Message:      textToPtr(row.Message),
		Error:        textToPtr(row.Error),
		Metadata:     jsonObjectOrEmpty(row.Metadata),
		OccurredAt:   timestampToString(row.OccurredAt),
		CreatedAt:    timestampToString(row.CreatedAt),
	}
}

// latestSyncEventPerProvider returns the most recent sync event for each provider
// in a workspace (one row per provider). Used to keep a provider card's summary
// accurate even when the global recent-event window is saturated by other,
// higher-frequency providers (see ListIntegrations / Defect 1).
func (h *Handler) latestSyncEventPerProvider(ctx context.Context, workspaceID pgtype.UUID) ([]db.IntegrationSyncEvent, error) {
	const q = `
SELECT DISTINCT ON (provider)
  id, workspace_id, connection_id, provider, direction, object_type, object_id,
  external_id, external_url, project_id, status, message, error, metadata,
  occurred_at, created_at
FROM integration_sync_event
WHERE workspace_id = $1
ORDER BY provider, occurred_at DESC, created_at DESC`
	rows, err := h.DB.Query(ctx, q, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.IntegrationSyncEvent
	for rows.Next() {
		var e db.IntegrationSyncEvent
		if err := rows.Scan(
			&e.ID, &e.WorkspaceID, &e.ConnectionID, &e.Provider, &e.Direction,
			&e.ObjectType, &e.ObjectID, &e.ExternalID, &e.ExternalUrl, &e.ProjectID,
			&e.Status, &e.Message, &e.Error, &e.Metadata, &e.OccurredAt, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// sealConnectionConfig moves a plaintext "app_secret" in a connection config into
// an encrypted "app_secret_enc" (base64 of secretbox ciphertext). When the admin
// saves without re-entering the secret, the existing encrypted value is carried
// over from existingConfig so it isn't lost. App-level secrets live ONLY here
// (connection level), never in a per-user account.
func (h *Handler) sealConnectionConfig(newConfig, existingConfig []byte) ([]byte, error) {
	m := map[string]any{}
	if len(newConfig) > 0 {
		if err := json.Unmarshal(newConfig, &m); err != nil {
			return newConfig, err
		}
	}
	secret, _ := m["app_secret"].(string)
	delete(m, "app_secret")
	secret = strings.TrimSpace(secret)
	if secret != "" {
		if h.IntegrationSecrets == nil {
			return nil, fmt.Errorf("credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)")
		}
		sealed, err := h.IntegrationSecrets.Seal([]byte(secret))
		if err != nil {
			return nil, err
		}
		m["app_secret_enc"] = base64.StdEncoding.EncodeToString(sealed)
	} else if _, has := m["app_secret_enc"]; !has && len(existingConfig) > 0 {
		var old map[string]any
		if json.Unmarshal(existingConfig, &old) == nil {
			if enc, ok := old["app_secret_enc"].(string); ok && enc != "" {
				m["app_secret_enc"] = enc
			}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return newConfig, err
	}
	return out, nil
}

// connectionAppSecretFromConfig decrypts the connection-level app_secret.
func (h *Handler) connectionAppSecretFromConfig(config []byte) (string, error) {
	enc := jsonStringField(config, "app_secret_enc")
	if enc == "" {
		return "", fmt.Errorf("no app_secret configured for this connection")
	}
	if h.IntegrationSecrets == nil {
		return "", fmt.Errorf("credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)")
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	plain, err := h.IntegrationSecrets.Open(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// maskConnectionConfig strips the encrypted app_secret from a config before it is
// returned to clients, replacing it with a boolean so the UI can show "configured".
func maskConnectionConfig(config []byte) json.RawMessage {
	obj := jsonObjectOrEmpty(config)
	var m map[string]any
	if json.Unmarshal(obj, &m) != nil {
		return obj
	}
	if _, ok := m["app_secret_enc"]; ok {
		delete(m, "app_secret_enc")
		m["app_secret_configured"] = true
	}
	delete(m, "app_secret")
	out, err := json.Marshal(m)
	if err != nil {
		return obj
	}
	return out
}

func jsonObjectOrEmpty(raw []byte) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(raw)
}
