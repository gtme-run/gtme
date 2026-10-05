package e2e

import (
	"strings"
	"testing"
)

// TestPlanNamesAnExplicitCacheOff (#130): `cache: 0d` turns caching off, and
// the plan says the operator set it rather than that nothing is set.
func TestPlanNamesAnExplicitCacheOff(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", strings.Replace(csvToMockYAML, "cache: 30d", "cache: 0d", 1))
	res := h.mustRun("plan", "pipeline.yaml")
	contains(t, res.stderr, "cache:     off (cache: 0d)", "plan output")
	if strings.Contains(res.stderr, "no cache:") {
		t.Errorf("plan says cache: is not set when it is:\n%s", res.stderr)
	}
}

// TestGroupsAddUnknownKeyNamesTheKey (#124): a key the ledger has never seen
// is named, with the fix, instead of a bare "ledger: not found".
func TestGroupsAddUnknownKeyNamesTheKey(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", csvToMockYAML)
	h.mustRun("run", "pipeline.yaml")

	res := h.run("groups", "add", "customers", "nobody@nowhere.io")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, `no identity known by key "nobody@nowhere.io"`, "stderr")
	contains(t, res.stderr, "source it first", "stderr")
	if strings.Contains(res.stderr, "ledger: not found") {
		t.Errorf("the internal error leaked:\n%s", res.stderr)
	}
}

// TestQuerySaveKeepsOnlyAQueryThatRuns (#140): a statement that fails is not
// stored, so --list never holds a segment that cannot execute.
func TestQuerySaveKeepsOnlyAQueryThatRuns(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", csvToMockYAML)
	h.mustRun("run", "pipeline.yaml")

	for name, sql := range map[string]string{
		"typo":   `SELECT identity_id FROM current_valuez`,
		"sneaky": `WITH x AS (SELECT 1) DELETE FROM deliveries`,
	} {
		res := h.run("query", "--save", name, sql)
		if res.code != 2 {
			t.Errorf("exit = %d for %q, want 2\n%s", res.code, sql, res.stderr)
		}
		if strings.Contains(res.stderr, "saved segment") {
			t.Errorf("a failing query was reported saved:\n%s", res.stderr)
		}
		contains(t, res.stderr, "was not saved", "stderr for "+name)
		if got := h.run("query", "--name", name); got.code != 2 || !strings.Contains(got.stderr, "no saved segment named") {
			t.Errorf("segment %q exists after a failed save (exit %d):\n%s", name, got.code, got.stderr)
		}
	}

	// A query that runs is still saved, and replays.
	ok := h.mustRun("query", "--save", "people", `SELECT id AS identity_id FROM identities`)
	contains(t, ok.stderr, `saved segment "people"`, "stderr")
	h.mustRun("query", "--name", "people")
}

// TestPlanSuggestsTheNearestAdapterForATypo (#159): a near miss of an adapter
// this machine has is a typo, so plan suggests it instead of an install.
func TestPlanSuggestsTheNearestAdapterForATypo(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", strings.Replace(csvToMockYAML, "use: mock-enrich-py", "use: demo/enrcih", 1))
	res := h.run("plan", "pipeline.yaml")
	if res.code == 0 {
		t.Fatalf("plan passed with an unknown adapter\n%s", res.stderr)
	}
	contains(t, res.stderr, `unknown adapter "demo/enrcih" — did you mean "demo/enrich"?`, "stderr")
	if strings.Contains(res.stderr, "gtme adapters add demo/enrcih") {
		t.Errorf("plan told the operator to install a typo:\n%s", res.stderr)
	}
	if n := strings.Count(res.stderr, "looked for: "); n != strings.Count(res.stderr, "{manifest.json + run, or binding.yaml}") || hasRepeatedLine(res.stderr, "looked for: ") {
		t.Errorf("a looked-for line repeats:\n%s", res.stderr)
	}

	// An id nothing here resembles still gets the install line.
	h.write("far.yaml", strings.Replace(csvToMockYAML, "use: mock-enrich-py", "use: hubspot/contact-search", 1))
	res = h.run("plan", "far.yaml")
	contains(t, res.stderr, "gtme adapters add hubspot/contact-search", "stderr")
}

func hasRepeatedLine(text, prefix string) bool {
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if seen[line] {
			return true
		}
		seen[line] = true
	}
	return false
}

// TestHelpAgentNamesConcurrencyAndLegs (#138, #141): the agent document
// lists the run flag that sizes the worker pool, and calls a traverse's
// typed stretch a leg, the word ADR-058 fixed.
func TestHelpAgentNamesConcurrencyAndLegs(t *testing.T) {
	h := newHarness(t)
	res := h.mustRun("help", "--agent")
	contains(t, res.stdout, "--concurrency N", "help --agent")
	contains(t, res.stdout, "into a new leg of entity_type", "help --agent")
	if strings.Contains(res.stdout, "into a new segment of") {
		t.Errorf("help --agent still calls a leg a segment")
	}
}

// TestPlanReportsOnlyTheMissingAdapter (#159): what an unknown adapter
// provides is unknown, so the unmet needs of the steps after it follow from
// it and are not reported as problems of their own.
func TestPlanReportsOnlyTheMissingAdapter(t *testing.T) {
	h := newHarness(t)
	h.write("pipeline.yaml", `name: missing
version: 1
source:
  use: hubspot/contact-search
steps:
  - id: opener
    use: text/compose
    uses: [first_name]
    provides: [opener]
    with:
      template: "Hi {{ record.first_name }}"
`)
	res := h.run("plan", "pipeline.yaml")
	if res.code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "gtme adapters add hubspot/contact-search", "stderr")
	if strings.Contains(res.stderr, "which no earlier step provides") {
		t.Errorf("plan reported a need that only follows from the missing adapter:\n%s", res.stderr)
	}

	// With every adapter present, an unmet need is still a problem.
	h.write("people.csv", peopleCSV)
	h.write("present.yaml", `name: present
version: 1
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: opener
    use: text/compose
    uses: [first_name]
    provides: [opener]
    with:
      template: "Hi {{ record.first_name }}"
`)
	res = h.run("plan", "present.yaml")
	contains(t, res.stderr, "needs first_name, which no earlier step provides", "stderr with the source installed")
}

// TestAdaptersHelpFlagPrintsUsage (#158): a help flag after a subcommand is
// a question, not a reference to install.
func TestAdaptersHelpFlagPrintsUsage(t *testing.T) {
	h := newHarness(t)
	res := h.run("adapters", "add", "--help")
	if res.code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "usage: gtme adapters", "stderr")
	if strings.Contains(res.stderr, "is not github.com") {
		t.Errorf("--help was parsed as a reference:\n%s", res.stderr)
	}
}
