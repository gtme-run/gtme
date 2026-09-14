// Package template is the one loader, parser, reference check and renderer
// behind `template:` (SPEC §7, §9, §10 item 10; ADR-057): the operator's
// text on every participant step. The dialect is a bounded Liquid — a fixed
// tag set, a fixed filter allowlist, no state, no includes — and what a
// template may reference is set by the step's role: a batch step (ai/*)
// sees config.* only, a per-record step (text/*, human/*, agent/*) sees
// record.* too, limited to its uses: and of:. Plan enforces both by
// walking the parsed template; rendering is lenient on purpose, so a
// record that lacks a declared field still renders (`| default:` is the
// operator's tool for that), exactly as on_missing: run dispatches it.
package template

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/osteele/liquid"
	"github.com/osteele/liquid/expressions"
	"github.com/osteele/liquid/filters"
	"github.com/osteele/liquid/render"
)

// Key is the with: key (SPEC §9).
const Key = "template"

// Scope is what a template may reference (ADR-057).
type Scope int

const (
	// Batch renders once per step over config.* only — an ai/* step, whose
	// records arrive as ADR-035's fenced payload, never through the text.
	Batch Scope = iota
	// Record renders once per record over config.* and record.*.
	Record
	// Binding is a request template leaf (SPEC §10a, M31): objects only —
	// no block tags in a URL — over record.*, config.*, variables.* and
	// session, none of them declared-list checked (the binding's needs
	// and config_schema are its own contract).
	Binding
)

// Tags is the closed tag set (SPEC §10 item 10). Clause tags (else, elsif,
// when) are listed with their blocks.
var Tags = map[string]bool{
	"if": true, "elsif": true, "else": true, "unless": true, "case": true, "when": true,
	"for": true, "comment": true, "raw": true,
}

// Filters is the closed filter allowlist (SPEC §10 item 10).
var Filters = map[string]bool{
	"default": true, "truncate": true, "truncatewords": true, "size": true, "first": true, "last": true,
	"join": true, "upcase": true, "downcase": true, "capitalize": true, "strip": true, "date": true,
}

// Load reads a step's template: the string as written, or the file the
// `{file: <path>}` form names, relative to dir. It returns the source, the
// file reference (empty for an inline template), and whether the key was
// present at all. A malformed form or an unreadable file is an error —
// at plan time, a plan error (SPEC §7).
func Load(with map[string]any, dir string) (source, file string, present bool, err error) {
	v, ok := with[Key]
	if !ok {
		return "", "", false, nil
	}
	switch t := v.(type) {
	case string:
		return t, "", true, nil
	case map[string]any:
		f, _ := t["file"].(string)
		if strings.TrimSpace(f) == "" || len(t) != 1 {
			return "", "", true, fmt.Errorf("template: the file form is {file: <path>} (SPEC §9, ADR-057)")
		}
		if dir == "" {
			return "", f, true, fmt.Errorf("template: {file: %s} resolves relative to the pipeline file, which is not known here", f)
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return "", f, true, fmt.Errorf("template: reading %s: %w", f, err)
		}
		return string(body), f, true, nil
	default:
		return "", "", true, fmt.Errorf("template: must be a string or {file: <path>} (got %T)", v)
	}
}

var (
	engineOnce sync.Once
	engine     *liquid.Engine
)

// eng is the shared engine: the standard grammar, with the bounded dialect
// enforced by Check at plan time (SPEC §7) rather than by the engine —
// nothing renders that plan did not pass.
func eng() *liquid.Engine {
	engineOnce.Do(func() { engine = liquid.NewEngine() })
	return engine
}

// Checked is what plan learned about a template: the config keys and the
// record fields it references (sorted, deduplicated), for the judgment
// signature (ADR-039) and the surface.
type Checked struct {
	ConfigKeys   []string
	RecordFields []string
}

