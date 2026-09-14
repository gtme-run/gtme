package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInlineFileAndForms(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "judge.md"), []byte("Keep {{ config.who }}.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, file, present, err := Load(map[string]any{"template": "inline"}, dir)
	if err != nil || !present || file != "" || src != "inline" {
		t.Fatalf("inline: %q %q %v %v", src, file, present, err)
	}
	src, file, present, err = Load(map[string]any{"template": map[string]any{"file": "judge.md"}}, dir)
	if err != nil || !present || file != "judge.md" || src != "Keep {{ config.who }}.\n" {
		t.Fatalf("file: %q %q %v %v", src, file, present, err)
	}
	if _, _, present, _ := Load(map[string]any{}, dir); present {
		t.Fatal("absent key reported present")
	}
	if _, _, _, err := Load(map[string]any{"template": map[string]any{"file": "nope.md"}}, dir); err == nil || !strings.Contains(err.Error(), "nope.md") {
		t.Fatalf("missing file: %v", err)
	}
	if _, _, _, err := Load(map[string]any{"template": map[string]any{"path": "x"}}, dir); err == nil || !strings.Contains(err.Error(), "{file: <path>}") {
		t.Fatalf("bad form: %v", err)
	}
	if _, _, _, err := Load(map[string]any{"template": 3}, dir); err == nil {
		t.Fatal("number accepted")
	}
	if _, _, _, err := Load(map[string]any{"template": map[string]any{"file": "judge.md"}}, ""); err == nil {
		t.Fatal("file form with no dir accepted")
	}
}

func TestCheckScopeAndDialect(t *testing.T) {
	cfg := map[string]any{"template": "x", "persona": "cto", "batch_size": 25}
	uses := []string{"first_name", "recent_posts", "review.first_line"}
	cases := []struct {
		name   string
		src    string
		scope  Scope
		want   string // a substring of the one problem, or "" for none
		config []string
		record []string
	}{
		{"plain text", "Keep decision makers.", Batch, "", nil, nil},
		{"config on a batch step", "Speak to a {{ config.persona }}.", Batch, "", []string{"persona"}, nil},
		{"record on a batch step", "Hi {{ record.first_name }}", Batch, "not in scope for a batch step", nil, nil},
		{"record on a per-record step", "Hi {{ record.first_name | default: 'there' }}", Record, "", nil, []string{"first_name"}},
		{"nested namespaced field", "{{ record.review.first_line }}", Record, "", nil, []string{"review.first_line"}},
		{"bracket namespaced field", `{{ record["review.first_line"] }}`, Record, "", nil, []string{"review.first_line"}},
		{"descends into a declared field", "{{ record.recent_posts.first.title }}", Record, "", nil, []string{"recent_posts"}},
		{"undeclared field", "{{ record.title }}", Record, "not in this step's uses:", nil, nil},
		{"unknown config key", "{{ config.nope }}", Record, "names no key", nil, nil},
		{"the template itself is not config", "{{ config.template }}", Batch, "names no key", nil, nil},
		{"bare name", "{{ first_name }}", Record, "is not a variable here", nil, nil},
		{"loop variable", "{% for p in record.recent_posts limit:1 %}{{ p.title }} {{ forloop.index }}{% endfor %}", Record, "", nil, []string{"recent_posts"}},
		{"loop over undeclared", "{% for p in record.posts %}{{ p }}{% endfor %}", Record, "record.posts is not", nil, nil},
		{"if with comparison", "{% if record.first_name == 'x' and config.persona contains 'c' %}y{% elsif record.recent_posts.size > 1 %}z{% else %}w{% endif %}", Record, "", []string{"persona"}, []string{"first_name", "recent_posts"}},
		{"case when", "{% case record.first_name %}{% when 'a' %}1{% else %}2{% endcase %}", Record, "", nil, []string{"first_name"}},
		{"unless", "{% unless record.first_name %}anon{% endunless %}", Record, "", nil, []string{"first_name"}},
		{"comment body is not scanned", "{% comment %}{{ nope }}{% endcomment %}ok", Record, "", nil, nil},
		{"raw body is not scanned", "{% raw %}{{ nope }}{% endraw %}", Record, "", nil, nil},
		{"unlisted filter", "{{ record.first_name | reverse }}", Record, `filter "reverse" is not in the dialect`, nil, []string{"first_name"}},
		{"listed filters chain", "{{ record.first_name | strip | upcase | truncate: 3, '' }}", Record, "", nil, []string{"first_name"}},
		{"include", "{% include 'x' %}", Record, "{% include %} is not in the dialect", nil, nil},
		{"assign", "{% assign x = 1 %}", Record, "{% assign %} is not in the dialect", nil, nil},
		{"capture", "{% capture x %}y{% endcapture %}", Record, "{% capture %} is not in the dialect", nil, nil},
		{"parse error", "{% if %}", Record, "unterminated", nil, nil},
		{"string literal is not a reference", "{{ 'record.nope' }}", Batch, "", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, problems := Check(tc.src, tc.scope, uses, "", cfg)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("problems = %v, want none", problems)
				}
			} else {
				if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
					t.Fatalf("problems = %v, want one containing %q", problems, tc.want)
				}
			}
			if strings.Join(got.ConfigKeys, ",") != strings.Join(tc.config, ",") {
				t.Errorf("config keys = %v, want %v", got.ConfigKeys, tc.config)
			}
			if strings.Join(got.RecordFields, ",") != strings.Join(tc.record, ",") {
				t.Errorf("record fields = %v, want %v", got.RecordFields, tc.record)
			}
		})
	}
}

func TestCheckOfCountsAsDeclared(t *testing.T) {
	got, problems := Check("{{ record.draft }}", Record, nil, "draft", map[string]any{})
	if len(problems) != 0 || strings.Join(got.RecordFields, ",") != "draft" {
		t.Fatalf("of: %v %v", got, problems)
	}
}

func TestRenderIsLenientAndNestsNamespaces(t *testing.T) {
	rec := map[string]any{
		"first_name":        "",
		"recent_posts":      []any{map[string]any{"title": "A"}, map[string]any{"title": "B"}},
		"review.first_line": "Hi.",
	}
	src := "Hi {{ record.first_name | default: 'there' }}: {% for p in record.recent_posts limit:1 %}{{ p.title }}{% endfor %} / {{ record.review.first_line }} / {{ record.missing | default: 'none' }} / {{ config.persona | upcase }}"
	out, err := Render(src, map[string]any{"persona": "cto", "template": src}, rec)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hi there: A / Hi. / none / CTO" {
		t.Fatalf("out = %q", out)
	}
	// A batch render has no record binding at all.
	if out, err := Render("{{ config.persona }}", map[string]any{"persona": "x"}, nil); err != nil || out != "x" {
		t.Fatalf("batch: %q %v", out, err)
	}
}

func TestNestKeepsFlatKeyOnCollision(t *testing.T) {
	out := nest(map[string]any{"web": "flat", "web.homepage": "h"})
	if out["web"] != "flat" || out["web.homepage"] != "h" {
		t.Fatalf("nest = %v", out)
	}
}
