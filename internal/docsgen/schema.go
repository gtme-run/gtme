package docsgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// obj is a JSON object that keeps its keys in file order, so a key table
// reads in the order the schema's author wrote it (name before steps),
// not alphabetically.
type obj struct {
	keys []string
	m    map[string]any
}

func (o *obj) get(k string) any {
	if o == nil {
		return nil
	}
	return o.m[k]
}

func (o *obj) has(k string) bool {
	if o == nil {
		return false
	}
	_, ok := o.m[k]
	return ok
}

func (o *obj) str(k string) string {
	s, _ := o.get(k).(string)
	return s
}

func (o *obj) child(k string) *obj {
	c, _ := o.get(k).(*obj)
	return c
}

// decodeOrdered parses JSON into *obj, []any, string, json.Number, bool, nil.
func decodeOrdered(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func decodeObj(b []byte) (*obj, error) {
	v, err := decodeOrdered(b)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*obj)
	if !ok {
		return nil, fmt.Errorf("not a JSON object")
	}
	return o, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch d {
	case '{':
		o := &obj{m: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := kt.(string)
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			o.keys = append(o.keys, k)
			o.m[k] = v
		}
		_, err := dec.Token()
		return o, err
	case '[':
		var out []any
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		_, err := dec.Token()
		return out, err
	}
	return nil, fmt.Errorf("unexpected %v", d)
}

// keyRow is one row of a generated key table.
type keyRow struct {
	Key, Type, Required, Description string
}

// flattener turns a JSON Schema into key rows: nested objects as dotted
// keys, map values under an upper-case placeholder, array items under `[]`.
type flattener struct {
	root *obj
	// label returns the text for a $ref that should not be inlined (a
	// pipeline's step, which has its own table); "" inlines the definition.
	label func(ref string) string
}

// resolve follows a local $ref (#/definitions/name) unless label claims it.
func (f *flattener) resolve(p *obj) *obj {
	for i := 0; p != nil && i < 8; i++ {
		ref := p.str("$ref")
		if ref == "" || (f.label != nil && f.label(ref) != "") {
			return p
		}
		name := strings.TrimPrefix(ref, "#/definitions/")
		def := f.root.child("definitions").child(name)
		if def == nil {
			return p
		}
		p = def
	}
	return p
}

// placeholder names a map key a user chooses, from the map's own key.
func placeholder(parent string) string {
	switch parent {
	case "fields", "provides":
		return "FIELD"
	case "errors":
		return "STATUS"
	case "contents":
		return "PATH"
	case "variables", "query", "headers":
		return "NAME"
	}
	return "KEY"
}

// alternatives returns the oneOf/anyOf branches of a schema.
func alternatives(p *obj) []*obj {
	var out []*obj
	for _, k := range []string{"oneOf", "anyOf"} {
		if xs, ok := p.get(k).([]any); ok {
			for _, x := range xs {
				if o, ok := x.(*obj); ok {
					out = append(out, o)
				}
			}
		}
	}
	return out
}

// onlyRequired reports whether a branch is `{required: [...]}` and nothing
// else: a constraint on which keys appear, not a type.
func onlyRequired(p *obj) bool {
	return p != nil && len(p.keys) == 1 && p.has("required")
}

// complex reports whether a schema has keys of its own to list.
func (f *flattener) complex(p *obj) bool {
	p = f.resolve(p)
	if p == nil {
		return false
	}
	if p.child("properties") != nil {
		return true
	}
	if ap := p.child("additionalProperties"); ap != nil && f.complex(ap) {
		return true
	}
	for _, a := range alternatives(p) {
		if f.complex(a) {
			return true
		}
	}
	if it := p.child("items"); it != nil && f.complex(it) {
		return true
	}
	return false
}

