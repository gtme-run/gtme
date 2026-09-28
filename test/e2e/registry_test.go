package e2e

// The vendor adapters are registry entries (ADR-059, M33). The e2e suite
// installs them from test/fixtures/registry — a copy of the gtme-bindings
// entries, on the harness's GTME_ADAPTER_PATH — never from the network.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// registryDir is the local copy of the registry's vendor entries.
func registryDir() string {
	return filepath.Join(repoRoot(), "test", "fixtures", "registry")
}

// writeBinding installs an external binding adapter into the harness home
// under newID, rewriting the id line so it can sit beside another adapter.
func (h *harness) writeBinding(newID string, yamlPath string) {
	h.t.Helper()
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		h.t.Fatalf("reading %s: %v", yamlPath, err)
	}
	oldID := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "id: ") {
			oldID = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
			break
		}
	}
	if oldID == "" {
		h.t.Fatalf("no id line in %s", yamlPath)
	}
	doc := strings.Replace(string(raw), "id: "+oldID, "id: "+newID, 1)
	dir := filepath.Join(h.home, ".gtme", "adapters", strings.ReplaceAll(newID, "/", "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binding.yaml"), []byte(doc), 0o644); err != nil {
		h.t.Fatalf("write binding: %v", err)
	}
	// The conformance fixtures travel with the binding — they are what
	// --simulate serves and what a bundle packs (SPEC §8).
	if fixtures, err := os.ReadFile(filepath.Join(filepath.Dir(yamlPath), "fixtures", "conformance.json")); err == nil {
		if err := os.MkdirAll(filepath.Join(dir, "fixtures"), 0o755); err != nil {
			h.t.Fatalf("mkdir fixtures: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fixtures", "conformance.json"), fixtures, 0o644); err != nil {
			h.t.Fatalf("write fixtures: %v", err)
		}
	}
}

// ledgerFields reads every field value in a harness ledger, keyed by identity.
func ledgerFields(h *harness) map[string]map[string]string {
	rows, err := h.open().DB().Query(`
		SELECT i.entity_type || ':' || i.identity_key, f.field, f.value
		FROM field_values f JOIN identities i ON i.id = f.identity_id`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var id, field, value string
		if err := rows.Scan(&id, &field, &value); err != nil {
			h.t.Fatal(err)
		}
		if out[id] == nil {
			out[id] = map[string]string{}
		}
		out[id][field] = value
	}
	return out
}

