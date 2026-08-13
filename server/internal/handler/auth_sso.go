package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Generic OIDC single sign-on. Mirrors the Google OAuth login flow: the
// browser is redirected to the IdP's authorization endpoint, returns to the
// frontend /auth/callback page with ?code=&state=, and the page posts the
// code to POST /auth/sso, which exchanges it server-side and issues the same
// JWT + cookies as every other login path. Unlike Google, the authorization
// endpoint is not known to the frontend (it comes from OIDC discovery), so
// login starts at GET /auth/sso/login, which resolves discovery and 302s.
//
// Env vars are read at request time — like GOOGLE_CLIENT_ID — so operators
// can wire or rotate the IdP without a server restart:
//
//	MULTICA_OIDC_ISSUER        e.g. https://idp.example.com/realms/main
//	MULTICA_OIDC_CLIENT_ID
//	MULTICA_OIDC_CLIENT_SECRET
//	MULTICA_OIDC_DISPLAY_NAME  optional button label, default "SSO"
//	MULTICA_OIDC_SCOPES        optional, default "openid email profile"
//	MULTICA_OIDC_REDIRECT_URI  optional, default FRONTEND_ORIGIN + /auth/callback

const defaultOIDCScopes = "openid email profile"

// ssoStateMaxLen caps the client-supplied state we relay to the IdP. The
// state is an opaque frontend round-trip value (platform/next/cli hints),
// never interpreted server-side.
const ssoStateMaxLen = 2048

func oidcIssuer() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("MULTICA_OIDC_ISSUER")), "/")
}

func oidcConfigured() bool {
	return oidcIssuer() != "" &&
		os.Getenv("MULTICA_OIDC_CLIENT_ID") != "" &&
		os.Getenv("MULTICA_OIDC_CLIENT_SECRET") != ""
}

func oidcDisplayName() string {
	if name := strings.TrimSpace(os.Getenv("MULTICA_OIDC_DISPLAY_NAME")); name != "" {
		return name
	}
	return "SSO"
}