// typeOf renders a schema's type in a few words.
func (f *flattener) typeOf(p *obj) string {
	if p == nil {
		return "any"
	}
	if ref := p.str("$ref"); ref != "" && f.label != nil {
		if l := f.label(ref); l != "" {
			return l
		}
	}
	p = f.resolve(p)
	if p.has("const") {
		return "always " + code(fmt.Sprint(p.get("const")))
	}
	if e, ok := p.get("enum").([]any); ok {
		vals := make([]string, len(e))
		for i, x := range e {
			vals[i] = code(fmt.Sprint(x))
		}
		return "one of " + strings.Join(vals, ", ")
	}
	t := p.str("type")
	if t == "" {
		var alts []string
		for _, a := range alternatives(p) {
			if onlyRequired(a) {
				continue
			}
			alts = append(alts, f.typeOf(a))
		}
		if len(alts) > 0 {
			return strings.Join(dedupe(alts), " or ")
		}
		if n := p.child("not"); n != nil && len(n.keys) == 0 {
			return "not allowed"
		}
		if p.child("properties") != nil {
			return "object"
		}
		return "any"
	}
	switch t {
	case "array":
		if it := p.child("items"); it != nil {
			return "array of " + f.typeOf(it)
		}
		return "array"
	case "object":
		if ap := p.child("additionalProperties"); ap != nil && p.child("properties") == nil && !f.complex(ap) {
			return "map of " + f.typeOf(ap)
		}
		return "object"
	case "string":
		if pat := p.str("pattern"); pat != "" {
			return "string matching " + code(pat)
		}
	}
	return t
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// rows flattens a schema's properties (and what they nest) into key rows.
func (f *flattener) rows(s *obj, prefix string) []keyRow {
	s = f.resolve(s)
	if s == nil {
		return nil
	}
	req := map[string]string{}
	if r, ok := s.get("required").([]any); ok {
		for _, x := range r {
			req[fmt.Sprint(x)] = "yes"
		}
	}
	// anyOf: [{required: [use]}, {required: [group]}] makes each "one of".
	var either []string
	for _, a := range alternatives(s) {
		if onlyRequired(a) {
			if r, ok := a.get("required").([]any); ok && len(r) == 1 {
				either = append(either, fmt.Sprint(r[0]))
			}
		}
	}
	if len(either) > 1 {
		names := make([]string, len(either))
		for i, n := range either {
			names[i] = code(n)
		}
		for _, n := range either {
			if req[n] == "" {
				req[n] = "one of " + strings.Join(names, ", ")
			}
		}
	}
	var out []keyRow
	props := s.child("properties")
	if props == nil {
		return nil
	}
	for _, name := range props.keys {
		p, _ := props.get(name).(*obj)
		out = append(out, f.row(prefix+name, name, p, req[name])...)
	}
	return out
}

// row renders one key and everything beneath it.
func (f *flattener) row(key, name string, p *obj, required string) []keyRow {
	if required == "" {
		required = "no"
	}
	desc := p.str("description")
	r := f.resolve(p)
	if desc == "" && r != p {
		desc = r.str("description")
	}
	out := []keyRow{{Key: key, Type: f.typeOf(p), Required: required, Description: desc}}
	if ref := p.str("$ref"); ref != "" && f.label != nil && f.label(ref) != "" {
		return out
	}
	out = append(out, f.children(key, name, r)...)
	return out
}

// children renders what a schema nests: its properties, the value schema of
// a map, the properties of object branches, and array items.
func (f *flattener) children(key, name string, p *obj) []keyRow {
	if p == nil {
		return nil
	}
	var out []keyRow
	if p.child("properties") != nil {
		out = append(out, f.rows(p, key+".")...)
	}
	if ap := p.child("additionalProperties"); ap != nil && f.complex(ap) {
		ph := key + "." + placeholder(name)
		out = append(out, f.row(ph, placeholder(name), ap, "")...)
	}
	seen := map[string]bool{}
	for _, row := range out {
		seen[row.Key] = true
	}
	for _, a := range alternatives(p) {
		a = f.resolve(a)
		if onlyRequired(a) {
			continue
		}
		for _, row := range f.children(key, name, a) {
			if !seen[row.Key] {
				seen[row.Key] = true
				out = append(out, row)
			}
		}
	}
	if it := p.child("items"); it != nil && f.complex(it) {
		out = append(out, f.rows(f.resolve(it), key+"[].")...)
	}
	return out
}

// keyTable renders rows as a markdown table; a missing description is "—".
func keyTable(rows []keyRow) string {
	var cells [][]string
	for _, r := range rows {
		cells = append(cells, []string{code(r.Key), cell(r.Type), r.Required, descCell(r.Description)})
	}
	return table([]string{"Key", "Type", "Required", "Description"}, cells)
}

func descCell(d string) string {
	if strings.TrimSpace(d) == "" {
		return "—"
	}
	return cell(prose(d))
}

// hasPlaceholder reports whether any row key carries an upper-case segment.
func hasPlaceholder(rows []keyRow) bool {
	for _, r := range rows {
		for _, seg := range strings.Split(strings.ReplaceAll(r.Key, "[]", ""), ".") {
			if seg != "" && seg == strings.ToUpper(seg) && strings.ToLower(seg) != seg {
				return true
			}
		}
	}
	return false
}

// schemaHeadline splits a message schema's title, "ATTEST (adapter → runner)",
// into the message name and its direction.
func schemaHeadline(title string) (name, direction string) {
	name = title
	if i := strings.Index(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		name, direction = title[:i], title[i+2:len(title)-1]
	}
	return name, direction
}

var adrOnlyRe = regexp.MustCompile(`^ADR-\d+\.\s+`)

// firstSentence returns a description's first sentence, verbatim, skipping
// a leading bare "ADR-038." citation.
func firstSentence(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n\n", 2)[0])
	s = adrOnlyRe.ReplaceAllString(s, "")
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '.':
			if depth == 0 && (i+1 == len(s) || s[i+1] == ' ') {
				return s[:i+1]
			}
		}
	}
	return s
}
