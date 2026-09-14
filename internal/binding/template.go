package binding

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gtme-run/gtme/internal/template"
)

// tmplContext is everything a template may draw from (SPEC §10a): step config,
// the record's projected canonical fields, the resolved deliver variables, and
// the session id. Since M31 (ADR-057) a leaf is a Liquid object in the one
// dialect every `{{ }}` in gtme shares — `{{ record.linkedin_url | default:
// record.linkedin_internal_url }}` — evaluated by internal/template; the
// engine keeps the typed-leaf and omitempty rules, and verify keeps the
// filter allowlist and refuses the retired `{{a|b}}` alternatives.
type tmplContext struct {
	Config    map[string]any
	Record    map[string]any
	Variables map[string]string
	Session   string
}

var placeholderRE = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// vars binds the four namespaces for evaluation; a namespaced record field
// reads nested as well as flat (internal/template.Nest).
func (c tmplContext) vars() map[string]any {
	vars := map[string]any{"session": c.Session}
	if c.Config != nil {
		vars["config"] = c.Config
	} else {
		vars["config"] = map[string]any{}
	}
	if c.Record != nil {
		vars["record"] = template.Nest(c.Record)
	} else {
		vars["record"] = map[string]any{}
	}
	variables := map[string]any{}
	for k, v := range c.Variables {
		variables[k] = v
	}
	vars["variables"] = variables
	return vars
}

// resolveValue resolves one template leaf. A leaf that is exactly one
// placeholder substitutes the typed value; otherwise placeholders interpolate
// as strings. ok=false means the leaf resolved empty and must be omitted
// (the engine's omitempty rule).
func (c tmplContext) resolveValue(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		return c.resolveString(t)
	case map[string]any:
		out := map[string]any{}
		for k, item := range t {
			if k == "$variables" {
				continue // handled by the caller's splice pass
			}
			if r, ok := c.resolveValue(item); ok {
				out[k] = r
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			if r, ok := c.resolveValue(item); ok {
				out = append(out, r)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case nil:
		return nil, false
	default:
		return v, true // numbers, booleans pass through typed
	}
}

func (c tmplContext) resolveString(s string) (any, bool) {
	trimmed := strings.TrimSpace(s)
	if m := placeholderRE.FindStringSubmatch(trimmed); m != nil && m[0] == trimmed {
		v := c.lookup(m[1])
		if isEmpty(v) {
			return nil, false
		}
		return v, true
	}
	missing := false
	out := placeholderRE.ReplaceAllStringFunc(s, func(ph string) string {
		expr := placeholderRE.FindStringSubmatch(ph)[1]
		v := c.lookup(expr)
		if isEmpty(v) {
			missing = true
			return ""
		}
		return stringifyTemplate(v)
	})
	if missing || strings.TrimSpace(out) == "" {
		return nil, false
	}
	return out, true
}

// lookup evaluates one placeholder expression to a typed value; an
// expression that fails to evaluate (verify already checked the dialect,
// so this is a runtime filter error) resolves empty, which omits the leaf.
func (c tmplContext) lookup(expr string) any {
	v, err := template.Eval(expr, c.vars())
	if err != nil {
		return nil
	}
	return v
}

// resolveString1 renders a template to a plain string (URL, query params).
func (c tmplContext) renderString(s string) string {
	v, ok := c.resolveString(s)
	if !ok {
		return ""
	}
	return stringifyTemplate(v)
}

// resolveBody resolves a request body template, splicing '$variables': true
// objects with the resolved variables that no other placeholder consumed.
func (c tmplContext) resolveBody(body any) any {
	consumed := map[string]bool{}
	collectVariableRefs(body, consumed)
	v, ok := c.resolveWithSplice(body, consumed)
	if !ok {
		return nil
	}
	return v
}

func (c tmplContext) resolveWithSplice(v any, consumed map[string]bool) (any, bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		switch t := v.(type) {
		case []any:
			out := make([]any, 0, len(t))
			for _, item := range t {
				if r, ok := c.resolveWithSplice(item, consumed); ok {
					out = append(out, r)
				}
			}
			if len(out) == 0 {
				return nil, false
			}
			return out, true
		default:
			return c.resolveValue(v)
		}
	}

	out := map[string]any{}
	splice := false
	for k, item := range m {
		if k == "$variables" {
			if b, ok := item.(bool); ok && b {
				splice = true
			}
			continue
		}
		if r, ok := c.resolveWithSplice(item, consumed); ok {
			out[k] = r
		}
	}
	if splice {
		for name, val := range c.Variables {
			if consumed[name] || val == "" {
				continue
			}
			if _, taken := out[name]; !taken {
				out[name] = val
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

var variableRefRE = regexp.MustCompile(`\bvariables\.([A-Za-z0-9_\-]+)`)

// collectVariableRefs finds every 'variables.<name>' reference in a body
// template, so the '$variables' splice excludes individually-routed variables
// (the declarative form of first-class-field routing).
func collectVariableRefs(v any, into map[string]bool) {
	switch t := v.(type) {
	case string:
		for _, m := range placeholderRE.FindAllStringSubmatch(t, -1) {
			for _, ref := range variableRefRE.FindAllStringSubmatch(m[1], -1) {
				into[ref[1]] = true
			}
		}
	case map[string]any:
		for _, item := range t {
			collectVariableRefs(item, into)
		}
	case []any:
		for _, item := range t {
			collectVariableRefs(item, into)
		}
	}
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

// stringifyTemplate renders a typed value into a string context.
func stringifyTemplate(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(raw)
	}
}