// Check parses a template and applies the dialect and the scope (SPEC §7):
// a parse error, an unknown tag or filter, a record.* reference on a batch
// step, a record.* reference outside uses:/of:, a config.* key absent from
// the step's with:, or a bare name that is neither, each become one
// problem. Problems are returned together so the operator fixes the
// template in one pass.
func Check(source string, scope Scope, uses []string, of string, config map[string]any) (Checked, []string) {
	var out Checked
	if msg := retiredAlternatives(source); msg != "" {
		return out, []string{msg}
	}
	tpl, err := eng().ParseString(source)
	if err != nil {
		return out, []string{"template: " + cleanErr(err)}
	}
	declared := append([]string(nil), uses...)
	if of != "" {
		declared = append(declared, of)
	}
	s := &scanner{scope: scope, declared: declared, config: config, seenConfig: map[string]bool{}, seenRecord: map[string]bool{}, seenProblem: map[string]bool{}}
	s.walk(tpl.GetRoot(), nil)
	out.ConfigKeys = sortedKeys(s.seenConfig)
	out.RecordFields = sortedKeys(s.seenRecord)
	return out, s.problems
}

// Render renders a template over its bindings: config.* and, for a
// per-record step, record.* — dotted (namespaced) field names reachable
// both as record["ns.name"] and as record.ns.name. Lenient on absent
// values by design (see the package comment).
func Render(source string, config map[string]any, record map[string]any) (string, error) {
	b := liquid.Bindings{"config": Config(config)}
	if record != nil {
		b["record"] = nest(record)
	}
	out, err := eng().ParseAndRenderString(source, b)
	if err != nil {
		return "", fmt.Errorf("template: %s", cleanErr(err))
	}
	return out, nil
}

var (
	exprOnce sync.Once
	exprCfg  expressions.Config
)

// Eval evaluates one object expression — the inside of `{{ }}` — to a typed
// value over vars, the way a binding's single-placeholder leaf substitutes
// the typed value (SPEC §10a). The standard filters are registered; the
// allowlist is Check's job, at verify time.
func Eval(expr string, vars map[string]any) (any, error) {
	exprOnce.Do(func() {
		exprCfg = expressions.NewConfig()
		filters.AddStandardFilters(&exprCfg)
	})
	v, err := expressions.EvaluateString(strings.TrimSpace(expr), expressions.NewContext(vars, exprCfg))
	if err != nil {
		return nil, fmt.Errorf("template: %s", cleanErr(err))
	}
	return v, nil
}

// Nest is nest, for callers that bind their own record.
func Nest(fields map[string]any) map[string]any { return nest(fields) }

// Config is the config.* a template sees: the step's own with:, minus the
// template itself (SPEC §9).
func Config(with map[string]any) map[string]any {
	out := make(map[string]any, len(with))
	for k, v := range with {
		if k != Key {
			out[k] = v
		}
	}
	return out
}

// nest exposes a namespaced field (`review.first_line`, SPEC §4a) as a
// nested lookup too, so `record.review.first_line` reads it — a dotted
// key is a lookup path in Liquid. A flat key that would collide with the
// namespace keeps its value and the nesting is skipped.
func nest(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	for k, v := range fields {
		if !strings.Contains(k, ".") {
			continue
		}
		parts := strings.Split(k, ".")
		cur := out
		ok := true
		for _, p := range parts[:len(parts)-1] {
			next, exists := cur[p]
			if !exists {
				m := map[string]any{}
				cur[p] = m
				cur = m
				continue
			}
			m, isMap := next.(map[string]any)
			if !isMap {
				ok = false
				break
			}
			cur = m
		}
		if ok {
			if _, taken := cur[parts[len(parts)-1]]; !taken {
				cur[parts[len(parts)-1]] = v
			}
		}
	}
	return out
}

// scanner walks the render tree collecting references and problems.
type scanner struct {
	scope       Scope
	declared    []string
	config      map[string]any
	problems    []string
	seenConfig  map[string]bool
	seenRecord  map[string]bool
	seenProblem map[string]bool
}

func (s *scanner) problem(msg string) {
	if s.seenProblem[msg] {
		return
	}
	s.seenProblem[msg] = true
	s.problems = append(s.problems, msg)
}