// bindingFixtureServer serves a registry entry's conformance responses over
// HTTP, so a real run (not --simulate) exercises the engine end to end.
func bindingFixtureServer(t *testing.T, name string) *httptest.Server {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(registryDir(), name, "fixtures", "conformance.json"))
	if err != nil {
		t.Fatalf("fixtures for %s: %v", name, err)
	}
	var set struct {
		Responses []struct {
			Match  string          `json:"match"`
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("fixtures for %s: %v", name, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		for _, res := range set.Responses {
			if strings.Contains(key, res.Match) {
				w.Header().Set("Content-Type", "application/json")
				if res.Status != 0 {
					w.WriteHeader(res.Status)
				}
				w.Write(res.Body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestApolloRegistryEntry: apollo/search, installed from a local path,
// sources records end to end — pure YAML, no Go.
func TestApolloRegistryEntry(t *testing.T) {
	srv := bindingFixtureServer(t, "apollo-search")
	h := newHarness(t)
	h.write("p.yaml", fmt.Sprintf(`name: apollo-binding
source:
  use: apollo/search
  with:
    query: vp marketing
    base_url: %q
`, srv.URL))
	res := h.runWithEnv([]string{"APOLLO_API_KEY=k"}, "", "run", "p.yaml")
	if res.code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", res.code, res.stderr)
	}
	// Masked rows (ADR-043) key on the name-hash tier; find Jane by field.
	fields := ledgerFields(h)
	var jane map[string]string
	for _, f := range fields {
		if f["company_name"] == `"Acme Inc"` {
			jane = f
		}
	}
	if jane == nil || jane["last_name"] != `"D."` || jane["apollo.has_email"] != "true" {
		t.Errorf("jane = %v (all: %v)", jane, fields)
	}
	if _, has := jane["email"]; has {
		t.Errorf("masked search must not write an email: %v", jane["email"])
	}
	// The engine attached payloads under the default ADR-030 declaration.
	if n := h.queryInt(`SELECT count(*) FROM payloads WHERE adapter = 'apollo/search'`); n != 2 {
		t.Errorf("payloads = %d, want 2", n)
	}
}

// TestHarvestEntriesMatchTheGoAdapter is M33's parity acceptance: over the
// Go adapter's recorded fixtures (now the entries' conformance fixtures),
// harvest/profile@2 and harvest/recent-posts write the role_history and
// recent_posts the retired Go harvest/profile@1 wrote — the values below
// were captured from it before it was deleted. The record arrives with only
// the internal LinkedIn form, so the profile step's public-URL recovery
// (ADR-020) is what gives the posts step its linkedin_url.
func TestHarvestEntriesMatchTheGoAdapter(t *testing.T) {
	profile := bindingFixtureServer(t, "harvest-profile")
	posts := bindingFixtureServer(t, "harvest-recent-posts")
	h := newHarness(t)
	h.write("contacts.csv", "Full Name,Internal\nJane Doe,https://www.linkedin.com/in/ACwAA123\n")
	h.write("p.yaml", fmt.Sprintf(`name: harvest-parity
source:
  use: csv/source
  with:
    path: contacts.csv
    columns:
      full_name: Full Name
      linkedin_internal_url: Internal
steps:
  - id: profile
    use: harvest/profile
    with:
      base_url: %q
  - id: posts
    use: harvest/recent-posts
    with:
      base_url: %q
`, profile.URL, posts.URL))
	res := h.runWithEnv([]string{"HARVEST_API_KEY=k"}, "", "run", "p.yaml")
	if res.code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", res.code, res.stderr)
	}
	fields := ledgerFields(h)
	var jane map[string]string
	for _, f := range fields {
		if f["linkedin_url"] == `"https://www.linkedin.com/in/jane-doe"` {
			jane = f
		}
	}
	if jane == nil {
		t.Fatalf("no record carries the recovered public linkedin_url: %v", fields)
	}
	wantHistory := []string{
		"VP Marketing at Acme Inc (2022–present)",
		"Director of Demand Gen at Initech (2019–2022)",
	}
	wantPosts := []string{
		"We cut our CAC in half by killing three channels. Here is what we kept.",
		"Hiring two lifecycle marketers. DMs open.",
		"A fourth post that should not appear when the limit is three.",
	}
	for field, want := range map[string][]string{"role_history": wantHistory, "recent_posts": wantPosts} {
		var got []string
		if err := json.Unmarshal([]byte(jane[field]), &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %s, want %q", field, jane[field], want)
		}
	}
	// Each step bills its one call at the config default (ADR-046).
	for _, step := range []string{"profile", "posts"} {
		got := h.queryStrings(`SELECT CAST(total(amount_usd) AS TEXT) FROM costs WHERE step_id = '` + step + `'`)
		if len(got) != 1 || got[0] != "0.012" {
			t.Errorf("step %s cost = %v, want 0.012", step, got)
		}
	}
}

// TestHarvestProfileRefusesPostsLimit: version 2 dropped posts_limit, and
// plan names the entry that took it (ADR-059).
func TestHarvestProfileRefusesPostsLimit(t *testing.T) {
	h := newHarness(t)
	h.write("contacts.csv", "Full Name,LinkedIn\nJane Doe,https://www.linkedin.com/in/jane-doe\n")
	h.write("p.yaml", `name: posts-limit
source:
  use: csv/source
  with:
    path: contacts.csv
    columns:
      full_name: Full Name
      linkedin_url: LinkedIn
steps:
  - id: profile
    use: harvest/profile
    with:
      posts_limit: 3
`)
	res := h.run("plan", "p.yaml")
	if res.code == 0 {
		t.Fatalf("plan accepted posts_limit on harvest/profile@2:\n%s", res.stdout)
	}
	if out := res.stdout + res.stderr; !strings.Contains(out, "harvest/recent-posts") {
		t.Errorf("the plan error should name harvest/recent-posts:\n%s", out)
	}
}
