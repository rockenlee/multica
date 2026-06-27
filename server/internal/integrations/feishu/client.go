// Package feishu is a minimal Feishu (Lark) Open API client for the integration
// module: creating tasks for outbound issue sync and listing Drive folder files
// for resource content fetch. It authenticates with a tenant_access_token minted
// from the app's app_id + app_secret.
package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://open.feishu.cn"
	defaultTimeout = 15 * time.Second
)

// Client talks to the Feishu Open API using an internal app's credentials.
type Client struct {
	BaseURL    string
	AppID      string
	AppSecret  string
	HTTPClient *http.Client
}

// New builds a client. baseURL defaults to https://open.feishu.cn.
func New(baseURL, appID, appSecret string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	// Feishu Open APIs are HTTPS-only; an http:// base URL breaks the OAuth token
	// POST (the redirect drops the body). Upgrade it transparently.
	if strings.HasPrefix(baseURL, "http://") {
		baseURL = "https://" + strings.TrimPrefix(baseURL, "http://")
	}
	return &Client{BaseURL: baseURL, AppID: appID, AppSecret: appSecret, HTTPClient: hc}
}

func (c *Client) tenantToken(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{"app_id": c.AppID, "app_secret": c.AppSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu: token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Code  int    `json:"code"`
		Msg   string `json:"msg"`
		Token string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("feishu: decode token: %w", err)
	}
	if out.Code != 0 || out.Token == "" {
		return "", fmt.Errorf("feishu: tenant token error code %d: %s", out.Code, out.Msg)
	}
	return out.Token, nil
}

// CreateTask creates a Feishu task and returns its guid and applink URL.
func (c *Client) CreateTask(ctx context.Context, summary, description string) (guid, taskURL string, err error) {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return "", "", err
	}
	body, _ := json.Marshal(map[string]string{"summary": summary, "description": description})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/open-apis/task/v2/tasks", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("feishu: create task failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Task struct {
				GUID string `json:"guid"`
				URL  string `json:"url"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("feishu: decode task: %w", err)
	}
	if out.Code != 0 {
		return "", "", fmt.Errorf("feishu: create task code %d: %s", out.Code, out.Msg)
	}
	return out.Data.Task.GUID, out.Data.Task.URL, nil
}

// UserToken is the result of an OAuth code exchange or refresh.
type UserToken struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
}

// oauthToken posts to the v2 OAuth token endpoint (shared by code-exchange and
// refresh). appID/appSecret authenticate the app; extra carries the grant-specific
// fields (code+redirect_uri, or refresh_token).
func (c *Client) oauthToken(ctx context.Context, appID, appSecret, grantType string, extra map[string]string) (UserToken, error) {
	payload := map[string]string{
		"grant_type":    grantType,
		"client_id":     appID,
		"client_secret": appSecret,
	}
	for k, v := range extra {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/open-apis/authen/v2/oauth/token", bytes.NewReader(body))
	if err != nil {
		return UserToken{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return UserToken{}, fmt.Errorf("feishu: oauth token request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		UserToken
		Code             int    `json:"code"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return UserToken{}, fmt.Errorf("feishu: decode oauth token: %w", err)
	}
	if out.Error != "" || out.AccessToken == "" {
		return UserToken{}, fmt.Errorf("feishu: oauth token error %s: %s", out.Error, out.ErrorDescription)
	}
	return out.UserToken, nil
}

// ExchangeCode swaps an authorization code for a user_access_token + refresh_token.
func (c *Client) ExchangeCode(ctx context.Context, appID, appSecret, code, redirectURI string) (UserToken, error) {
	return c.oauthToken(ctx, appID, appSecret, "authorization_code", map[string]string{
		"code":         code,
		"redirect_uri": redirectURI,
	})
}

// RefreshUserToken exchanges a refresh_token for a fresh user_access_token (and a
// possibly-rotated refresh_token).
func (c *Client) RefreshUserToken(ctx context.Context, appID, appSecret, refreshToken string) (UserToken, error) {
	return c.oauthToken(ctx, appID, appSecret, "refresh_token", map[string]string{
		"refresh_token": refreshToken,
	})
}

// Task is a listed Feishu task used for inbound mirroring.
type Task struct {
	GUID    string `json:"guid"`
	Summary string `json:"summary"`
	URL     string `json:"url"`
}

// ListTasksWithUserToken lists the tasks visible to a user, using that user's
// user_access_token (personal tasks are NOT visible to a tenant token). The
// caller supplies the token because minting it requires a user OAuth grant.
func (c *Client) ListTasksWithUserToken(ctx context.Context, userAccessToken string) ([]Task, error) {
	endpoint := c.BaseURL + "/open-apis/task/v2/tasks?page_size=50"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feishu: list tasks failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []Task `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("feishu: decode tasks: %w", err)
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("feishu: list tasks code %d: %s", out.Code, out.Msg)
	}
	return out.Data.Items, nil
}

// CompleteTask marks a task complete (done=true) or incomplete (done=false) using
// the user's user_access_token. Feishu has no in-progress state — completion is
// binary, so any non-done Multica status maps to incomplete.
func (c *Client) CompleteTask(ctx context.Context, userAccessToken, guid string, done bool) error {
	completedAt := "0"
	if done {
		completedAt = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	body, _ := json.Marshal(map[string]any{
		"task":          map[string]any{"completed_at": completedAt},
		"update_fields": []string{"completed_at"},
	})
	endpoint := c.BaseURL + "/open-apis/task/v2/tasks/" + url.PathEscape(guid)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("feishu: complete task failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("feishu: decode complete task: %w", err)
	}
	if out.Code != 0 {
		return fmt.Errorf("feishu: complete task code %d: %s", out.Code, out.Msg)
	}
	return nil
}

// CreateTaskComment adds a comment to a task using the user's user_access_token.
// Used by controlled outbound sync when a comment is added to a mirrored issue.
func (c *Client) CreateTaskComment(ctx context.Context, userAccessToken, guid, content string) error {
	body, _ := json.Marshal(map[string]any{
		"resource_type": "task",
		"resource_id":   guid,
		"content":       content,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/open-apis/task/v2/comments", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("feishu: create comment failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("feishu: decode comment: %w", err)
	}
	if out.Code != 0 {
		return fmt.Errorf("feishu: create comment code %d: %s", out.Code, out.Msg)
	}
	return nil
}

// ListDriveFiles lists the names of files directly under a Drive folder. The app
// must have access to the folder (otherwise Feishu returns a permission error).
func (c *Client) ListDriveFiles(ctx context.Context, folderToken string) ([]string, error) {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := c.BaseURL + "/open-apis/drive/v1/files?folder_token=" + url.QueryEscape(folderToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feishu: list drive files failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Files []struct {
				Name  string `json:"name"`
				Token string `json:"token"`
				Type  string `json:"type"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("feishu: decode drive files: %w", err)
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("feishu: list drive files code %d: %s", out.Code, out.Msg)
	}
	files := make([]string, 0, len(out.Data.Files))
	for _, f := range out.Data.Files {
		files = append(files, fmt.Sprintf("%s (%s)", f.Name, f.Type))
	}
	return files, nil
}