// walk visits a node with the loop variables in scope.
func (s *scanner) walk(n render.Node, loops []string) {
	switch t := n.(type) {
	case *render.SeqNode:
		for _, c := range t.Children {
			s.walk(c, loops)
		}
	case *render.BlockNode:
		if s.scope == Binding {
			s.problem(fmt.Sprintf("template: {%% %s %%} — a request template is an object ({{ … }}), not a block (SPEC §10a)", t.Name))
			return
		}
		s.block(t, loops)
	case *render.TagNode:
		if s.scope == Binding {
			s.problem(fmt.Sprintf("template: {%% %s %%} — a request template is an object ({{ … }}), not a block (SPEC §10a)", t.Name))
			return
		}
		if !Tags[t.Name] {
			s.problem(fmt.Sprintf("template: {%% %s %%} is not in the dialect — the tags are if/elsif/else/unless/case/when, for, comment, raw (SPEC §10 item 10, ADR-057)", t.Name))
			return
		}
		s.expr(t.Args, loops)
	case *render.ObjectNode:
		s.expr(objectExpr(t.Source), loops)
	}
}

func (s *scanner) block(b *render.BlockNode, loops []string) {
	if !Tags[b.Name] {
		s.problem(fmt.Sprintf("template: {%% %s %%} is not in the dialect — the tags are if/elsif/else/unless/case/when, for, comment, raw (SPEC §10 item 10, ADR-057)", b.Name))
		return
	}
	inner := loops
	switch b.Name {
	case "comment", "raw":
		return // never rendered, never scanned
	case "for":
		v, rest, ok := strings.Cut(strings.TrimSpace(b.Args), " ")
		if ok && strings.HasPrefix(strings.TrimSpace(rest), "in ") {
			inner = append(append([]string(nil), loops...), strings.TrimSpace(v), "forloop")
			s.expr(strings.TrimPrefix(strings.TrimSpace(rest), "in "), loops)
		} else {
			s.expr(b.Args, loops)
		}
	default:
		s.expr(b.Args, loops)
	}
	for _, c := range b.Body {
		s.walk(c, inner)
	}
	for _, cl := range b.Clauses {
		s.block(cl, loops)
	}
}

// objectExpr strips {{ }} (and whitespace-control dashes) from an object.
func objectExpr(source string) string {
	s := strings.TrimSpace(source)
	s = strings.TrimPrefix(s, "{{")
	s = strings.TrimSuffix(s, "}}")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimSuffix(s, "-")
	return s
}

var keywords = map[string]bool{
	"and": true, "or": true, "not": true, "contains": true, "in": true,
	"limit": true, "offset": true, "reversed": true,
	"true": true, "false": true, "nil": true, "null": true, "empty": true, "blank": true,
}

// expr scans one expression for variable chains and filter names.
func (s *scanner) expr(src string, loops []string) {
	i := 0
	filterNext := false
	for i < len(src) {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(src) && src[j] != c {
				j++
			}
			i = j + 1
		case c == '|':
			filterNext = true
			i++
		case isIdentStart(c):
			chain, next := readChain(src, i)
			i = next
			if filterNext {
				filterNext = false
				name := chain[0]
				if roots[name] {
					// The retired `{{a|b}}` alternatives (M31): a variable in
					// filter position is the old fallback, not a filter.
					s.problem(fmt.Sprintf("template: `{{ a | %s }}` — alternatives are Liquid's default filter now: write `{{ a | default: %s }}` (ADR-057, M31)", strings.Join(chain, "."), strings.Join(chain, ".")))
					continue
				}
				if !Filters[name] {
					s.problem(fmt.Sprintf("template: filter %q is not in the dialect — the filters are %s (SPEC §10 item 10, ADR-057)", name, strings.Join(sortedKeys(Filters), ", ")))
				}
				continue
			}
			// A named parameter (limit:1) is not a variable.
			if k := skipSpaces(src, next); k < len(src) && src[k] == ':' && len(chain) == 1 {
				i = k + 1
				continue
			}
			s.ref(chain, loops)
		default:
			i++
		}
	}
}

// roots are the binding namespaces (SPEC §10a).
var roots = map[string]bool{"record": true, "config": true, "variables": true, "session": true}

