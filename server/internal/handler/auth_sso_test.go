package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// newFakeIdP starts an OIDC provider stub serving discovery, token, and
// userinfo endpoints. userinfo is returned verbatim as provided.
func newFakeIdP(t *testing.T, userinfo map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"issuer": %q,
			"authorization_endpoint": %q,
			"token_endpoint": %q,
			"userinfo_endpoint": %q
		}`, srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/userinfo")
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at-test-123","token_type":"Bearer"}`)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-test-123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(userinfo)
	})
	return srv
}

func setOIDCEnv(t *testing.T, issuer string) {
	t.Helper()
	t.Setenv("MULTICA_OIDC_ISSUER", issuer)
	t.Setenv("MULTICA_OIDC_CLIENT_ID", "multica-client")
	t.Setenv("MULTICA_OIDC_CLIENT_SECRET", "multica-secret")
	t.Setenv("MULTICA_OIDC_REDIRECT_URI", "https://app.example.com/auth/callback")
}

func TestGetConfigExposesSSO(t *testing.T) {
	h := &Handler{}

	fetch := func() map[string]json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		w := httptest.NewRecorder()
		h.GetConfig(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GetConfig: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
			t.Fatalf("decode config: %v", err)
		}
		return cfg
	}

	// Unconfigured: both fields must be omitted to keep the previous shape.
	t.Setenv("MULTICA_OIDC_ISSUER", "")
	t.Setenv("MULTICA_OIDC_CLIENT_ID", "")
	t.Setenv("MULTICA_OIDC_CLIENT_SECRET", "")
	cfg := fetch()
	if _, ok := cfg["sso_enabled"]; ok {
		t.Fatal("sso_enabled must be omitted when OIDC is unconfigured")
	}
	if _, ok := cfg["sso_display_name"]; ok {
		t.Fatal("sso_display_name must be omitted when OIDC is unconfigured")
	}

	// Configured: enabled with the default display name.
	setOIDCEnv(t, "https://idp.example.com")
	cfg = fetch()
	if string(cfg["sso_enabled"]) != "true" {
		t.Fatalf("sso_enabled: want true, got %s", cfg["sso_enabled"])
	}
	if string(cfg["sso_display_name"]) != `"SSO"` {
		t.Fatalf("sso_display_name: want default SSO, got %s", cfg["sso_display_name"])
	}

	// Operator-supplied display name wins.
	t.Setenv("MULTICA_OIDC_DISPLAY_NAME", "HBC SSO")
	cfg = fetch()
	if string(cfg["sso_display_name"]) != `"HBC SSO"` {
		t.Fatalf("sso_display_name: want HBC SSO, got %s", cfg["sso_display_name"])
	}
}

func TestSSOLoginRedirect(t *testing.T) {
	h := &Handler{}

	t.Run("unconfigured", func(t *testing.T) {
		t.Setenv("MULTICA_OIDC_ISSUER", "")
		req := httptest.NewRequest(http.MethodGet, "/auth/sso/login", nil)
		w := httptest.NewRecorder()
		h.SSOLoginRedirect(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 when unconfigured, got %d", w.Code)
		}
	})

	t.Run("redirects_to_idp", func(t *testing.T) {
		idp := newFakeIdP(t, map[string]any{})
		setOIDCEnv(t, idp.URL)

		state := "platform:desktop,next:/inbox"
		req := httptest.NewRequest(http.MethodGet, "/auth/sso/login?state="+url.QueryEscape(state), nil)
		w := httptest.NewRecorder()
		h.SSOLoginRedirect(w, req)
		if w.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d: %s", w.Code, w.Body.String())
		}

		loc, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatalf("parse redirect location: %v", err)
		}
		if !strings.HasPrefix(loc.String(), idp.URL+"/authorize") {
			t.Fatalf("redirect target: want %s/authorize..., got %s", idp.URL, loc)
		}
		q := loc.Query()
		if q.Get("response_type") != "code" {
			t.Fatalf("response_type: want code, got %q", q.Get("response_type"))
		}
		if q.Get("client_id") != "multica-client" {
			t.Fatalf("client_id: want multica-client, got %q", q.Get("client_id"))
		}
		if q.Get("redirect_uri") != "https://app.example.com/auth/callback" {
			t.Fatalf("redirect_uri: got %q", q.Get("redirect_uri"))
		}
		if q.Get("scope") != defaultOIDCScopes {
			t.Fatalf("scope: want %q, got %q", defaultOIDCScopes, q.Get("scope"))
		}
		if q.Get("state") != state {
			t.Fatalf("state: want round-tripped %q, got %q", state, q.Get("state"))
		}
	})

	t.Run("rejects_mismatched_discovery_issuer", func(t *testing.T) {
		// A discovery document claiming a different issuer must be refused.
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"issuer":"https://evil.example.com","authorization_endpoint":"x","token_endpoint":"x","userinfo_endpoint":"x"}`)
		})
		setOIDCEnv(t, srv.URL)

		req := httptest.NewRequest(http.MethodGet, "/auth/sso/login", nil)
		w := httptest.NewRecorder()
		h.SSOLoginRedirect(w, req)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("expected 502 for issuer mismatch, got %d", w.Code)
		}
	})
}

func postSSOLogin(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/sso", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SSOLogin(w, req)
	return w
}

func TestSSOLogin(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		t.Setenv("MULTICA_OIDC_ISSUER", "")
		w := postSSOLogin(t, &Handler{}, `{"code":"abc"}`)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 when unconfigured, got %d", w.Code)
		}
	})

	t.Run("missing_code", func(t *testing.T) {
		w := postSSOLogin(t, &Handler{}, `{}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 without code, got %d", w.Code)
		}
	})

	// Full exchange against the stub IdP, ending at the shared signup gate:
	// proves discovery, token exchange, and userinfo parsing all work without
	// needing a real database.
	t.Run("full_exchange_hits_signup_gate", func(t *testing.T) {
		idp := newFakeIdP(t, map[string]any{
			"sub":            "user-1",
			"email":          "new@blocked.com",
			"email_verified": true,
			"name":           "New User",
		})
		setOIDCEnv(t, idp.URL)

		h := newTestHandler(Config{AllowSignup: false})
		h.Queries = db.New(&mockDB{getUserErr: pgx.ErrNoRows})

		w := postSSOLogin(t, h, `{"code":"good-code"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 from signup gate, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "registration is disabled") {
			t.Fatalf("expected signup-disabled error, got %s", w.Body.String())
		}
	})

	t.Run("rejects_unverified_email", func(t *testing.T) {
		idp := newFakeIdP(t, map[string]any{
			"email":          "user@company.com",
			"email_verified": false,
		})
		setOIDCEnv(t, idp.URL)

		w := postSSOLogin(t, &Handler{}, `{"code":"good-code"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for unverified email, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects_missing_email", func(t *testing.T) {
		idp := newFakeIdP(t, map[string]any{"sub": "user-1"})
		setOIDCEnv(t, idp.URL)

		w := postSSOLogin(t, &Handler{}, `{"code":"good-code"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing email claim, got %d: %s", w.Code, w.Body.String())
		}
	})
}
