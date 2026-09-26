package docsgen

import (
	"fmt"
	"strings"
)

// ledgerPages renders reference/ledger-schema/index.md and one node per
// table or view in spec/ledger.sql.
func (s *site) ledgerPages() []*page {
	out := []*page{s.ledgerIndex()}
	for _, o := range s.objects {
		out = append(out, s.ledgerNode(o))
	}
	return out
}

// agentObject finds the `gtme help --agent` note for a ledger object.
func (s *site) agentObject(name string) *agentLedgerObj {
	for i := range s.agent.Ledger.Objects {
		if s.agent.Ledger.Objects[i].Name == name {
			return &s.agent.Ledger.Objects[i]
		}
	}
	return nil
}

func (s *site) ledgerIndex() *page {
	tables, views := 0, 0
	for _, o := range s.objects {
		if o.Kind == "view" {
			views++
		} else {
			tables++
		}
	}
	var b strings.Builder
	b.WriteString("# Ledger schema and views\n\n")
	fmt.Fprintf(&b, "The ledger is one SQLite file with %d tables and %d views, as `spec/ledger.sql` states them. Each object's own page has its columns and the DDL. %s\n\n", tables, views, prose(s.agent.Ledger.Note))
	for _, kind := range []string{"view", "table"} {
		var rows [][]string
		for _, o := range s.objects {
			if o.Kind != kind {
				continue
			}
			does := ""
			cols := make([]string, len(o.Columns))
			for i, c := range o.Columns {
				cols[i] = c.Name
			}
			if ao := s.agentObject(o.Name); ao != nil {
				does = ao.Does
				if len(cols) == 0 {
					cols = ao.Columns
				}
			}
			rows = append(rows, []string{
				fmt.Sprintf("[%s](/reference/ledger-schema/%s)", code(o.Name), o.Name),
				cell(strings.Join(cols, ", ")),
				cell(prose(does)),
			})
		}
		if kind == "view" {
			b.WriteString("## Views\n\nThe read surface. A `sql/*` step, a `{query:}` value, and `gtme query` are written against these first.\n\n")
		} else {
			b.WriteString("\n## Tables\n\nThe rows the views rank and join. Read them for provenance.\n\n")
		}
		b.WriteString(table([]string{"Object", "Columns", "Holds"}, rows))
	}
	if len(s.agent.Ledger.Shapes) > 0 {
		b.WriteString("\n## Query shapes\n\nThe queries a `sql/*` step or a config value is usually one of, from `gtme help --agent`.\n")
		for _, q := range s.agent.Ledger.Shapes {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n\n```sql\n%s\n```\n", capitalize(q.Name), prose(sentence(q.Use)), strings.TrimRight(q.SQL, "\n"))
		}
	}
	b.WriteString(s.usedIn("/reference/ledger-schema"))
	return &page{
		path:        "reference/ledger-schema/index.md",
		node:        "/reference/ledger-schema",
		name:        "Ledger schema and views",
		description: "Every table and view in the ledger with its columns and what it holds, plus the query shapes SQL steps take, generated from spec/ledger.sql",
		audience:    "You're about to write SQL against the ledger and need the views to read first, the tables behind them, and a query shape to start from.",
		learn:       []string{"the views a query reads first", "every table and what its rows hold", "the query shapes a sql step usually takes"},
		roles:       []string{"operator", "builder", "agent"},
		body:        b.String(),
	}
}

func (s *site) ledgerNode(o ledgerObject) *page {
	node := "/reference/ledger-schema/" + o.Name
	ao := s.agentObject(o.Name)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", o.Name)
	if ao != nil {
		fmt.Fprintf(&b, "A %s. %s\n\n", o.Kind, prose(sentence(ao.Does)))
	} else {
		fmt.Fprintf(&b, "A %s the spec defines and `gtme help --agent` leaves out of its public read surface.\n\n", o.Kind)
	}

	b.WriteString("## Columns\n\n")
	if len(o.Columns) > 0 {
		var rows [][]string
		for _, c := range o.Columns {
			rows = append(rows, []string{code(c.Name), c.Type, cell(c.Constraint)})
		}
		b.WriteString(table([]string{"Column", "Type", "Constraints"}, rows))
		if len(o.Constraint) > 0 {
			b.WriteString("\nTable constraints: ")
			parts := make([]string, len(o.Constraint))
			for i, c := range o.Constraint {
				parts[i] = code(c)
			}
			b.WriteString(strings.Join(parts, "; ") + ".\n")
		}
	} else if ao != nil {
		parts := make([]string, len(ao.Columns))
		for i, c := range ao.Columns {
			parts[i] = code(c)
		}
		b.WriteString(strings.Join(parts, ", ") + ".\n")
	}

	b.WriteString("\n## Definition\n\nFrom `spec/ledger.sql`, which is the machine-checkable form of SPEC.md §3:\n\n```sql\n" + strings.TrimRight(o.Definition, "\n") + "\n```\n")
	b.WriteString(s.usedIn(node))

	desc := fmt.Sprintf("The %s %s in the ledger: its columns and its DDL", o.Name, o.Kind)
	if ao != nil {
		desc = short(ao.Does)
	}
	return &page{
		path:        "reference/ledger-schema/" + o.Name + ".md",
		node:        node,
		name:        o.Name,
		description: desc,
		audience:    fmt.Sprintf("You're writing SQL that reads `%s` and need its columns, what each holds, and the DDL as the spec states it.", o.Name),
		learn:       []string{"every column and its type", "what the object holds and who writes it", "the DDL as the spec states it"},
		roles:       []string{"operator", "builder", "agent"},
		body:        b.String(),
	}
}
