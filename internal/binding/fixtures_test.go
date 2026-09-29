package binding

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// serve asks a fixture set which response answers url, returning its body's
// "which" field, or "" for the no-match 404.
func serve(t *testing.T, set *FixtureSet, method, url string) string {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := set.Doer().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	which, _ := body["which"].(string)
	return which
}

func fixtureSet(matches ...string) *FixtureSet {
	set := &FixtureSet{}
	for _, m := range matches {
		set.Responses = append(set.Responses, FixtureResponse{Match: m, Body: map[string]any{"which": m}})
	}
	return set
}

// TestFixtureQueryMatchIsExactPerParameter (#166): page=2& must not answer
// a request whose page is 3 just because per_page=2& is in its query.
func TestFixtureQueryMatchIsExactPerParameter(t *testing.T) {
	set := fixtureSet("?page=1&", "?page=2&", "?page=3&")
	for url, want := range map[string]string{
		"https://api.vendor.test/companies?page=1&per_page=2&q=crm": "?page=1&",
		"https://api.vendor.test/companies?page=3&per_page=2&q=crm": "?page=3&",
		"https://api.vendor.test/companies?page=2&per_page=5":       "?page=2&",
		"https://api.vendor.test/companies?offset=0&page=2":         "?page=2&", // last param, no trailing &
		"https://api.vendor.test/companies?page=20&per_page=2":      "",         // 20 is not 2
	} {
		if got := serve(t, set, "GET", url); got != want {
			t.Errorf("%s answered by %q, want %q", url, got, want)
		}
	}
	// The issue's own shape: no leading ?, so only per-parameter matching
	// keeps page=2& out of per_page=2&.
	bare := fixtureSet("page=2&", "page=3&")
	if got := serve(t, bare, "GET", "https://api.vendor.test/companies?page=3&per_page=2&q=crm"); got != "page=3&" {
		t.Errorf("page 3 answered by %q, want %q", got, "page=3&")
	}
}

// TestFixtureMatchKeepsPathAndPlainForms: the "METHOD path" form, a path
// with a query, and a bare fragment of the URL still match as before.
func TestFixtureMatchKeepsPathAndPlainForms(t *testing.T) {
	cases := []struct {
		match, method, url string
		want               bool
	}{
		{"GET /verify", "GET", "https://api.verifier.example/verify?email=jane%40acme.com", true},
		{"POST /verify", "GET", "https://api.verifier.example/verify", false},
		{"GET /v1/find?domain=acme.com", "GET", "https://api.example/v1/find?domain=acme.com&name=Jane", true},
		{"GET /v1/find?domain=acme.com", "GET", "https://api.example/v1/find?domain=acme.com.au", false},
		{"jane-doe", "GET", "https://api.example/profile?url=https%3A%2F%2Flinkedin.com%2Fin%2Fjane-doe", true},
		{"email=jane.doe@acme.com", "GET", "https://api.example/verify?email=jane.doe%40acme.com", true},
		{"email=jane.doe@acme.com", "GET", "https://api.example/verify?other_email=jane.doe%40acme.com", false},
	}
	for _, c := range cases {
		got := serve(t, fixtureSet(c.match), c.method, c.url) != ""
		if got != c.want {
			t.Errorf("match %q vs %s %s = %v, want %v", c.match, c.method, c.url, got, c.want)
		}
	}
}
