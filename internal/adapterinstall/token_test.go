package adapterinstall

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A stale GITHUB_TOKEN must not block a public install (#97): GitHub's 401
// is about the token, so the request is asked again without it.
func TestResolveCommitRetriesWithoutARejectedToken(t *testing.T) {
	var withToken, without int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			withToken++
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		without++
		w.Write([]byte(`{"sha":"` + strings.Repeat("a", 40) + `"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GTME_GITHUB_API", srv.URL)
	t.Setenv("GITHUB_TOKEN", "ghp_stale")
	tokenRejected.Store(false)

	sha, err := ResolveCommit(Ref{Owner: "o", Repo: "r", Path: "p", Ref: "main"})
	if err != nil {
		t.Fatalf("a public ref failed behind a stale token: %v", err)
	}
	if sha != strings.Repeat("a", 40) || withToken != 1 || without != 1 {
		t.Errorf("sha = %q, requests with token = %d, without = %d", sha, withToken, without)
	}
}

// When the retry also fails (a private repository), the error names the
// token: the 404 alone does not say why.
func TestResolveCommitNamesARejectedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GTME_GITHUB_API", srv.URL)
	t.Setenv("GITHUB_TOKEN", "ghp_stale")
	tokenRejected.Store(false)

	_, err := ResolveCommit(Ref{Owner: "o", Repo: "r", Path: "p", Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") || !strings.Contains(err.Error(), "404") {
		t.Errorf("error should carry the 404 and name the token, got: %v", err)
	}
}
