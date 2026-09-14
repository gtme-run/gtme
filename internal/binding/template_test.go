package binding

import (
	"strings"
	"testing"
)

// The engine's template rules on the shared parser (ADR-057, M31): a
// single-placeholder leaf substitutes the typed value, an interpolated leaf
// omits itself when any placeholder is empty, `| default:` is the fallback,
// and the `$variables` splice still excludes individually routed variables.
func TestTemplateLeavesOnTheOneDialect(t *testing.T) {
	c := tmplContext{
		Config:    map[string]any{"base_url": "https://x", "limit": float64(25), "main_only": true},
		Record:    map[string]any{"email": "a@x.com", "linkedin_url": "", "linkedin_internal_url": "in/a", "web.homepage": "h"},
		Variables: map[string]string{"first_name": "Ann", "personalization": ""},
		Session:   "s1",
	}
	// Typed leaves.
	if v, ok := c.resolveValue("{{config.limit}}"); !ok || v != float64(25) {
		t.Errorf("typed number leaf = %v %v", v, ok)
	}
	if v, ok := c.resolveValue("{{ config.main_only }}"); !ok || v != true {
		t.Errorf("typed bool leaf = %v %v", v, ok)
	}
	// Interpolation, and omission on an empty placeholder.
	if s := c.renderString("{{config.base_url}}/u/{{record.email}}?s={{ session }}"); s != "https://x/u/a@x.com?s=s1" {
		t.Errorf("interpolated = %q", s)
	}
	if s := c.renderString("{{config.base_url}}/{{record.linkedin_url}}"); s != "" {
		t.Errorf("a leaf with an empty placeholder should omit itself, got %q", s)
	}
	// The fallback is Liquid's default filter, chained.
	if s := c.renderString("{{ record.linkedin_url | default: record.linkedin_internal_url }}"); s != "in/a" {
		t.Errorf("default fallback = %q", s)
	}
	if s := c.renderString("{{ record.nope | default: record.linkedin_url | default: 'none' }}"); s != "none" {
		t.Errorf("chained default = %q", s)
	}
	// Namespaced fields read flat and nested.
	if s := c.renderString(`{{ record["web.homepage"] }}-{{ record.web.homepage }}`); s != "h-h" {
		t.Errorf("namespaced = %q", s)
	}
	// Body: the splice excludes what a placeholder routed individually, and
	// omits empty leaves.
	body := map[string]any{
		"email":      "{{record.email}}",
		"first_name": "{{ variables.first_name | upcase }}",
		"missing":    "{{record.linkedin_url}}",
		"custom":     map[string]any{"$variables": true},
	}
	out, _ := c.resolveBody(body).(map[string]any)
	if out["email"] != "a@x.com" || out["first_name"] != "ANN" {
		t.Errorf("body = %v", out)
	}
	if _, present := out["missing"]; present {
		t.Errorf("an empty leaf should be omitted: %v", out)
	}
	if _, present := out["custom"]; present {
		t.Errorf("the splice had nothing left to add (first_name routed, personalization empty): %v", out)
	}
}

func TestVerifyRefusesTheRetiredAlternativesAndTheRestOfTheDialect(t *testing.T) {
	base := `id: x/lookup
version: 1
role: enrich
entity_type: person
needs:
  type: object
  required: [email]
  properties:
    email: { type: string }
provides:
  type: object
  additionalProperties: false
  properties:
    title: { type: string }
request:
  method: GET
  url: URL
extract:
  records: person
  fields:
    title: title
`
	cases := []struct{ url, want string }{
		{`"{{record.linkedin_url|record.linkedin_internal_url}}"`, "write `{{ record.linkedin_url | default: record.linkedin_internal_url }}`"},
		{`"{{ record.email | reverse }}"`, `filter "reverse" is not in the dialect`},
		{`"{% if record.email %}x{% endif %}"`, "a request template is an object"},
		{`"{{ nope.email }}"`, "is not a variable here"},
	}
	for _, tc := range cases {
		_, err := Parse([]byte(strings.Replace(base, "url: URL", "url: "+tc.url, 1)))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "request.url") {
			t.Errorf("%s: err = %v, want one naming request.url and containing %q", tc.url, err, tc.want)
		}
	}
	ok := strings.Replace(base, "url: URL", `url: "{{ config.base_url | default: 'https://x' }}/{{ record.email | downcase }}"`, 1)
	if _, err := Parse([]byte(ok)); err != nil {
		t.Errorf("a leaf in the dialect should parse: %v", err)
	}
}
