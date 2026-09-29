package adapterinstall

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveIndex(t *testing.T, doc any) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
	t.Cleanup(srv.Close)
	t.Setenv("GTME_REGISTRY", srv.URL+"/index.json")
	t.Setenv("GITHUB_TOKEN", "")
}

func row(id, role string) map[string]any {
	return map[string]any{
		"id": id, "description": "d", "role": role, "entity_type": "person",
		"source": map[string]any{"url": "github.com/o/r", "path": id, "ref": "main", "sha": strings.Repeat("1", 40)},
		"sha256": strings.Repeat("a", 64), "tier": "community",
	}
}

// TestLoadIndexAcceptsEverySpecRole (#174): every role SPEC §6 defines,
// traverse and review included, is a valid index row.
func TestLoadIndexAcceptsEverySpecRole(t *testing.T) {
	roles := []string{"source", "traverse", "filter", "enrich", "verify", "compose", "review", "deliver"}
	var rows []map[string]any
	for _, r := range roles {
		rows = append(rows, row("v/"+r, r))
	}
	serveIndex(t, map[string]any{"version": 1, "bindings": rows})
	ix, err := LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Bindings) != len(roles) || len(ix.Skipped) != 0 {
		t.Fatalf("loaded %d, skipped %v; want all %d roles", len(ix.Bindings), ix.Skipped, len(roles))
	}
}

// TestLoadIndexSkipsUnreadableRows (#174): a row this binary cannot read is
// skipped and named, and the rest of the index still loads, including a
// process entry.
func TestLoadIndexSkipsUnreadableRows(t *testing.T) {
	unknownKind := row("v/wasm", "enrich")
	unknownKind["kind"] = "wasm"
	missing := row("v/nodesc", "enrich")
	delete(missing, "description")
	process := row("v/proc", "deliver")
	delete(process, "sha256")
	process["kind"], process["tier"], process["release"] = "process", "verified", "v1.0.0"
	process["assets"] = map[string]any{"darwin/arm64": map[string]any{"url": "https://x/a.tar.gz", "sha256": strings.Repeat("b", 64)}}
	serveIndex(t, map[string]any{"version": 1, "bindings": []any{
		row("v/good", "source"), row("v/future", "summon"), unknownKind, missing, "not an object", process,
	}})

	ix, err := LoadIndex()
	if err != nil {
		t.Fatalf("one bad row must not reject the whole index: %v", err)
	}
	var got []string
	for _, e := range ix.Bindings {
		got = append(got, e.ID)
	}
	if strings.Join(got, ",") != "v/good,v/proc" {
		t.Errorf("loaded %v, want v/good and v/proc", got)
	}
	if e := ix.Find("v/proc"); e == nil || !e.IsProcess() || e.Release != "v1.0.0" {
		t.Errorf("process entry not loaded intact: %+v", e)
	}
	var skipped []string
	for _, s := range ix.Skipped {
		if s.Err == nil {
			t.Errorf("skipped %s carries no reason", s.ID)
		}
		skipped = append(skipped, s.ID)
	}
	if r := ix.Skipped[0].Reason(); !strings.HasPrefix(r, "role: ") {
		t.Errorf("reason = %q, want it to name the role member", r)
	}
	if strings.Join(skipped, ",") != "v/future,v/wasm,v/nodesc,row 4" {
		t.Errorf("skipped %v, want v/future, v/wasm, v/nodesc and row 4 (bindings/4)", skipped)
	}
}

// TestLoadIndexRefusesABadDocument: the document itself (its version, its
// bindings array) is still checked whole.
func TestLoadIndexRefusesABadDocument(t *testing.T) {
	serveIndex(t, map[string]any{"version": 2, "bindings": []any{row("v/good", "source")}})
	if _, err := LoadIndex(); err == nil {
		t.Error("an index at an unknown version loaded")
	}
	serveIndex(t, map[string]any{"version": 1, "bindings": "nope"})
	if _, err := LoadIndex(); err == nil {
		t.Error("an index whose bindings is not an array loaded")
	}
}