func oidcRedirectURI() string {
	if v := strings.TrimSpace(os.Getenv("MULTICA_OIDC_REDIRECT_URI")); v != "" {
		return v
	}
	if origin := resolveFrontendAppURL(); origin != "" {
		return origin + "/auth/callback"
	}
	return ""
}

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// fetchOIDCDiscovery resolves the provider metadata for the configured
// issuer. Fetched per login attempt rather than cached: the auth routes are
// rate-limited to a handful of requests per minute, and re-reading keeps
// issuer rotation restart-free, matching the env-at-request-time contract.
func fetchOIDCDiscovery(ctx context.Context, issuer string) (oidcDiscovery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return oidcDiscovery{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcDiscovery{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return oidcDiscovery{}, fmt.Errorf("discovery returned status %d", resp.StatusCode)
	}

	var doc oidcDiscovery
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return oidcDiscovery{}, err
	}

	// Guard against a mistyped issuer resolving to some other well-known
	// document: the metadata must claim the issuer we asked for.
	if strings.TrimRight(doc.Issuer, "/") != issuer {
		return oidcDiscovery{}, fmt.Errorf("discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.UserinfoEndpoint == "" {
		return oidcDiscovery{}, errors.New("discovery document is missing required endpoints")
	}
	return doc, nil
}

// SSOLoginRedirect starts the OIDC flow: GET /auth/sso/login?state=...
// resolves discovery and 302s the browser to the IdP's authorization
// endpoint. The state is relayed untouched and comes back to the frontend
// callback, exactly like the Google flow's client-built state.
func (h *Handler) SSOLoginRedirect(w http.ResponseWriter, r *http.Request) {
	if !oidcConfigured() {
		writeError(w, http.StatusServiceUnavailable, "SSO login is not configured")
		return
	}

	redirectURI := oidcRedirectURI()
	if redirectURI == "" {
		slog.Error("sso login misconfigured: no redirect URI (set MULTICA_OIDC_REDIRECT_URI or FRONTEND_ORIGIN)")
		writeError(w, http.StatusServiceUnavailable, "SSO login is not configured")
		return
	}

	discovery, err := fetchOIDCDiscovery(r.Context(), oidcIssuer())
	if err != nil {
		slog.Error("sso discovery failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to reach the SSO provider")
		return
	}

	state := r.URL.Query().Get("state")
	if len(state) > ssoStateMaxLen {
		writeError(w, http.StatusBadRequest, "state too long")
		return
	}

	scopes := strings.TrimSpace(os.Getenv("MULTICA_OIDC_SCOPES"))
	if scopes == "" {
		scopes = defaultOIDCScopes
	}

	authURL, err := url.Parse(discovery.AuthorizationEndpoint)
	if err != nil {
		slog.Error("sso discovery returned invalid authorization endpoint", "error", err)
		writeError(w, http.StatusBadGateway, "failed to reach the SSO provider")
		return
	}
	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", os.Getenv("MULTICA_OIDC_CLIENT_ID"))
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scopes)
	if state != "" {
		q.Set("state", state)
	}
	authURL.RawQuery = q.Encode()

	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

type SSOLoginRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

type oidcTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type oidcUserInfo struct {
	Email string `json:"email"`
	// Pointer so "claim absent" (common for enterprise IdPs) is
	// distinguishable from an explicit false, which we reject.
	EmailVerified *bool  `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// SSOLogin completes the OIDC flow: exchanges the authorization code at the
// IdP's token endpoint and reads the userinfo endpoint over the same
// authenticated backend channel, then joins the shared
// findOrCreateUser → issueJWT → SetAuthCookies path.
func (h *Handler) SSOLogin(w http.ResponseWriter, r *http.Request) {
	var req SSOLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	if !oidcConfigured() {
		writeError(w, http.StatusServiceUnavailable, "SSO login is not configured")
		return
	}

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = oidcRedirectURI()
	}

	discovery, err := fetchOIDCDiscovery(r.Context(), oidcIssuer())
	if err != nil {
		slog.Error("sso discovery failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to reach the SSO provider")
		return
	}

	// Exchange authorization code for tokens.
	tokenReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, discovery.TokenEndpoint, strings.NewReader(url.Values{
		"code":          {req.Code},
		"client_id":     {os.Getenv("MULTICA_OIDC_CLIENT_ID")},
		"client_secret": {os.Getenv("MULTICA_OIDC_CLIENT_SECRET")},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}.Encode()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		slog.Error("sso token exchange failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to exchange code with the SSO provider")
		return
	}
	defer tokenResp.Body.Close()

	tokenBody, err := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read the SSO token response")
		return
	}

	if tokenResp.StatusCode != http.StatusOK {
		slog.Error("sso token exchange returned error", "status", tokenResp.StatusCode, "body", string(tokenBody))
		writeError(w, http.StatusBadRequest, "failed to exchange code with the SSO provider")
		return
	}

	var oToken oidcTokenResponse
	if err := json.Unmarshal(tokenBody, &oToken); err != nil {
		writeError(w, http.StatusBadGateway, "failed to parse the SSO token response")
		return
	}

	// Fetch identity claims from the userinfo endpoint. The claims arrive
	// over TLS directly from the IdP using the access token we just obtained
	// with the client secret, so — as with the Google flow — no local ID
	// token signature verification is needed.
	userInfoReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, discovery.UserinfoEndpoint, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	userInfoReq.Header.Set("Authorization", "Bearer "+oToken.AccessToken)
	userInfoReq.Header.Set("Accept", "application/json")

	userInfoResp, err := http.DefaultClient.Do(userInfoReq)
	if err != nil {
		slog.Error("sso userinfo fetch failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to fetch user info from the SSO provider")
		return
	}
	defer userInfoResp.Body.Close()

	if userInfoResp.StatusCode != http.StatusOK {
		slog.Error("sso userinfo returned error", "status", userInfoResp.StatusCode)
		writeError(w, http.StatusBadGateway, "failed to fetch user info from the SSO provider")
		return
	}

	var oUser oidcUserInfo
	if err := json.NewDecoder(io.LimitReader(userInfoResp.Body, 1<<20)).Decode(&oUser); err != nil {
		writeError(w, http.StatusBadGateway, "failed to parse the SSO user info")
		return
	}

	if oUser.Email == "" {
		writeError(w, http.StatusBadRequest, "SSO account has no email; the provider must release an email claim")
		return
	}
	if oUser.EmailVerified != nil && !*oUser.EmailVerified {
		writeError(w, http.StatusForbidden, "SSO account email is not verified")
		return
	}

	email := strings.ToLower(strings.TrimSpace(oUser.Email))

	user, isNew, err := h.findOrCreateUser(r.Context(), email)
	if err != nil {
		var signupErr SignupError
		if errors.As(err, &signupErr) {
			writeError(w, http.StatusForbidden, signupErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if isNew {
		evt := analytics.Signup(uuidToString(user.ID), user.Email, signupSourceFromRequest(r))
		evt.Properties["auth_method"] = "sso"
		obsmetrics.RecordEvent(h.Analytics, h.Metrics, evt)
	}

	// Adopt profile fields from the IdP when the local ones are still the
	// creation defaults, matching the Google flow.
	needsUpdate := false
	newName := user.Name
	newAvatar := user.AvatarUrl

	if oUser.Name != "" && user.Name == strings.Split(email, "@")[0] {
		newName = oUser.Name
		needsUpdate = true
	}
	if oUser.Picture != "" && !user.AvatarUrl.Valid {
		newAvatar = pgtype.Text{String: oUser.Picture, Valid: true}
		needsUpdate = true
	}

	if needsUpdate {
		updated, err := h.Queries.UpdateUser(r.Context(), db.UpdateUserParams{
			ID:        user.ID,
			Name:      newName,
			AvatarUrl: newAvatar,
		})
		if err == nil {
			user = updated
		}
	}

	tokenString, err := h.issueJWT(user)
	if err != nil {
		slog.Warn("sso login failed", append(logger.RequestAttrs(r), "error", err, "email", email)...)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}

	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(72 * time.Hour)) {
			http.SetCookie(w, cookie)
		}
	}

	slog.Info("user logged in via sso", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID), "email", user.Email)...)
	writeJSON(w, http.StatusOK, LoginResponse{
		Token: tokenString,
		User:  h.userToResponse(user),
	})
}
