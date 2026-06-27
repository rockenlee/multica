// Package zentao is a minimal ZenTao REST (api.php/v1) client for the
// integration module: reading project context for an agent and creating work
// items for outbound issue sync. It authenticates with a Token header (the
// same token the zentao CLI uses).
package zentao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 20 * time.Second

// Client talks to a ZenTao instance's api.php/v1.
type Client struct {
	BaseURL    string // zentao root, e.g. https://host/zentao
	Token      string // Token header
	HTTPClient *http.Client
}

// New builds a client. baseURL should be the ZenTao root (…/zentao); the
// /api.php/v1 suffix is appended automatically.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/api.php/v1") {
		base += "/api.php/v1"
	}
	return &Client{BaseURL: base, Token: token, HTTPClient: hc}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Token", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("zentao: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return raw, resp.StatusCode, nil
}

// Login exchanges an account + password for a session token (valid ~24h, per the
// server's expiredTime). Lets a user connect ZenTao without copying a token by
// hand, and lets the worker re-mint an expired token. Call New(baseURL, "", hc).
func (c *Client) Login(ctx context.Context, account, password string) (string, error) {
	raw, status, err := c.do(ctx, http.MethodPost, "/tokens", map[string]string{"account": account, "password": password})
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return "", fmt.Errorf("zentao login failed: %s", e.Error)
		}
		return "", fmt.Errorf("zentao login HTTP %d", status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("zentao: decode token: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("zentao login: empty token in response")
	}
	return out.Token, nil
}

// Task is a listed ZenTao task used for inbound mirroring.
type Task struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Status string `json:"status"`
}

// ListTasks lists tasks in an execution for inbound polling.
func (c *Client) ListTasks(ctx context.Context, executionID string) ([]Task, error) {
	raw, status, err := c.do(ctx, http.MethodGet, "/executions/"+executionID+"/tasks", nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("zentao: list tasks HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Tasks []Task `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("zentao: decode tasks: %w", err)
	}
	return out.Tasks, nil
}

// GetProject returns a project's fields as a generic map, used to give an agent
// the project's context (name, description, status, etc.).
func (c *Client) GetProject(ctx context.Context, projectID string) (map[string]any, error) {
	raw, status, err := c.do(ctx, http.MethodGet, "/projects/"+projectID, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("zentao: get project HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("zentao: decode project: %w", err)
	}
	return out, nil
}

// CreateTask creates a task in an execution and returns the new task id on
// success. A non-empty taskID means the task was created; otherwise errMsg
// carries the reason (e.g. a kanban project with no execution returns HTTP 400).
func (c *Client) CreateTask(ctx context.Context, executionID, name, description, estStarted, deadline string) (taskID, errMsg string, err error) {
	payload := map[string]any{
		"name":       name,
		"type":       "devel",
		"pri":        3,
		"desc":       description,
		"estStarted": estStarted,
		"deadline":   deadline,
	}
	raw, status, err := c.do(ctx, http.MethodPost, "/executions/"+executionID+"/tasks", payload)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		ID    string `json:"id"`
		Error any    `json:"error"`
	}
	_ = json.Unmarshal(raw, &resp)
	if status >= 200 && status < 300 && resp.ID != "" {
		return resp.ID, "", nil
	}
	return "", fmt.Sprintf("HTTP %d: %s", status, strings.TrimSpace(string(raw))), nil
}

// UpdateTaskStatus sets a task's status (wait/doing/done/pause/cancel/closed).
// ZenTao's task PUT is a full-form edit that revalidates required fields, so we
// GET the task first and resend name/execution/type/consumed with the new
// status. Used by controlled outbound sync when a mirrored issue's status
// changes in Multica.
func (c *Client) UpdateTaskStatus(ctx context.Context, taskID, status string) error {
	raw, code, err := c.do(ctx, http.MethodGet, "/tasks/"+taskID, nil)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("zentao: get task HTTP %d: %s", code, strings.TrimSpace(string(raw)))
	}
	var t map[string]any
	if err := json.Unmarshal(raw, &t); err != nil {
		return fmt.Errorf("zentao: decode task: %w", err)
	}
	payload := map[string]any{
		"name":      t["name"],
		"execution": t["execution"],
		"type":      t["type"],
		"consumed":  t["consumed"],
		"status":    status,
	}
	raw2, code2, err := c.do(ctx, http.MethodPut, "/tasks/"+taskID, payload)
	if err != nil {
		return err
	}
	if code2 < 200 || code2 >= 300 {
		return fmt.Errorf("zentao: update task status HTTP %d: %s", code2, strings.TrimSpace(string(raw2)))
	}
	return nil
}
