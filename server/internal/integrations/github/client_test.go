package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/rockenlee/opentypeless.git": "rockenlee/opentypeless",
		"https://github.com/rockenlee/opentypeless":     "rockenlee/opentypeless",
		"http://github.com/owner/repo/":                 "owner/repo",
		"git@github.com:rockenlee/opentypeless.git":     "rockenlee/opentypeless",
		"ssh://git@github.com/owner/repo.git":           "owner/repo",
		"":                                              "",
		"https://github.com/owner":                      "",
		"not a url":                                     "",
		"https://github.com/a/b/c.git":                  "",
	}
	for in, want := range cases {
		if got := RepoFromURL(in); got != want {
			t.Errorf("RepoFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// newTestClient points the client at a test server by rewriting the api base
// through the http.Client transport.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := New("", srv.Client())
	c.HTTPClient.Transport = rewriteTransport{base: srv.URL, rt: srv.Client().Transport}
	return c
}

// rewriteTransport redirects api.github.com requests to the test server.
type rewriteTransport struct {
	base string
	rt   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), apiBase) {
		newURL := t.base + strings.TrimPrefix(req.URL.String(), apiBase)
		r, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		r.Header = req.Header
		req = r
	}
	rt := t.rt
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(req)
}

func TestListFilesResolvesDefaultBranch(t *testing.T) {
	var sawTreeRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/repo":
			w.Write([]byte(`{"default_branch":"main"}`))
		case strings.HasPrefix(r.URL.Path, "/repos/owner/repo/git/trees/"):
			sawTreeRef = strings.TrimPrefix(r.URL.Path, "/repos/owner/repo/git/trees/")
			w.Write([]byte(`{"tree":[{"path":"README.md","type":"blob"},{"path":"src","type":"tree"},{"path":"src/main.go","type":"blob"}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	files, err := newTestClient(t, srv).ListFiles(context.Background(), "owner/repo", "", 100)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if sawTreeRef != "main" {
		t.Errorf("tree ref = %q, want main", sawTreeRef)
	}
	want := []string{"README.md", "src/main.go"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Errorf("files = %v, want %v", files, want)
	}
}

func TestListFilesUsesBranchHintAndLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo" {
			t.Errorf("should not resolve default branch when hint is given")
		}
		if !strings.HasSuffix(r.URL.Path, "/git/trees/dev") {
			t.Errorf("unexpected tree path %s", r.URL.Path)
		}
		w.Write([]byte(`{"tree":[{"path":"a","type":"blob"},{"path":"b","type":"blob"},{"path":"c","type":"blob"}]}`))
	}))
	defer srv.Close()

	files, err := newTestClient(t, srv).ListFiles(context.Background(), "owner/repo", "dev", 2)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("limit not applied: got %d files", len(files))
	}
}

func TestGetFileRawContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/contents/dir/file.txt" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("ref = %q, want main", r.URL.Query().Get("ref"))
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github.raw" {
			t.Errorf("Accept = %q", accept)
		}
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	content, err := newTestClient(t, srv).GetFile(context.Background(), "owner/repo", "dir/file.txt", "main")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if content != "hello world" {
		t.Errorf("content = %q", content)
	}
}

func TestGetFilePrivateRepoSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetFile(context.Background(), "owner/repo", "x", "")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected HTTP 404 error, got %v", err)
	}
}
