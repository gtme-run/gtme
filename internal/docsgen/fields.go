package docsgen

import (
	"fmt"
	"strings"
)

// fieldPages renders reference/fields/index.md and one node per type file.
func (s *site) fieldPages() []*page {
	out := []*page{s.fieldIndex()}
	for _, t := range s.types {
		out = append(out, s.fieldNode(t))
	}
	return out
}

func kindOf(t fieldType) string {
	switch t.Kind {
	case "subject":
		return "subject: what a pipeline delivers to"
	case "signal":
		return "signal: what a pipeline finds and traverses from"
	}
	return t.Kind
}

func keyedBy(t fieldType) string {
	var parts []string
	for _, tier := range t.Identity {
		from, _ := identityExpr(tier)
		parts = append(parts, from)
	}
	return strings.Join(parts, ", then ")
}

func fieldRows(t fieldType) [][]string {
	var rows [][]string
	for _, f := range t.Fields {
		typ := f.Type
		if f.Type == "array" && f.ItemsType != "" {
			typ = "array of " + f.ItemsType
		}
		if len(f.Enum) > 0 {
			vals := make([]string, len(f.Enum))
			for i, e := range f.Enum {
				vals[i] = fmt.Sprint(e)
			}
			typ += ", one of " + strings.Join(vals, ", ")
		}
		if f.Format != "" {
			typ += " (" + f.Format + ")"
		}
		tier := f.Tier
		if f.Reserved {
			tier += ", reserved"
		}
		rows = append(rows, []string{code(f.Name), tier, cell(typ), code(f.Normalization), cell(prose(f.Description))})
	}
	return rows
}

var fieldHeader = []string{"Field", "Tier", "Type", "Normalization", "Description"}

func (s *site) fieldIndex() *page {
	var b strings.Builder
	b.WriteString("# Canonical field registry\n\n")
	fmt.Fprintf(&b, "The %d types the binary ships, and every canonical field of each. A field name not listed under a type must carry a vendor's or a pipeline's prefix. Each type's own page adds how a record is keyed, tier by tier, and the relations its fields mint.\n\n", len(s.types))
	var rows [][]string
	for _, t := range s.types {
		rows = append(rows, []string{
			fmt.Sprintf("[%s](/reference/fields/%s)", code(t.EntityType), t.EntityType),
			cell(kindOf(t)),
			cell(keyedBy(t)),
			fmt.Sprint(len(t.Fields)),
		})
	}
	b.WriteString(table([]string{"Type", "Kind", "Keyed by", "Fields"}, rows))
	for _, t := range s.types {
		fmt.Fprintf(&b, "\n## %s\n\n", t.EntityType)
		b.WriteString(table(fieldHeader, fieldRows(t)))
	}
	b.WriteString("\n`Tier` is `identity` for a field a key is built from and `core` for the rest; `reserved` marks a key tier no adapter provides yet. `Normalization` is the rule that puts a value in its stored form.\n")
	b.WriteString(s.usedIn("/reference/fields"))
	return &page{
		path:        "reference/fields/index.md",
		node:        "/reference/fields",
		name:        "Canonical field registry",
		description: "Every canonical field of every built-in type, with its tier, type, and normalization rule, generated from spec/fields",
		audience:    "You're mapping a CSV column or a vendor's field onto gtme's names and need the exact field name and its rule.",
		learn:       []string{"every canonical field per type", "which fields a key is built from", "the normalization rule each value gets"},
		roles:       []string{"builder", "extender", "agent"},
		body:        b.String(),
	}
}

func (s *site) fieldNode(t fieldType) *page {
	node := "/reference/fields/" + t.EntityType
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t.EntityType)
	b.WriteString(prose(t.Description) + "\n\n")

	b.WriteString("## Identity\n\n")
	fmt.Fprintf(&b, "A %s is keyed by the first of these tiers the record has, strongest first.\n\n", t.EntityType)
	var rows [][]string
	for i, tier := range t.Identity {
		from, looks := identityExpr(tier)
		rows = append(rows, []string{fmt.Sprint(i + 1), cell(from), cell(looks)})
	}
	b.WriteString(table([]string{"Tier", "From", "Key"}, rows))
	if len(t.IdentityRaw) > 0 {
		b.WriteString("\nAs the type file states it:\n\n```json\n" + pretty(t.IdentityRaw) + "\n```\n")
	}

	b.WriteString("\n## Fields\n\n")
	b.WriteString(table(fieldHeader, fieldRows(t)))

	var refs [][]string
	for _, f := range t.Fields {
		if f.Reference != nil {
			carried := make([]string, len(f.Reference.Fields))
			for i, c := range f.Reference.Fields {
				carried[i] = code(c)
			}
			refs = append(refs, []string{code(f.Name), fmt.Sprintf("[%s](/reference/fields/%s)", code(f.Reference.Type), f.Reference.Type), code(f.Reference.Relation), strings.Join(carried, ", ")})
		}
	}
	if len(refs) > 0 {
		b.WriteString("\n## References\n\nA field with a reference names a record of another type. A source that writes it also mints that record and the relation between them.\n\n")
		b.WriteString(table([]string{"Field", "Names a", "Relation", "Fields carried"}, refs))
	}

	var examples [][]string
	for _, f := range t.Fields {
		if f.Example != nil && f.Example != "" {
			examples = append(examples, []string{code(f.Name), cell(prose(fmt.Sprint(f.Example)))})
		}
	}
	if len(examples) > 0 {
		b.WriteString("\n## Examples\n\n")
		b.WriteString(table([]string{"Field", "Example value"}, examples))
	}
	b.WriteString(s.usedIn(node))

	return &page{
		path:        "reference/fields/" + t.EntityType + ".md",
		node:        node,
		name:        t.EntityType,
		description: fmt.Sprintf("Every canonical field of a %s record, how one is keyed tier by tier, and the relations its fields mint, generated from spec/fields/%s.json", t.EntityType, t.EntityType),
		audience:    fmt.Sprintf("You're mapping a column or a vendor field to a %s field and need the exact name, its tier, and its normalization rule.", t.EntityType),
		learn:       []string{fmt.Sprintf("every canonical field of a %s, with its tier and type", t.EntityType), fmt.Sprintf("how a %s is keyed, tier by tier", t.EntityType), "which fields mint a relation to another type"},
		roles:       []string{"builder", "extender", "agent"},
		body:        b.String(),
	}
}
