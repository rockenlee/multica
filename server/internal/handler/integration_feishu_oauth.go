package handler

import (
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/feishu"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// feishuOAuthAccountKey is the account_key under which a user's Feishu OAuth
// refresh token is stored (distinct from the app_secret account used for
// outbound). The inbound worker only mints user tokens from this account.
const feishuOAuthAccountKey = "feishu_user_oauth"

// feishuOAuthScope is requested at authorize time. task:task grants read access
// to the user's personal tasks; offline_access is REQUIRED for Feishu to return
// a refresh_token (without it the exchange yields only a 2h access_token).
const feishuOAuthScope = "task:task offline_access"

// feishuOAuthRedirectURI is where Feishu sends the browser back with the code.
// It must be whitelisted in the Feishu app console (Security Settings → Redirect
// URL). Configurable so non-localhost deployments can override it.
func feishuOAuthRedirectURI() string {
	base := strings.TrimSpace(os.Getenv("MULTICA_INTEGRATION_OAUTH_BASE"))
	if base == "" {
		base = "http://localhost:8080"
	}
	return strings.TrimRight(base, "/") + "/api/integrations/feishu/oauth/callback"
}

type feishuOAuthState struct {
	WS   string `json:"ws"`
	Conn string `json:"conn"`
	User string `json:"user"`
}

func encodeFeishuState(s feishuOAuthState) string {
	b, _ := json.Marshal(s)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeFeishuState(raw string) (feishuOAuthState, error) {
	var s feishuOAuthState
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// FeishuOAuthStart (authed, member-level) returns the Feishu authorize URL the
// user opens to grant the app access to their personal tasks. State carries the
// workspace/connection/user so the public callback can attribute the token.
func (h *Handler) FeishuOAuthStart(w http.ResponseWriter, r *http.Request) {
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
	conn, err := h.Queries.GetIntegrationConnectionInWorkspace(r.Context(), db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "integration connection not found")
		return
	}
	if conn.Provider != "feishu" {
		writeError(w, http.StatusBadRequest, "connection is not a feishu connection")
		return
	}
	appID := jsonStringField(conn.Config, "app_id")
	if appID == "" {
		writeError(w, http.StatusBadRequest, "feishu connection is missing app_id in config")
		return
	}
	if _, err := h.connectionAppSecretFromConfig(conn.Config); err != nil {
		writeError(w, http.StatusBadRequest, "this Feishu connection has no app secret configured yet; ask a workspace admin to set it in integration settings")
		return
	}
	state := encodeFeishuState(feishuOAuthState{
		WS:   uuidToString(wsUUID),
		Conn: uuidToString(connUUID),
		User: uuidToString(userUUID),
	})
	authorize := "https://accounts.feishu.cn/open-apis/authen/v1/authorize?" + url.Values{
		"client_id":    {appID},
		"redirect_uri": {feishuOAuthRedirectURI()},
		"scope":        {feishuOAuthScope},
		"state":        {state},
	}.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"authorize_url": authorize})
}

// FeishuOAuthCallback (public — the browser redirect target) exchanges the code
// for a refresh token and stores it so the inbound worker can mint user tokens.
func (h *Handler) FeishuOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.IntegrationSecrets == nil {
		feishuOAuthHTML(w, false, "credential storage disabled (set MULTICA_INTEGRATION_SECRET_KEY)")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	rawState := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || rawState == "" {
		feishuOAuthHTML(w, false, "missing code or state")
		return
	}
	st, err := decodeFeishuState(rawState)
	if err != nil {
		feishuOAuthHTML(w, false, "invalid state")
		return
	}
	wsUUID, err1 := parseUUIDLoose(st.WS)
	connUUID, err2 := parseUUIDLoose(st.Conn)
	userUUID, err3 := parseUUIDLoose(st.User)
	if err1 != nil || err2 != nil || err3 != nil {
		feishuOAuthHTML(w, false, "invalid state ids")
		return
	}
	ctx := r.Context()
	conn, err := h.Queries.GetIntegrationConnectionInWorkspace(ctx, db.GetIntegrationConnectionInWorkspaceParams{
		ID:          connUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		feishuOAuthHTML(w, false, "connection not found")
		return
	}
	appID := jsonStringField(conn.Config, "app_id")
	appSecret, err := h.connectionAppSecretFromConfig(conn.Config)
	if err != nil {
		feishuOAuthHTML(w, false, "this Feishu connection has no app secret configured (ask a workspace admin)")
		return
	}
	baseURL := ""
	if conn.BaseUrl.Valid {
		baseURL = conn.BaseUrl.String
	}
	tok, err := feishu.New(baseURL, appID, appSecret, nil).ExchangeCode(ctx, appID, appSecret, code, feishuOAuthRedirectURI())
	if err != nil {
		feishuOAuthHTML(w, false, "token exchange failed: "+err.Error())
		return
	}
	// Store the per-member OAuth grant + the current access token and its expiry,
	// so the worker reuses the ~2h access token and only refreshes (rotating the
	// single-use refresh token) when it's near expiry. The app_secret lives at the
	// connection level (admin-configured), not in any per-user account.
	blob, _ := json.Marshal(map[string]any{
		"refresh_token":     tok.RefreshToken,
		"access_token":      tok.AccessToken,
		"access_expires_at": time.Now().Unix() + int64(tok.ExpiresIn),
	})
	sealed, err := h.IntegrationSecrets.Seal(blob)
	if err != nil {
		feishuOAuthHTML(w, false, "failed to encrypt token")
		return
	}
	var expiresAt pgtype.Timestamptz
	if tok.RefreshTokenExpiresIn > 0 {
		expiresAt = pgtype.Timestamptz{Time: time.Now().Add(time.Duration(tok.RefreshTokenExpiresIn) * time.Second), Valid: true}
	}
	if _, err := h.Queries.UpsertIntegrationUserAccount(ctx, db.UpsertIntegrationUserAccountParams{
		WorkspaceID:         wsUUID,
		ConnectionID:        connUUID,
		UserID:              userUUID,
		AccountKey:          feishuOAuthAccountKey,
		AccountName:         "Feishu personal tasks (OAuth)",
		CredentialEncrypted: sealed,
		Scopes:              []byte(`["` + feishuOAuthScope + `"]`),
		Config:              []byte("{}"),
		Status:              "active",
		SyncEnabled:         true,
		ExpiresAt:           expiresAt,
	}); err != nil {
		feishuOAuthHTML(w, false, "failed to store token")
		return
	}
	feishuOAuthHTML(w, true, "")
}

// feishuAppSecret returns the user's stored Feishu app_secret (the non-OAuth
// account credential) for a connection, or a user-facing error message.
func feishuOAuthHTML(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if ok {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Feishu connected</title>` +
			`<body style="font-family:system-ui,sans-serif;text-align:center;padding:48px">` +
			`<h2>✅ 飞书授权成功</h2><p>你的飞书个人任务将自动同步到 Multica，可以关闭此页面。</p></body>`))
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Feishu auth failed</title>` +
		`<body style="font-family:system-ui,sans-serif;text-align:center;padding:48px">` +
		`<h2>❌ 飞书授权失败</h2><p>` + html.EscapeString(msg) + `</p></body>`))
}
