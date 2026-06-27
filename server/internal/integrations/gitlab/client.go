// Package gitlab is a minimal GitLab REST client for the integration module's
// outbound issue sync. It implements only what outbound creation needs:
// turning a Multica issue into a real GitLab issue. Inbound mirroring and
// repo content fetch live elsewhere (the daemon clones repos directly).
package gitlab

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

const defaultTimeout = 15 * time.Second

// Client talks to a GitLab instance with a personal access token.
type Client struct {
	BaseURL    string // e.g. https://gitlab.hbc.tech
	Token      string // PRIVATE-TOKEN
	HTTPClient *http.Client
}

// New builds a client. A nil http.Client falls back to a timeout-bound default.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTPClient: hc}
}

// CreateIssueNote posts a note (comment) on an issue. Used by controlled
// outbound sync when a comment is added to a mirrored issue in Multica.
func (c *Client) CreateIssueNote(ctx context.Context, projectRef string, iid int64, body string) error {
	if c.Token == "" {
		return fmt.Errorf("gitlab: missing credential")
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues/%d/notes", c.BaseURL, url.PathEscape(projectRef), iid)
	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab: create note failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("gitlab: create note HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

type createdIssue struct {
	ID     int64  `json:"id"`
	IID    int64  `json:"iid"`
	WebURL string `json:"web_url"`
	State  string `json:"state"`
}

// UpdateIssueState closes or reopens an issue. stateEvent is "close" or
// "reopen" (GitLab's only state transitions). Used by controlled outbound sync
// when a mirrored issue's status changes in Multica.
func (c *Client) UpdateIssueState(ctx context.Context, projectRef string, iid int64, stateEvent string) error {
	if c.Token == "" {
		return fmt.Errorf("gitlab: missing credential")
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues/%d?state_event=%s",
		c.BaseURL, url.PathEscape(projectRef), iid, url.QueryEscape(stateEvent))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab: update issue state failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("gitlab: update issue state HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// CreateIssue opens an issue in the given project (path "group/repo" or a
// numeric id) and returns the per-project issue number, web URL and state.
func (c *Client) CreateIssue(ctx context.Context, projectRef, title, description string) (sourceID, sourceURL, state string, err error) {
	if c.Token == "" {
		return "", "", "", fmt.Errorf("gitlab: missing credential")
	}
	if strings.TrimSpace(projectRef) == "" {
		return "", "", "", fmt.Errorf("gitlab: missing project reference")
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues", c.BaseURL, url.PathEscape(projectRef))
	body, _ := json.Marshal(map[string]string{"title": title, "description": description})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("gitlab: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", fmt.Errorf("gitlab: create issue HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out createdIssue
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", "", fmt.Errorf("gitlab: decode response: %w", err)
	}
	return strconv.FormatInt(out.IID, 10), out.WebURL, out.State, nil
}

type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// ListFiles returns up to `limit` blob paths in the project's default branch,
// used to give an agent the resource's file inventory.
func (c *Client) ListFiles(ctx context.Context, projectRef string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/tree?recursive=true&per_page=%d",
		c.BaseURL, url.PathEscape(projectRef), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: list files failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab: list files HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var entries []treeEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("gitlab: decode tree: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type == "blob" {
			files = append(files, e.Path)
		}
	}
	return files, nil
}

// GetFile fetches the raw content of a file at the given path (default branch
// when ref is empty).
func (c *Client) GetFile(ctx context.Context, projectRef, path, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		c.BaseURL, url.PathEscape(projectRef), url.PathEscape(path), url.QueryEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gitlab: get file failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("gitlab: get file HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return string(raw), nil
}

// Issue is a listed GitLab issue used for inbound mirroring.
type Issue struct {
	IID         int64  `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
	State       string `json:"state"`
	UpdatedAt   string `json:"updated_at"`
}

// ListIssues returns issues in a project for inbound polling. state is "opened",
// "closed", or "all"; updatedAfter (RFC3339, optional) limits to recent changes.
func (c *Client) ListIssues(ctx context.Context, projectRef, state, updatedAfter string, perPage int) ([]Issue, error) {
	if perPage <= 0 {
		perPage = 50
	}
	if state == "" {
		state = "all"
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues?state=%s&per_page=%d&order_by=updated_at",
		c.BaseURL, url.PathEscape(projectRef), url.QueryEscape(state), perPage)
	if strings.TrimSpace(updatedAfter) != "" {
		endpoint += "&updated_after=" + url.QueryEscape(updatedAfter)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: list issues failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab: list issues HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var issues []Issue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, fmt.Errorf("gitlab: decode issues: %w", err)
	}
	return issues, nil
}

// Ping verifies the credential by fetching the authenticated user. Used by the
// connection test path.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v4/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("gitlab: ping HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
