package docsgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// singles renders the six single reference pages from the real repo files.
func singles(t *testing.T) (map[string]string, map[string][]byte) {
	t.Helper()
	agent := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(agent, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := Load(repoRoot(t), agent)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := parseDocs(in.Docs)
	if err != nil {
		t.Fatal(err)
	}
	repo := in.Repo
	s := &site{agent: &agentDoc{}, pages: docs, backlinks: backlinks(docs)}
	pages, err := s.singlePages(repo)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range pages {
		out[p.path] = s.assemble(p)
	}
	return out, repo
}

// Every single page renders at reference/<slug>.md, opens with the
// generator's marker, and carries the frontmatter the lint and the site
// read: name, a description under 160 characters, for, learn, and
// generated_by.
func TestSinglePagesRenderWithMarker(t *testing.T) {
	pages, _ := singles(t)
	if len(pages) != len(singleSlugs) {
		t.Fatalf("%d pages, want %d", len(pages), len(singleSlugs))
	}
	for _, slug := range singleSlugs {
		path := "reference/" + slug + ".md"
		text, ok := pages[path]
		if !ok {
			t.Errorf("%s: not rendered", path)
			continue
		}
		if !strings.HasPrefix(text, "---\n"+markerLine+"\n") {
			t.Errorf("%s: first frontmatter line is not the generator's marker", path)
		}
		if !IsGenerated(text) {
			t.Errorf("%s: no generated_by", path)
		}
		fm, body, err := splitFrontmatter(text)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var f struct {
			Name        string   `yaml:"name"`
			Description string   `yaml:"description"`
			For         string   `yaml:"for"`
			Learn       []string `yaml:"learn"`
			Links       []struct {
				To, Description string
			} `yaml:"links"`
		}
		if err := yaml.Unmarshal([]byte(fm), &f); err != nil {
			t.Fatalf("%s: frontmatter: %v", path, err)
		}
		if f.Name == "" || f.Description == "" || f.For == "" || len(f.Learn) < 2 || len(f.Learn) > 4 {
			t.Errorf("%s: frontmatter incomplete: %+v", path, f)
		}
		if len(f.Description) > 160 {
			t.Errorf("%s: description is %d chars", path, len(f.Description))
		}
		for _, l := range f.Links {
			if l.Description == "" {
				t.Errorf("%s: link to %s has no description", path, l.To)
			}
		}
		if strings.Count(body, "\n# ") != 1 {
			t.Errorf("%s: want exactly one h1 (a column-0 comment in an example reads as one)", path)
		}
	}
}

// The pipeline page's rows come from the schema: every property of the
// top level and of a step is a row, with the schema's description verbatim.
func TestPipelineYAMLRowsComeFromSchema(t *testing.T) {
	pages, repo := singles(t)
	page := pages["reference/pipeline-yaml.md"]
	var schema struct {
		Properties  map[string]struct{ Description string } `json:"properties"`
		Definitions struct {
			Step struct {
				Properties map[string]struct{ Description string } `json:"properties"`
			} `json:"step"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(repo["spec/schemas/pipeline.schema.json"], &schema); err != nil {
		t.Fatal(err)
	}
	for _, props := range []map[string]struct{ Description string }{schema.Properties, schema.Definitions.Step.Properties} {
		for name, p := range props {
			if !strings.Contains(page, "| `"+name+"` |") {
				t.Errorf("no row for %s", name)
			}
			if p.Description != "" && !strings.Contains(page, cell(prose(p.Description))) {
				t.Errorf("%s: description not verbatim", name)
			}
		}
	}
	for _, want := range []string{
		"| `name` | string | yes |",
		"| `use` | string | one of `use`, `group` |",
		"| `suppress.within` | string matching `^[0-9]+d$` | yes | — |",
		"| `provides.FIELD.canonical` | boolean | no |",
		"| `source` | step (see Step keys) | yes |",
		"use: demo/enrich",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("pipeline-yaml lacks %q", want)
		}
	}
	if strings.Contains(page, "See it run's second file") {
		t.Error("the example kept hello.yaml's header comment")
	}
}

// The binding page splits the schema into the manifest surface (keys a
// process manifest shares) and blocks, and lists the process-manifest keys
// a binding does not take.
func TestBindingManifestBlocks(t *testing.T) {
	pages, _ := singles(t)
	page := pages["reference/binding-manifest.md"]
	for _, want := range []string{
		"## Manifest surface",
		"| `relation.from` | one of `record`, `parent` | yes | — | same key |",
		"| `version` | integer | yes | Process manifest: Manifest version; part of the provenance string. | same key |",
		"## Process manifest keys a binding does not take",
		"| `emits_key_fields` |",
		"## Request\n\nThe `request` block, required.",
		"| `extract.fields.FIELD.each` | string | no |",
		"| `errors.STATUS.verdict` | one of `fail_record`, `fail_run`, `retry`, `skip` | yes |",
		"id: apollo/search",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("binding-manifest lacks %q", want)
		}
	}
}

// The wire page has one overview row and one section per message schema,
// in the README's order, and its example is the csv/source stream only.
func TestWireProtocolPage(t *testing.T) {
	pages, repo := singles(t)
	page := pages["reference/wire-protocol.md"]
	files := repoFiles(repo).glob("spec/schemas/msg-*.schema.json")
	for _, f := range files {
		o, err := decodeObj(repo[f])
		if err != nil {
			t.Fatal(err)
		}
		name, dir := schemaHeadline(o.str("title"))
		if !strings.Contains(page, "| `"+name+"` | "+dir+" |") {
			t.Errorf("no overview row for %s (%s)", name, dir)
		}
		if !strings.Contains(page, "\n## "+name+" ("+dir+")\n") {
			t.Errorf("no section for %s (%s)", name, dir)
		}
	}
	if open, end := strings.Index(page, "## OPEN (runner"), strings.Index(page, "## END (both"); open < 0 || end < open {
		t.Error("messages are not in the README's order")
	}
	if !strings.Contains(page, "| `key.identity_key` | string | yes |") {
		t.Error("the key definition was not inlined")
	}
	ex := page[strings.Index(page, "## Example"):]
	if !strings.Contains(ex, `"adapter":"csv/source"`) || strings.Contains(ex, `"stream":"enrich"`) {
		t.Error("example is not the csv/source stream alone")
	}
	for _, stale := range []string{"mock_score", "mock_note"} {
		if strings.Contains(page, stale) {
			t.Errorf("page renders %s (issue #171)", stale)
		}
	}
}

// Bundles, plugin skills, and the conformance kit take their rows from the
// bundle schema, each SKILL.md, and the conformance tests.
func TestBundlesPluginConformanceRows(t *testing.T) {
	pages, _ := singles(t)
	for path, wants := range map[string][]string{
		"reference/bundles.md": {
			"| `bundle_format_version` | integer | yes | Format version of the bundle layout itself. |",
			"| `contents` | map of string matching `^[0-9a-f]{64}$` | yes |",
			"From `bundles/email-waterfall/manifest.json`",
		},
		"reference/plugin-skills.md": {
			"| `/gtme:create-pipeline` |",
			"| `/gtme:analyze` |",
			"(/start/for-agents) has the plugin install.",
		},
		"reference/conformance.md": {
			"| `spec/wire/*.ndjson` (1: basic-run.ndjson) | Golden transcripts recorded from the real adapters | `TestWireTranscriptValidatesAgainstSchemas`, `TestTranscriptRoundTripsThroughProtocol` |",
			"| `spec/acceptance/*.yaml` (8:",
			"| `TestShippedBindingsParse` | `binding_test.go` | `spec/bindings/*/binding.yaml`, `spec/bindings/*/fixtures/*`",
			"| `TestEmbeddedRegistryMatchesSpecDir` | `registry_test.go` | `spec/fields/*.json` | The embedded registry is the same files",
		},
	} {
		for _, want := range wants {
			if !strings.Contains(pages[path], want) {
				t.Errorf("%s lacks %q", path, want)
			}
		}
	}
}

// The flattener keeps schema order, resolves local refs, names map keys
// with a placeholder, and reads an anyOf of required lists as "one of".
func TestFlattener(t *testing.T) {
	schema, err := decodeObj([]byte(`{
	  "anyOf": [{"required": ["b"]}, {"required": ["a"]}],
	  "required": ["z"],
	  "properties": {
	    "z": {"type": "string", "description": "first"},
	    "b": {"$ref": "#/definitions/k"},
	    "a": {"type": "object", "additionalProperties": {"type": "object", "properties": {"x": {"type": "integer"}}}},
	    "m": {"type": "object", "additionalProperties": {"type": "string"}}
	  },
	  "definitions": {"k": {"type": "object", "description": "a key", "properties": {"id": {"type": "string"}}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	f := &flattener{root: schema}
	got := f.rows(schema, "")
	want := []keyRow{
		{"z", "string", "yes", "first"},
		{"b", "object", "one of `b`, `a`", "a key"},
		{"b.id", "string", "no", ""},
		{"a", "object", "one of `b`, `a`", ""},
		{"a.KEY", "object", "no", ""},
		{"a.KEY.x", "integer", "no", ""},
		{"m", "map of string", "no", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The outline rewrite marks each single page generated.
func TestRewriteOutlineMarksSinglesGenerated(t *testing.T) {
	outline := "collections:\n  guides:\n    - { slug: bundles, name: Share, status: planned }\n  reference:\n"
	for _, slug := range singleSlugs {
		outline += "    - { slug: " + slug + ", name: X, status: planned }\n"
	}
	outline += "pages:\n  - { slug: glossary, name: Glossary, status: generated }\nterms:\n  x: /y\n"
	s := &site{agent: &agentDoc{}}
	// No cli/adapters/fields/ledger entries in this outline: give the
	// rewrite nothing to add children to.
	got, err := rewriteOutline(outline, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range singleSlugs {
		if !strings.Contains(got, "- { slug: "+slug+", name: X, status: generated }") {
			t.Errorf("%s not marked generated:\n%s", slug, got)
		}
	}
	if !strings.Contains(got, "slug: bundles, name: Share, status: planned") {
		t.Error("the guides entry named bundles was rewritten too")
	}
}
