// Package github is a minimal GitHub REST client for reading repository
// content so an agent can inspect a github_repo project resource. It mirrors
// the gitlab client's read surface (ListFiles/GetFile) and nothing more —
// issue sync is not implemented here, and the daemon clones repos directly for
// task execution.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiBase        = "https://api.github.com"
	defaultTimeout = 15 * time.Second
)

// Client talks to the GitHub REST API. Token is optional: when empty the
// client makes unauthenticated requests, which work for public repositories.
// There is no github connection/credential in the integration module today
// (the daemon relies on local `gh` auth when cloning), so the content-fetch
// path runs unauthenticated and private repos surface GitHub's own 404/403.
type Client struct {
	Token      string
	HTTPClient *http.Client
}

// New builds a client. A nil http.Client falls back to a timeout-bound default.
func New(token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{Token: strings.TrimSpace(token), HTTPClient: hc}
}

// RepoFromURL extracts an "owner/repo" reference from a github repo URL. It
// accepts https/ssh/git URLs and scp-like ssh shorthand (git@github.com:owner/
// repo.git), trimming a trailing ".git". Returns "" when it cannot find exactly
// an owner and repo segment.
func RepoFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		path = u.Path
	} else if colon := strings.Index(raw, ":"); colon > 0 {
		// scp-like ssh shorthand: [user@]host:owner/repo
		path = raw[colon+1:]
	} else {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func (c *Client) newRequest(ctx context.Context, endpoint, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

// defaultBranch resolves a repo's default branch via the repository endpoint.
func (c *Client) defaultBranch(ctx context.Context, repo string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s", apiBase, repo)
	req, err := c.newRequest(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: get repo failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github: get repo HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("github: decode repo: %w", err)
	}
	if out.DefaultBranch == "" {
		return "", fmt.Errorf("github: repo has no default branch")
	}
	return out.DefaultBranch, nil
}

type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// ListFiles returns up to `limit` blob paths in the given branch (the repo's
// default branch when branch is empty), used to give an agent the resource's
// file inventory.
func (c *Client) ListFiles(ctx context.Context, repo, branch string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		b, err := c.defaultBranch(ctx, repo)
		if err != nil {
			return nil, err
		}
		branch = b
	}
	endpoint := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", apiBase, repo, url.PathEscape(branch))
	req, err := c.newRequest(ctx, endpoint, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: list files failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github: list files HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Tree []treeEntry `json:"tree"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("github: decode tree: %w", err)
	}
	files := make([]string, 0, len(out.Tree))
	for _, e := range out.Tree {
		if e.Type == "blob" {
			files = append(files, e.Path)
			if len(files) >= limit {
				break
			}
		}
	}
	return files, nil
}

// GetFile fetches the raw content of a file at the given path (default branch
// when ref is empty).
func (c *Client) GetFile(ctx context.Context, repo, path, ref string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/contents/%s", apiBase, repo, gitHubEscapePath(path))
	if ref = strings.TrimSpace(ref); ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	req, err := c.newRequest(ctx, endpoint, "application/vnd.github.raw")
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: get file failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github: get file HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return string(raw), nil
}

// gitHubEscapePath percent-escapes each path segment while preserving the "/"
// separators that the contents API requires.
func gitHubEscapePath(p string) string {
	segments := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}
