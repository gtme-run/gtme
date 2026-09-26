package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/docsgen"
)

// The reference collection and the glossary under docs/ are generated
// (cmd/docsgen) from `gtme help --agent`, docs/_adapters.json, the spec
// artifacts, and each concept page's `defines:`. This test regenerates them
// from the built binary and fails when the committed pages differ, so a
// verb, an adapter, a field, a table, or a definition cannot change without
// `make docs-reference` running and its result being committed.
func TestDocsReferenceMatchesSources(t *testing.T) {
	h := newHarness(t)
	res := h.runWithEnv([]string{"GTME_ADAPTER_PATH=" + t.TempDir()}, "", "help", "--agent")
	if res.code != 0 {
		t.Fatalf("help --agent: exit %d\n%s", res.code, res.stderr)
	}
	agentPath := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(agentPath, []byte(res.stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := docsgen.Load(repoRoot(), agentPath)
	if err != nil {
		t.Fatalf("loading generator inputs: %v", err)
	}
	out, err := docsgen.Generate(in)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	drift, err := docsgen.Drift(filepath.Join(repoRoot(), "docs"), out)
	if err != nil {
		t.Fatal(err)
	}
	if len(drift) > 0 {
		t.Fatalf("docs/ differs from what cmd/docsgen writes; run `make docs-reference` and commit the result:\n  %s", strings.Join(drift, "\n  "))
	}
}
