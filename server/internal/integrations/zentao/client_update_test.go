package zentao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// mockZentao is a stateful ZenTao stand-in: GET returns the current task, PUT
// records the path and (when applyWrites) flips the stored status to reflect the
// action verb / body status. applyWrites=false models the v1 false-success a
// 2xx-but-no-change action route — used to prove UpdateTaskStatus verifies.
type mockZentao struct {
	mu          sync.Mutex
	status      string
	paths       []string
	applyWrites bool
}

func (m *mockZentao) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.paths = append(m.paths, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "6354", "status": m.status, "name": "t",
				"execution": "447", "type": "devel", "consumed": "2", "left": "3",
			})
		case http.MethodPut:
			if m.applyWrites {
				switch {
				case strings.HasSuffix(r.URL.Path, "/start"):
					m.status = "doing"
				case strings.HasSuffix(r.URL.Path, "/finish"):
					m.status = "done"
				case strings.HasSuffix(r.URL.Path, "/close"):
					m.status = "closed"
				default: // generic full-form PUT carries the target status in its body
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if s, ok := body["status"].(string); ok {
						m.status = s
					}
				}
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func (m *mockZentao) recorded() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.paths))
	copy(out, m.paths)
	return out
}

func contains(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

func anyHasSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

func anyContains(paths []string, sub string) bool {
	for _, p := range paths {
		if strings.Contains(p, sub) {
			return true
		}
	}
	return false
}

// TestUpdateTaskStatusActionsUseV2 proves that doing/done/closed transitions PUT
// the v2 action endpoint (not v1) and that UpdateTaskStatus verifies the
// source-side status before reporting applied=true.
func TestUpdateTaskStatusActionsUseV2(t *testing.T) {
	cases := []struct {
		target string
		action string // expected verb on the v2 path
	}{
		{"doing", "start"},
		{"done", "finish"},
		{"closed", "close"},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			m := &mockZentao{status: "wait", applyWrites: true}
			srv := m.server()
			defer srv.Close()
			cl := New(srv.URL, "tok", srv.Client())

			applied, err := cl.UpdateTaskStatus(context.Background(), "6354", c.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !applied {
				t.Fatalf("applied = false, want true")
			}
			paths := m.recorded()
			wantV2 := "PUT /api.php/v2/tasks/6354/" + c.action
			if !contains(paths, wantV2) {
				t.Fatalf("expected %q in %v", wantV2, paths)
			}
			if anyContains(paths, "/api.php/v1/tasks/6354/"+c.action) {
				t.Fatalf("action must not route to v1, got %v", paths)
			}
		})
	}
}

// TestUpdateTaskStatusFailsWhenSourceUnchanged proves the v1 false-success case:
// a 2xx write that does not change the source-side status returns an error so the
// handler records an error event instead of a false success.
func TestUpdateTaskStatusFailsWhenSourceUnchanged(t *testing.T) {
	m := &mockZentao{status: "wait", applyWrites: false} // write 2xx but status never moves
	srv := m.server()
	defer srv.Close()
	cl := New(srv.URL, "tok", srv.Client())

	applied, err := cl.UpdateTaskStatus(context.Background(), "6354", "doing")
	if err == nil {
		t.Fatalf("expected verification error when status stays unchanged")
	}
	if applied {
		t.Fatalf("applied = true, want false on failed verification")
	}
	// The v2 action was still attempted before verification failed.
	if !contains(m.recorded(), "PUT /api.php/v2/tasks/6354/start") {
		t.Fatalf("expected v2 start attempt, got %v", m.recorded())
	}
}

// TestUpdateTaskStatusCancelStaysV1Generic proves cancel keeps the generic
// full-form PUT /api.php/v1/tasks/{id} and never routes to a /cancel action.
func TestUpdateTaskStatusCancelStaysV1Generic(t *testing.T) {
	m := &mockZentao{status: "wait", applyWrites: true}
	srv := m.server()
	defer srv.Close()
	cl := New(srv.URL, "tok", srv.Client())

	applied, err := cl.UpdateTaskStatus(context.Background(), "6354", "cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !applied {
		t.Fatalf("applied = false, want true")
	}
	paths := m.recorded()
	if !contains(paths, "PUT /api.php/v1/tasks/6354") {
		t.Fatalf("expected generic v1 PUT, got %v", paths)
	}
	if anyHasSuffix(paths, "/cancel") {
		t.Fatalf("cancel must not route to a /cancel action endpoint, got %v", paths)
	}
	if anyContains(paths, "/api.php/v2") {
		t.Fatalf("cancel must stay on v1, got %v", paths)
	}
}

// TestUpdateTaskStatusPauseSkipped proves pause is a no-op skip (applied=false,
// nil error) and issues no write.
func TestUpdateTaskStatusPauseSkipped(t *testing.T) {
	m := &mockZentao{status: "wait", applyWrites: true}
	srv := m.server()
	defer srv.Close()
	cl := New(srv.URL, "tok", srv.Client())

	applied, err := cl.UpdateTaskStatus(context.Background(), "6354", "pause")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Fatalf("applied = true, want false (pause is skipped)")
	}
	for _, p := range m.recorded() {
		if strings.HasPrefix(p, "PUT ") {
			t.Fatalf("pause must not issue a write, got %v", m.recorded())
		}
	}
}
