package docsgen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// `gtme help --agent`

type agentDoc struct {
	Verbs     []agentVerb    `json:"verbs"`
	SQLSteps  []agentVerb    `json:"sql_steps"`
	Examples  []agentExample `json:"examples"`
	Ledger    agentLedger    `json:"ledger"`
	ExitCodes []agentExit    `json:"exit_codes"`
}

type agentVerb struct {
	Usage string `json:"usage"`
	Does  string `json:"does"`
}

type agentExample struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Yaml        string `json:"yaml"`
}

type agentLedger struct {
	Note    string           `json:"note"`
	Objects []agentLedgerObj `json:"objects"`
	Shapes  []agentShape     `json:"query_shapes"`
}

type agentLedgerObj struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Columns []string `json:"columns"`
	Does    string   `json:"does"`
}

type agentShape struct {
	Name string `json:"name"`
	Use  string `json:"use"`
	SQL  string `json:"sql"`
}

type agentExit struct {
	Code  int    `json:"code"`
	Means string `json:"means"`
}

func parseAgent(b []byte) (*agentDoc, error) {
	var d agentDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	if len(d.Verbs) == 0 {
		return nil, fmt.Errorf("no verbs")
	}
	if len(d.ExitCodes) == 0 {
		return nil, fmt.Errorf("no exit_codes; the binary predates the section")
	}
	return &d, nil
}

// verb groups the usage forms that share a second word: `gtme show ...`.
type verb struct {
	Name  string
	Forms []agentVerb
}

func groupVerbs(vs []agentVerb) []verb {
	var out []verb
	idx := map[string]int{}
	for _, v := range vs {
		f := strings.Fields(v.Usage)
		if len(f) < 2 || f[0] != "gtme" {
			continue
		}
		name := f[1]
		if i, ok := idx[name]; ok {
			out[i].Forms = append(out[i].Forms, v)
			continue
		}
		idx[name] = len(out)
		out = append(out, verb{Name: name, Forms: []agentVerb{v}})
	}
	return out
}

// flag is one `--option` a verb's usage names, with the clause of the
// `does` string that mentions it.
type flag struct {
	Name string
	Arg  string
	Form string
	Does string
}

var flagRe = regexp.MustCompile(`(--[a-z][a-z-]*)(?:[ =]([^\s\[\]|]+(?:\|[^\s\[\]|]+)*))?`)

func flagsOf(v verb) []flag {
	var out []flag
	seen := map[string]bool{}
	for _, f := range v.Forms {
		for _, m := range flagRe.FindAllStringSubmatch(f.Usage, -1) {
			name, arg := m[1], m[2]
			if i := strings.Index(arg, "|--"); i >= 0 {
				arg = arg[:i]
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, flag{Name: name, Arg: arg, Form: f.Usage, Does: clauseAbout(f.Does, name)})
		}
	}
	return out
}

// clauses splits a `does` string at the joints that separate one point from
// the next: `;`, an em dash, a sentence end, and a comma before a flag —
// outside parentheses, so a parenthetical stays with its clause.
func clauses(does string) []string {
	var out []string
	depth, start := 0, 0
	cut := func(i, skip int) {
		if c := strings.TrimSpace(does[start:i]); c != "" {
			out = append(out, c)
		}
		start = i + skip
	}
	for i := 0; i < len(does); i++ {
		switch {
		case does[i] == '(':
			depth++
		case does[i] == ')':
			depth--
		case depth > 0:
		case strings.HasPrefix(does[i:], "; "):
			cut(i, 2)
		case strings.HasPrefix(does[i:], " — "):
			cut(i, len(" — "))
		case strings.HasPrefix(does[i:], ". "):
			cut(i+1, 1)
		case strings.HasPrefix(does[i:], ", --"):
			cut(i, 2)
		}
	}
	cut(len(does), 0)
	return out
}

// clauseAbout returns the first clause of does that names the flag.
func clauseAbout(does, flag string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(flag) + `(?:[^a-z-]|$)`)
	for _, c := range clauses(does) {
		if re.MatchString(c) {
			return strings.TrimSuffix(c, ",")
		}
	}
	return ""
}

