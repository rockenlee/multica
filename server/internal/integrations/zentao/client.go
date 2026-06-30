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
	"strconv"
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
	return c.doURL(ctx, method, c.BaseURL+path, body)
}

// baseV2 returns the api.php/v2 root derived from the v1 BaseURL. ZenTao 21.7.8
// applies task action transitions (start/finish/close) only on /api.php/v2; the
// v1 action routes return 2xx without changing task.status. The zentao CLI also
// talks to /api.php/v2.
func (c *Client) baseV2() string {
	return strings.TrimSuffix(c.BaseURL, "/api.php/v1") + "/api.php/v2"
}

// doURL issues a request against an absolute URL (used so action transitions can
// target the v2 root while the rest of the client stays on v1).
func (c *Client) doURL(ctx context.Context, method, url string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	Desc      string `json:"desc"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

func (t *Task) UnmarshalJSON(raw []byte) error {
	var aux struct {
		ID             any `json:"id"`
		Name           any `json:"name"`
		Desc           any `json:"desc"`
		Status         any `json:"status"`
		Parent         any `json:"parent"`
		ParentID       any `json:"parentID"`
		ParentIDSnake  any `json:"parent_id"`
		LastEditedDate any `json:"lastEditedDate"`
		LastEdited     any `json:"lastEdited"`
		EditedDate     any `json:"editedDate"`
		UpdatedAt      any `json:"updatedAt"`
		UpdatedAtSnake any `json:"updated_at"`
		FinishedDate   any `json:"finishedDate"`
		ClosedDate     any `json:"closedDate"`
		AssignedDate   any `json:"assignedDate"`
		OpenedDate     any `json:"openedDate"`
		CreatedAt      any `json:"createdAt"`
	}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return err
	}
	t.ID = taskFieldString(aux.ID)
	t.Name = taskFieldString(aux.Name)
	t.Desc = taskFieldString(aux.Desc)
	t.Status = taskFieldString(aux.Status)
	t.ParentID = firstTaskID(taskFieldString(aux.Parent), taskFieldString(aux.ParentID), taskFieldString(aux.ParentIDSnake))
	baseUpdatedAt := firstTaskTimestamp(
		taskFieldString(aux.LastEditedDate),
		taskFieldString(aux.LastEdited),
		taskFieldString(aux.EditedDate),
		taskFieldString(aux.UpdatedAt),
		taskFieldString(aux.UpdatedAtSnake),
		taskFieldString(aux.AssignedDate),
		taskFieldString(aux.OpenedDate),
		taskFieldString(aux.CreatedAt),
	)
	switch strings.ToLower(t.Status) {
	case "done":
		t.UpdatedAt = firstTaskTimestamp(taskFieldString(aux.FinishedDate), baseUpdatedAt)
	case "closed":
		t.UpdatedAt = firstTaskTimestamp(taskFieldString(aux.ClosedDate), baseUpdatedAt)
	default:
		t.UpdatedAt = baseUpdatedAt
	}
	return nil
}

func firstTaskID(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" && v != "0" {
			return v
		}
	}
	return ""
}

func firstTaskTimestamp(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || v == "0" || strings.HasPrefix(v, "0000-00-00") {
			continue
		}
		return v
	}
	return ""
}

func taskFieldString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return strings.TrimSpace(x.String())
	}
	return ""
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

// taskNum extracts a numeric task field that ZenTao may return either as a JSON
// number or a string ("0.00", "8").
func taskNum(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}

func firstPositive(vals ...float64) float64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

// taskDateOr returns the task's date field (date part only) when present and
// valid, else the fallback (callers pass today as "2006-01-02"). ZenTao stores
// empty dates as "0000-00-00 ..." and may include a time component.
func taskDateOr(v any, fallback string) string {
	s := ""
	if str, ok := v.(string); ok {
		s = strings.TrimSpace(str)
	}
	if s == "" || strings.HasPrefix(s, "0000-00-00") {
		return fallback
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return fallback
}

// zentaoEditStatus is a sentinel returned in place of an action verb to signal
// that the transition has no ZenTao action endpoint and must go through the
// generic full-form PUT /tasks/{id} instead. UpdateTaskStatus routes it to the
// task path rather than /tasks/{id}/{action}.
const zentaoEditStatus = "edit"

// zentaoActionForStatus maps a target ZenTao status to its action endpoint verb
// (PUT /tasks/{id}/{action}) and that action's required payload, derived from the
// current task. ZenTao 21.7.8 models these transitions as action endpoints —
// start/finish/close — each with its own required fields; the generic task PUT
// silently no-ops or 400s ("剩余不能为0") for those. It returns:
//   - a real action verb ("start"/"finish"/"close") with that action's payload;
//   - (zentaoEditStatus, zentaoCancelPayload) for cancel, which has no action
//     endpoint and must use the generic full-form PUT /tasks/{id};
//   - ("", nil) when the target has no supported path (wait, pause, unknown).
//     ZenTao 21.7.8 exposes only start/finish/close/activate as task action
//     endpoints — there is no /pause and no /cancel route; wait/pause are skipped
//     by the caller.
func zentaoActionForStatus(target string, t map[string]any, today string) (string, map[string]any) {
	switch target {
	case "doing":
		// start requires left > 0 (a "doing" task must have remaining effort) and
		// realStarted (the date work began).
		left := firstPositive(taskNum(t["left"]), taskNum(t["estimate"]), 1)
		return "start", map[string]any{
			"left":        left,
			"realStarted": taskDateOr(t["realStarted"], today),
		}
	case "done":
		// finish requires currentConsumed > 0, realStarted and finishedDate; left
		// is zeroed.
		consumed := firstPositive(taskNum(t["consumed"]), taskNum(t["estimate"]), 1)
		return "finish", map[string]any{
			"currentConsumed": consumed,
			"consumed":        consumed,
			"left":            0,
			"realStarted":     taskDateOr(t["realStarted"], today),
			"finishedDate":    taskDateOr(t["finishedDate"], today),
			"assignedTo":      t["assignedTo"],
		}
	case "closed":
		return "close", map[string]any{}
	case "cancel":
		// No /cancel action endpoint; use the generic full-form PUT /tasks/{id}.
		return zentaoEditStatus, zentaoCancelPayload(t)
	default: // wait / pause / unknown — no action endpoint
		return "", nil
	}
}

// zentaoCancelPayload builds the generic full-form task PUT body that cancels a
// task. ZenTao 21.7.8 has NO /tasks/{id}/cancel action endpoint; the verified
// path (the delete/tombstone flow relies on it) is the generic PUT /tasks/{id}
// with status=cancel, which revalidates the whole form, so we resend the required
// fields from the fetched task. Kept deliberately separate from the action-based
// transition helper above so cancel never routes to an unsupported action route.
func zentaoCancelPayload(t map[string]any) map[string]any {
	return map[string]any{
		"name":      t["name"],
		"execution": t["execution"],
		"type":      t["type"],
		"consumed":  t["consumed"],
		"status":    "cancel",
	}
}

// getTask fetches a task's fields as a generic map from the v1 task endpoint.
func (c *Client) getTask(ctx context.Context, taskID string) (map[string]any, error) {
	raw, code, err := c.do(ctx, http.MethodGet, "/tasks/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("zentao: get task HTTP %d: %s", code, strings.TrimSpace(string(raw)))
	}
	var t map[string]any
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("zentao: decode task: %w", err)
	}
	return t, nil
}

// UpdateTaskStatus moves a task to the target ZenTao status. Normal transitions
// use the real action endpoints on the v2 root (PUT /api.php/v2/tasks/{id}/{action}:
// start/finish/close), which apply ZenTao's per-action field rules. The v1 action
// routes return 2xx without changing task.status (a false success), so action
// transitions MUST go through v2. cancel has no action endpoint in ZenTao 21.7.8,
// so it falls back to the generic full-form PUT /tasks/{id} with status=cancel
// (the verified delete path, which works on v1). We GET the task first to satisfy
// per-action requirements (left for start, consumed for finish) and to no-op when
// already in the target status. After the write we GET again and verify the
// source-side task.status equals the requested target; if not, we return an error
// rather than report a false success. Used by controlled outbound sync. Returns
// applied=false with a nil error when the task is already in the target status or
// the target has no supported path (wait, pause) — callers can record a "skipped"
// event for that case versus a "success" when applied=true.
func (c *Client) UpdateTaskStatus(ctx context.Context, taskID, status string) (applied bool, err error) {
	t, err := c.getTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	if cur, _ := t["status"].(string); cur == status {
		return false, nil // already in the target status (idempotent skip)
	}
	action, payload := zentaoActionForStatus(status, t, time.Now().Format("2006-01-02"))
	if action == "" {
		return false, nil // no supported path for this target (wait, pause) — skip
	}
	var raw2 []byte
	var code2 int
	if action == zentaoEditStatus {
		// cancel has no action endpoint; PUT the generic full-form edit on v1.
		raw2, code2, err = c.do(ctx, http.MethodPut, "/tasks/"+taskID, payload)
	} else {
		// start/finish/close apply only on the v2 action endpoint.
		raw2, code2, err = c.doURL(ctx, http.MethodPut, c.baseV2()+"/tasks/"+taskID+"/"+action, payload)
	}
	if err != nil {
		return false, err
	}
	if code2 < 200 || code2 >= 300 {
		return false, fmt.Errorf("zentao: task %s HTTP %d: %s", action, code2, strings.TrimSpace(string(raw2)))
	}
	// A 2xx from the action/PUT does not guarantee the transition applied (the v1
	// action route was the source of false successes). Verify the source-side
	// status actually reached the target before reporting success.
	after, err := c.getTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	if cur, _ := after["status"].(string); cur != status {
		return false, fmt.Errorf("zentao: task %s status did not change to %q (still %q) after %s", taskID, status, cur, action)
	}
	return true, nil
}
