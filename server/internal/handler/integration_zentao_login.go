package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/zentao"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// zentaoCorrectBaseURL appends the conventional /zentao path when it's missing,
// the usual cause of a 404 on api.php/v1. Already-correct URLs are returned as-is.
func zentaoCorrectBaseURL(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if b == "" || strings.HasSuffix(b, "/zentao") {
		return b
	}
	return b + "/zentao"
}

type zentaoLoginRequest struct {
	Account     string `json:"account"`
	Password    string `json:"password"`
	AccountKey  string `json:"account_key"`
	AccountName string `json:"account_name"`
}

// ZenTaoLogin (authed, member-level) exchanges a ZenTao account+password for a
// session token and stores it together with the credentials (all encrypted) so
// the inbound worker can silently re-login when the ~24h token expires — the
// user connects once instead of re-pasting a token every day.
func (h *Handler) ZenTaoLogin(w http.ResponseWriter, r *http.Request) {
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
	var req zentaoLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "account and password are required")
		return
	}
	if h.IntegrationSecrets == nil {
		writeError(w, http.StatusBadRequest, "credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)")
		return
	}
	conn, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "integration connection not found")
		return
	}
	if conn.Provider != "zentao" {
		writeError(w, http.StatusBadRequest, "connection is not a zentao connection")
		return
	}
	baseURL := ""
	if conn.BaseUrl.Valid {
		baseURL = conn.BaseUrl.String
	}
	token, err := zentao.New(baseURL, "", nil).Login(r.Context(), req.Account, req.Password)
	if err != nil && strings.Contains(err.Error(), "404") {
		// Common misconfig: the connection base_url omits the ZenTao path (the
		// instance lives at /zentao). Auto-correct, retry once, and persist the
		// fixed base_url so the worker/outbound use it too.
		if corrected := zentaoCorrectBaseURL(baseURL); corrected != strings.TrimRight(baseURL, "/") {
			if tok2, err2 := zentao.New(corrected, "", nil).Login(r.Context(), req.Account, req.Password); err2 == nil {
				token, err = tok2, nil
				_, _ = h.DB.Exec(r.Context(), `UPDATE integration_connection SET base_url=$1, updated_at=now() WHERE id=$2`, corrected, connUUID)
			}
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	blob, _ := json.Marshal(zentaoCredential{Token: token, Account: req.Account, Password: req.Password})
	sealed, err := h.IntegrationSecrets.Seal(blob)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt credential")
		return
	}
	accountKey := strings.TrimSpace(req.AccountKey)
	if accountKey == "" {
		accountKey = "default"
	}
	accountName := strings.TrimSpace(req.AccountName)
	if accountName == "" {
		accountName = req.Account
	}
	if _, err := h.Queries.UpsertIntegrationUserAccount(r.Context(), db.UpsertIntegrationUserAccountParams{
		WorkspaceID:         wsUUID,
		ConnectionID:        connUUID,
		UserID:              userUUID,
		AccountKey:          accountKey,
		AccountName:         accountName,
		ExternalUsername:    pgtype.Text{String: req.Account, Valid: true},
		CredentialEncrypted: sealed,
		Scopes:              []byte("[]"),
		Config:              []byte("{}"),
		Status:              "active",
		SyncEnabled:         true,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store credential")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "account_key": accountKey})
}