var optionalRe = regexp.MustCompile(`\s*\[[^\[\]]*\]`)

// required strips every optional group from a usage form.
func required(usage string) string {
	s := usage
	for {
		n := optionalRe.ReplaceAllString(s, "")
		if n == s {
			break
		}
		s = n
	}
	return strings.Join(strings.Fields(s), " ")
}

// ---------------------------------------------------------------------------
// docs/_adapters.json

type manifest struct {
	ID                  string          `json:"id"`
	Version             int             `json:"version"`
	Role                string          `json:"role"`
	EntityType          string          `json:"entity_type"`
	From                string          `json:"from"`
	Relation            json.RawMessage `json:"relation"`
	Needs               json.RawMessage `json:"needs"`
	Provides            json.RawMessage `json:"provides"`
	Credentials         []string        `json:"credentials"`
	CredentialsOptional []string        `json:"credentials_optional"`
	ConfigSchema        json.RawMessage `json:"config_schema"`
	FreshnessDays       int             `json:"freshness_days"`
	CostEstimateUSD     *float64        `json:"cost_estimate_usd"`
	Attests             bool            `json:"attests"`
}

func parseAdapters(b []byte) ([]manifest, error) {
	var doc struct {
		Adapters []manifest `json:"adapters"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Adapters) == 0 {
		return nil, fmt.Errorf("no adapters")
	}
	sort.Slice(doc.Adapters, func(i, j int) bool { return doc.Adapters[i].ID < doc.Adapters[j].ID })
	return doc.Adapters, nil
}

// roleOrder is the order a pipeline meets the roles in (SPEC §6).
var roleOrder = []string{"source", "traverse", "enrich", "verify", "filter", "compose", "review", "deliver"}

func roleRank(role string) int {
	for i, r := range roleOrder {
		if r == role {
			return i
		}
	}
	return len(roleOrder)
}

// schemaProp is one property of a JSON-schema object, flattened one level.
type schemaProp struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// schemaProps flattens a schema's properties (and one nested level, as
// `parent.child`) into rows, sorted by name.
func schemaProps(raw json.RawMessage) ([]schemaProp, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	closed, _ := s["additionalProperties"].(bool)
	closedSet := false
	if _, ok := s["additionalProperties"]; ok {
		closedSet = !closed
	}
	return flattenProps(s, ""), closedSet
}

func flattenProps(s map[string]any, prefix string) []schemaProp {
	props, _ := s["properties"].(map[string]any)
	req := map[string]bool{}
	if r, ok := s["required"].([]any); ok {
		for _, x := range r {
			if n, ok := x.(string); ok {
				req[n] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []schemaProp
	for _, n := range names {
		p, _ := props[n].(map[string]any)
		out = append(out, schemaProp{
			Name:        prefix + n,
			Type:        schemaType(p),
			Required:    req[n],
			Description: fmt.Sprint(orEmpty(p["description"])),
		})
		if t, _ := p["type"].(string); t == "object" {
			if _, has := p["properties"]; has {
				out = append(out, flattenProps(p, prefix+n+".")...)
			}
		}
	}
	return out
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// schemaType renders a property's type, enum, and array item type.
func schemaType(p map[string]any) string {
	t, _ := p["type"].(string)
	if e, ok := p["enum"].([]any); ok {
		vals := make([]string, len(e))
		for i, x := range e {
			vals[i] = fmt.Sprint(x)
		}
		return "one of " + strings.Join(vals, ", ")
	}
	if t == "array" {
		if items, ok := p["items"].(map[string]any); ok {
			if it, _ := items["type"].(string); it != "" {
				return "array of " + it
			}
		}
	}
	if t == "" {
		return "any"
	}
	return t
}

// needsSummary says what an adapter needs in a few words.
func needsSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "nothing"
	}
	var s any
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	if str, ok := s.(string); ok {
		if str == "dynamic" {
			return "the step's `uses:` fields"
		}
		return str
	}
	m, _ := s.(map[string]any)
	var parts []string
	if d, _ := m["dynamic"].(bool); d {
		parts = append(parts, "the step's `uses:` fields")
	}
	if r, ok := m["required"].([]any); ok && len(r) > 0 {
		parts = append(parts, codeList(r))
	}
	if alternatives, ok := m["anyOf"].([]any); ok {
		var alts []string
		for _, a := range alternatives {
			am, _ := a.(map[string]any)
			if r, ok := am["required"].([]any); ok {
				alts = append(alts, codeList(r))
			}
		}
		if len(alts) > 0 {
			parts = append(parts, "any of "+strings.Join(alts, " or "))
		}
	}
	if len(parts) == 0 {
		if props, ok := m["properties"].(map[string]any); ok && len(props) > 0 {
			return "optionally " + codeList(keysOf(props))
		}
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

// providesNames lists a provides schema's field names, sorted.
func providesNames(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	props, _ := m["properties"].(map[string]any)
	names := keysOf(props)
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n.(string)
	}
	return out
}

func keysOf(m map[string]any) []any {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = n
	}
	return out
}

func codeList(xs []any) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = code(fmt.Sprint(x))
	}
	return strings.Join(parts, ", ")
}

// pretty re-indents raw JSON for a code block.
func pretty(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// ---------------------------------------------------------------------------
// spec/fields/<type>.json

type fieldType struct {
	EntityType  string          `json:"entity_type"`
	Version     int             `json:"version"`
	Kind        string          `json:"kind"`
	Description string          `json:"description"`
	Identity    []any           `json:"identity"`
	IdentityRaw json.RawMessage `json:"-"`
	Fields      []field         `json:"fields"`
}

type field struct {
	Name          string     `json:"name"`
	Tier          string     `json:"tier"`
	Type          string     `json:"type"`
	Format        string     `json:"format"`
	Normalization string     `json:"normalization"`
	Description   string     `json:"description"`
	Example       any        `json:"example"`
	Enum          []any      `json:"enum"`
	Reserved      bool       `json:"reserved"`
	ItemsType     string     `json:"items_type"`
	Reference     *reference `json:"reference"`
}

type reference struct {
	Type     string   `json:"type"`
	Relation string   `json:"relation"`
	Fields   []string `json:"fields"`
}

func parseFieldFiles(files map[string][]byte) ([]fieldType, error) {
	var out []fieldType
	for name, b := range files {
		var t fieldType
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		var raw struct {
			Identity json.RawMessage `json:"identity"`
		}
		_ = json.Unmarshal(b, &raw)
		t.IdentityRaw = raw.Identity
		if t.EntityType == "" {
			return nil, fmt.Errorf("%s: no entity_type", name)
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityType < out[j].EntityType })
	return out, nil
}

// identityExpr renders one identity tier in words: the field, or the hash
// expression, with its prefix.
func identityExpr(tier any) (from, looks string) {
	m, _ := tier.(map[string]any)
	prefix, _ := m["prefix"].(string)
	if f, ok := m["field"].(string); ok {
		if prefix != "" {
			return code(f), "the value prefixed " + code(prefix)
		}
		return code(f), "the normalized value"
	}
	if h, ok := m["hash"].([]any); ok {
		parts := make([]string, len(h))
		for i, p := range h {
			parts[i] = hashPart(p)
		}
		return strings.Join(parts, " and "), "a SHA-256 of those, prefixed " + code(prefix)
	}
	return fmt.Sprint(tier), ""
}

func hashPart(p any) string {
	switch v := p.(type) {
	case string:
		return code(v)
	case map[string]any:
		if alts, ok := v["any"].([]any); ok {
			parts := make([]string, len(alts))
			for i, a := range alts {
				parts[i] = hashPart(a)
			}
			return "(" + strings.Join(parts, " or ") + ")"
		}
		if j, ok := v["join"].([]any); ok {
			parts := make([]string, len(j))
			for i, a := range j {
				parts[i] = hashPart(a)
			}
			return strings.Join(parts, " + ")
		}
	}
	return fmt.Sprint(p)
}