func (s *scanner) ref(chain []string, loops []string) {
	root := chain[0]
	if keywords[root] {
		return
	}
	for _, l := range loops {
		if l == root {
			return
		}
	}
	path := strings.Join(chain[1:], ".")
	if s.scope == Binding {
		if !roots[root] {
			s.problem(fmt.Sprintf("template: %q is not a variable here — a request template reads record.<field>, config.<key>, variables.<name> or session (SPEC §10a)", strings.Join(chain, ".")))
		}
		return
	}
	switch root {
	case "config":
		if path == "" {
			s.problem("template: config needs a key — config.<key> names one of the step's with: keys (ADR-057)")
			return
		}
		key := chain[1]
		if _, ok := s.config[key]; !ok || key == Key {
			s.problem(fmt.Sprintf("template: config.%s names no key of this step's with: (ADR-057)", key))
			return
		}
		s.seenConfig[key] = true
	case "record":
		if s.scope == Batch {
			s.problem(fmt.Sprintf("template: record.%s — records are not in scope for a batch step: an ai/* template renders once over config.* and the records arrive as the fenced payload (ADR-035, ADR-057); render per record with text/compose and pass the field through uses:", path))
			return
		}
		if path == "" {
			s.problem("template: record needs a field — record.<field> names one of the step's uses: fields (ADR-057)")
			return
		}
		field, ok := s.match(path)
		if !ok {
			s.problem(fmt.Sprintf("template: record.%s is not in this step's uses: (or of:) — declare it there first (SPEC §7, ADR-057)", path))
			return
		}
		s.seenRecord[field] = true
	default:
		s.problem(fmt.Sprintf("template: %q is not a variable here — write record.<field> or config.<key> (ADR-057)", strings.Join(chain, ".")))
	}
}

// match finds the declared field a record path reads: the path itself, or
// the longest declared field the path descends into (`recent_posts.first`
// reads recent_posts; `review.first_line` reads the namespaced field).
func (s *scanner) match(path string) (string, bool) {
	best := ""
	for _, f := range s.declared {
		if path == f || strings.HasPrefix(path, f+".") {
			if len(f) > len(best) {
				best = f
			}
		}
	}
	return best, best != ""
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdent(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}

// readChain reads a.b["c.d"].e starting at i: the segments, and where the
// chain ends. A bracket holding anything but a string literal is a
// wildcard segment.
func readChain(src string, i int) ([]string, int) {
	var chain []string
	for i < len(src) {
		if isIdentStart(src[i]) {
			j := i
			for j < len(src) && isIdent(src[j]) {
				j++
			}
			chain = append(chain, src[i:j])
			i = j
		} else {
			break
		}
		for i < len(src) {
			if src[i] == '.' && i+1 < len(src) && isIdentStart(src[i+1]) {
				i++
				break
			}
			if src[i] == '[' {
				end := strings.IndexByte(src[i:], ']')
				if end < 0 {
					return chain, len(src)
				}
				inner := strings.TrimSpace(src[i+1 : i+end])
				if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') {
					chain = append(chain, inner[1:len(inner)-1])
				} else {
					chain = append(chain, "*")
				}
				i += end + 1
				continue
			}
			return chain, i
		}
	}
	return chain, i
}

func skipSpaces(src string, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

var (
	objectRE      = regexp.MustCompile(`\{\{-?([^{}]*)-?\}\}`)
	alternativeRE = regexp.MustCompile(`\|\s*(record|config|variables|session)(\.|\s*\||\s*$)`)
)

// retiredAlternatives spots the pre-M31 `{{a|b}}` fallback — a namespace
// in filter position, which the parser would only call a syntax error —
// and names the `| default:` rewrite (ADR-057, M31).
func retiredAlternatives(source string) string {
	for _, m := range objectRE.FindAllStringSubmatch(source, -1) {
		expr := strings.TrimSpace(m[1])
		if !alternativeRE.MatchString(expr) {
			continue
		}
		parts := strings.Split(expr, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return fmt.Sprintf("template: `{{ %s }}` — alternatives are Liquid's default filter now (ADR-057, M31): write `{{ %s }}`", expr, strings.Join(parts, " | default: "))
	}
	return ""
}

var errPrefix = regexp.MustCompile(`^Liquid error: `)

func cleanErr(err error) string {
	return errPrefix.ReplaceAllString(err.Error(), "")
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
