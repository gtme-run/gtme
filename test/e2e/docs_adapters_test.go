package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// docs/_adapters.json is the built-in adapter listing gtme.run renders its
// connector pages from: `gtme help --agent` from a clean home, so only the
// adapters compiled into the binary appear. This test keeps the committed
// file honest — when a manifest changes, `make docs-adapters` regenerates it
// and this fails until the result is committed.
func TestDocsAdaptersJSONMatchesBinary(t *testing.T) {
	h := newHarness(t)
	// Override the harness's adapter path (last duplicate wins in exec.Cmd)
	// so the repo's external fixture adapters do not appear as built-ins.
	res := h.runWithEnv([]string{"GTME_ADAPTER_PATH=" + t.TempDir()}, "", "help", "--agent")
	if res.code != 0 {
		t.Fatalf("help --agent: exit %d\n%s", res.code, res.stderr)
	}
	live := canonicalAdapters(t, []byte(res.stdout), "gtme help --agent")

	path := filepath.Join(repoRoot(), "docs", "_adapters.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	committed := canonicalAdapters(t, raw, path)

	if live != committed {
		t.Fatalf("docs/_adapters.json does not match the binary's built-in adapters; run `make docs-adapters` and commit the result")
	}
}

// canonicalAdapters re-encodes a document's "adapters" array with sorted
// keys and no whitespace, so formatting differences cannot fail the test.
func canonicalAdapters(t *testing.T, raw []byte, what string) string {
	t.Helper()
	var doc struct {
		Adapters []any `json:"adapters"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: not JSON: %v", what, err)
	}
	if len(doc.Adapters) == 0 {
		t.Fatalf("%s: no adapters listed", what)
	}
	out, err := json.Marshal(doc.Adapters)
	if err != nil {
		t.Fatalf("%s: re-encoding: %v", what, err)
	}
	return string(out)
}
