package binding

import (
	"reflect"
	"strings"
	"testing"
)

const eachBinding = `
id: test/each
version: 1
role: enrich
entity_type: person
needs: {type: object, required: [linkedin_url], properties: {linkedin_url: {type: string}}}
provides: {type: object, properties: {lines: {type: array, items: {type: string}}}}
config_schema:
  type: object
  additionalProperties: false
  properties:
    n: {type: integer, default: 2}
request: {method: GET, url: "https://x/p"}
extract:
  records: "."
  fields:
    lines:
      each: "items|item"
      template: "{% if item.a %}{{ item.a | strip }}{% else %}{{ item.b }}{% endif %}"
      limit: "{{ config.n }}"
`

// The each: form (SPEC §10a, ADR-059): the template renders once per
// element, an empty render is dropped, the limit counts kept renders and
// may be a config reference, and the field provides a string array.
func TestEachRendersOncePerElementDropsEmptiesAndStopsAtTheLimit(t *testing.T) {
	b, err := Parse([]byte(eachBinding))
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{B: b}
	doc := map[string]any{"items": []any{
		map[string]any{"a": "  first "},
		map[string]any{"a": "", "b": ""},
		map[string]any{"b": "second"},
		map[string]any{"a": "third"},
	}}
	got := e.extractFields(doc, nil, e.configWithDefaults(nil))
	if want := []any{"first", "second"}; !reflect.DeepEqual(got["lines"], want) {
		t.Errorf("default limit 2: lines = %#v, want %#v", got["lines"], want)
	}
	got = e.extractFields(doc, nil, e.configWithDefaults(map[string]any{"n": float64(5)}))
	if want := []any{"first", "second", "third"}; !reflect.DeepEqual(got["lines"], want) {
		t.Errorf("limit 5: lines = %#v, want %#v", got["lines"], want)
	}
	// The | alternative path: the vendor's other spelling of the array.
	got = e.extractFields(map[string]any{"item": []any{map[string]any{"a": "x"}}}, nil, e.configWithDefaults(nil))
	if want := []any{"x"}; !reflect.DeepEqual(got["lines"], want) {
		t.Errorf("alternative path: lines = %#v", got["lines"])
	}
	// Nothing renders: the field is absent, not an empty array.
	got = e.extractFields(map[string]any{"items": []any{map[string]any{}}}, nil, e.configWithDefaults(nil))
	if _, present := got["lines"]; present {
		t.Errorf("an all-empty render should omit the field: %#v", got)
	}
}

// A literal limit, and no limit at all.
func TestEachLimitLiteralAndUnbounded(t *testing.T) {
	for _, tc := range []struct {
		limit string
		want  int
	}{{"limit: 1", 1}, {"", 3}} {
		src := strings.Replace(eachBinding, `limit: "{{ config.n }}"`, tc.limit, 1)
		b, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%q: %v", tc.limit, err)
		}
		e := &Engine{B: b}
		doc := map[string]any{"items": []any{map[string]any{"a": "1"}, map[string]any{"a": "2"}, map[string]any{"a": "3"}}}
		got, _ := e.extractFields(doc, nil, e.configWithDefaults(nil))["lines"].([]any)
		if len(got) != tc.want {
			t.Errorf("%q: got %d lines, want %d", tc.limit, len(got), tc.want)
		}
	}
}

// Parse holds the form to its contract: a template is required, it may use
// the dialect's tags but reads item.* only, each: does not mix with paths
// or a transform, and a limit is a positive number or a config reference.
func TestEachParseRefusals(t *testing.T) {
	for _, tc := range []struct{ name, from, to, want string }{
		{"no template", `      template: "{% if item.a %}{{ item.a | strip }}{% else %}{{ item.b }}{% endif %}"` + "\n", "", "each: needs a template"},
		{"record ref", `{{ item.b }}`, `{{ record.linkedin_url }}`, "item"},
		{"unknown filter", `{{ item.a | strip }}`, `{{ item.a | reverse }}`, "not in the dialect"},
		{"mixed path", `each: "items|item"`, "each: \"items|item\"\n      path: items", "does not mix"},
		{"zero limit", `limit: "{{ config.n }}"`, `limit: 0`, "limit"},
		{"record limit", `limit: "{{ config.n }}"`, `limit: "{{ record.n }}"`, "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(eachBinding, tc.from, tc.to, 1)
			if src == eachBinding {
				t.Fatalf("replacement %q did not apply", tc.from)
			}
			_, err := Parse([]byte(src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
