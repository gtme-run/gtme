package docsgen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func specFile(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The real spec/ledger.sql parses into every table and view SPEC §3 names,
// each table with its columns, and a semicolon inside a comment does not
// end a statement early.
func TestParseLedgerSQL(t *testing.T) {
	objects, err := parseLedgerSQL(specFile(t, "spec/ledger.sql"))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ledgerObject{}
	for _, o := range objects {
		byName[o.Name] = o
	}
	for _, want := range []string{"identities", "field_values", "relations", "runs", "run_records", "step_events", "costs", "deliveries", "groups", "group_events", "payloads"} {
		o, ok := byName[want]
		if !ok || o.Kind != "table" {
			t.Errorf("table %s missing", want)
		}
	}
	for _, want := range []string{"field_value_ranks", "current_fields", "group_members", "current_values", "group_membership"} {
		if o, ok := byName[want]; !ok || o.Kind != "view" {
			t.Errorf("view %s missing", want)
		}
	}
	d := byName["deliveries"]
	if got := len(d.Columns); got != 10 {
		t.Errorf("deliveries: %d columns parsed, want 10 (a `;` in a column comment ended the statement?)", got)
	}
	if len(d.Constraint) != 1 || !strings.HasPrefix(d.Constraint[0], "UNIQUE(") {
		t.Errorf("deliveries: table constraint %v", d.Constraint)
	}
	if !strings.Contains(byName["field_values"].Definition, "CREATE INDEX ix_fv_lookup") {
		t.Error("field_values: its index did not attach")
	}
	if !strings.HasPrefix(byName["field_value_ranks"].Definition, "-- The current-value projection") {
		t.Error("field_value_ranks: the comment block directly above it did not attach")
	}
}

func TestFlagsOf(t *testing.T) {
	v := verb{Name: "show", Forms: []agentVerb{
		{Usage: "gtme show <identity-key> [--fields a,b] [--provenance]", Does: "print the projection"},
		{Usage: "gtme show --run RUN_ID|last [--fields a,b] [--limit N]", Does: "list the records a run touched; --limit caps rows"},
	}}
	got := flagsOf(v)
	want := map[string]string{"--fields": "a,b", "--provenance": "", "--run": "RUN_ID|last", "--limit": "N"}
	if len(got) != len(want) {
		t.Fatalf("got %d flags %+v, want %d", len(got), got, len(want))
	}
	for _, f := range got {
		if arg, ok := want[f.Name]; !ok || arg != f.Arg {
			t.Errorf("%s: arg %q, want %q", f.Name, f.Arg, arg)
		}
		if f.Name == "--limit" && f.Does != "--limit caps rows" {
			t.Errorf("--limit: does %q", f.Does)
		}
	}
	g := verb{Name: "groups", Forms: []agentVerb{{Usage: "gtme groups [add NAME KEY...|--from-segment NAME|--query SQL|--type TYPE]"}}}
	for _, f := range flagsOf(g) {
		if strings.Contains(f.Arg, "--") {
			t.Errorf("%s: arg %q swallowed the next flag", f.Name, f.Arg)
		}
	}
}

func TestShort(t *testing.T) {
	long := "list groups with their entity type and derived character (members, added/removed/touched tallies), inspect one (members, events, and the pipelines that wrote to and sourced from it), or hand-edit membership; snapshots evaluate a segment"
	got := short(long)
	if len(got) > 160 {
		t.Errorf("%d chars: %s", len(got), got)
	}
	if strings.Count(got, "(") != strings.Count(got, ")") {
		t.Errorf("unbalanced: %s", got)
	}
	if got := short("execute a pipeline; --resume continues"); got != "Execute a pipeline" {
		t.Errorf("got %q", got)
	}
}

func TestRequired(t *testing.T) {
	if got := required("gtme answer [RUN_ID|last|PIPELINE] [STEP] [IDENTITY_KEY] [--set field=value ...] [--cost USD [--measured]]"); got != "gtme answer" {
		t.Errorf("got %q", got)
	}
}

func TestYamlItemUsing(t *testing.T) {
	doc := `name: x
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: score
    use: demo/enrich            # cached
    with:
      cost_per_record_usd: 0.01

  - id: out
    use: csv/deliver
    variables:
      score: demo.score
`
	if got := yamlItemUsing(doc, "demo/enrich"); !strings.HasPrefix(got, "  - id: score") || !strings.Contains(got, "cost_per_record_usd") || strings.Contains(got, "id: out") {
		t.Errorf("demo/enrich item:\n%s", got)
	}
	if got := yamlItemUsing(doc, "csv/source"); !strings.HasPrefix(got, "source:") || !strings.Contains(got, "people.csv") || strings.Contains(got, "steps:") {
		t.Errorf("csv/source item:\n%s", got)
	}
	if got := yamlItemUsing(doc, "csv/deliver"); !strings.HasPrefix(got, "  - id: out") || !strings.HasSuffix(got, "score: demo.score") {
		t.Errorf("csv/deliver item:\n%s", got)
	}
	if got := yamlItemUsing(doc, "nope/none"); got != "" {
		t.Errorf("unexpected match: %s", got)
	}
}

func TestCollectGlossaryRefusesTwoOwners(t *testing.T) {
	pages := []*docPage{
		{Path: "a.md", Defines: []entry{{term: "Ledger", definition: "x"}}},
		{Path: "b.md", Defines: []entry{{term: "ledger", definition: "y"}}},
	}
	if _, err := collectGlossary(pages); err == nil {
		t.Error("two pages defining one term should fail")
	}
}

func TestIsGenerated(t *testing.T) {
	if !IsGenerated("---\nname: x\ngenerated_by: \"y\"\n---\n\n# x\n") {
		t.Error("generated page not recognised")
	}
	if IsGenerated("---\nname: x\n---\n\ngenerated_by: in the body does not count\n") {
		t.Error("body text mistaken for frontmatter")
	}
}

// The CLI index lists each verb once, linking its own page; the verb page
// carries the forms and flags, and points at the shared exit codes
// instead of repeating them.
func TestCLIPagesDoNotRepeatEachOther(t *testing.T) {
	s := &site{
		agent: &agentDoc{
			Verbs: []agentVerb{
				{Usage: "gtme plan FILE", Does: "resolve adapters and print the plan"},
				{Usage: "gtme plan FILE --json", Does: "print the plan as JSON"},
				{Usage: "gtme run FILE", Does: "execute a pipeline"},
			},
			ExitCodes: []agentExit{{Code: 0, Means: "ok"}, {Code: 2, Means: "refused"}},
		},
		backlinks: map[string][]backlink{},
	}
	pages := s.cliPages()
	if len(pages) != 3 {
		t.Fatalf("want index + 2 verb pages, got %d", len(pages))
	}
	index := pages[0].body
	for _, want := range []string{"[`gtme plan`](/reference/cli/plan)", "[`gtme run`](/reference/cli/run)", "## Exit codes", "| 2 | refused |"} {
		if !strings.Contains(index, want) {
			t.Errorf("index lacks %q:\n%s", want, index)
		}
	}
	for _, no := range []string{"## gtme plan", "## gtme run", "| Form |"} {
		if strings.Contains(index, no) {
			t.Errorf("index still carries %q:\n%s", no, index)
		}
	}
	plan := pages[1].body
	for _, want := range []string{"## Forms", "gtme plan FILE --json", "| Form |", "(/reference/cli#exit-codes)"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan page lacks %q:\n%s", want, plan)
		}
	}
	if strings.Contains(plan, "## Exit codes") || strings.Contains(plan, "| 2 | refused |") {
		t.Errorf("plan page repeats the exit codes:\n%s", plan)
	}
}
