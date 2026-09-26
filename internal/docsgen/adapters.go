package docsgen

import (
	"fmt"
	"sort"
	"strings"
)

// adapterPages renders reference/adapters/index.md and one node per built-in.
func (s *site) adapterPages() []*page {
	out := []*page{s.adapterIndex()}
	for _, m := range s.adapters {
		out = append(out, s.adapterNode(m))
	}
	return out
}

func recordsOf(m manifest) string {
	if m.EntityType == "*" || m.EntityType == "" {
		return "any type"
	}
	return m.EntityType
}

// forRecords phrases the record type for prose: "for person records",
// "for records of any type".
func forRecords(m manifest) string {
	if m.EntityType == "*" || m.EntityType == "" {
		return "for records of any type"
	}
	return "for " + m.EntityType + " records"
}

func costOf(m manifest) string {
	if m.CostEstimateUSD == nil {
		return "not published"
	}
	if *m.CostEstimateUSD == 0 {
		return "$0"
	}
	return fmt.Sprintf("$%.4f", *m.CostEstimateUSD)
}

func credsOf(m manifest) string {
	var parts []string
	for _, c := range m.Credentials {
		parts = append(parts, code(c))
	}
	for _, c := range m.CredentialsOptional {
		parts = append(parts, code(c)+" (optional)")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func (s *site) adapterIndex() *page {
	sorted := append([]manifest{}, s.adapters...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := roleRank(sorted[i].Role), roleRank(sorted[j].Role)
		if ri != rj {
			return ri < rj
		}
		return sorted[i].ID < sorted[j].ID
	})
	var b strings.Builder
	b.WriteString("# Adapter catalog\n\n")
	fmt.Fprintf(&b, "The %d adapters built into the gtme binary, by role. Each row links the adapter's own page: its manifest, every `with:` key it accepts, and an example step. `gtme help --agent` prints the same manifests as JSON, and `gtme adapters` lists what's installed beside them, which the [registry](/reference/cli/adapters) can add to.\n\n", len(sorted))
	var rows [][]string
	for _, m := range sorted {
		prov := providesNames(m.Provides)
		provCell := "none declared"
		if len(prov) > 0 {
			provCell = fmt.Sprintf("%d", len(prov))
		}
		rows = append(rows, []string{
			fmt.Sprintf("[%s](/reference/adapters/%s)", code(m.ID), slugOf(m.ID)),
			m.Role,
			recordsOf(m),
			cell(needsSummary(m.Needs)),
			provCell,
			costOf(m),
			cell(credsOf(m)),
		})
	}
	b.WriteString(table([]string{"Adapter", "Role", "Records", "Needs", "Provides", "Per record", "Credentials"}, rows))
	b.WriteString("\n`Needs` is what a record must already carry for the adapter to run; `the step's uses: fields` means the step declares them. `Provides` counts the fields it can write. `Per record` is the estimate plan prints, and `not published` prints as `?` there.\n")

	if len(s.agent.SQLSteps) > 0 {
		b.WriteString("\n## Runner-owned steps\n\nThese have no manifest and no adapter session; the runner executes them from a `query:` in `with:`.\n\n")
		var rows [][]string
		for _, st := range s.agent.SQLSteps {
			rows = append(rows, []string{code(st.Usage), cell(prose(st.Does))})
		}
		b.WriteString(table([]string{"Step", "Does"}, rows))
	}
	b.WriteString(s.usedIn("/reference/adapters"))
	return &page{
		path:        "reference/adapters/index.md",
		node:        "/reference/adapters",
		name:        "Adapter catalog",
		description: "Every adapter built into the gtme binary with its role, record type, needs, provides, cost, and credentials, generated from its manifest",
		audience:    "You need an adapter for a step and want to see every built-in at once, with what each needs, writes, and costs.",
		learn:       []string{"every built-in adapter by role", "what each needs, provides, and costs", "which steps the runner owns without an adapter"},
		roles:       []string{"builder", "extender", "agent"},
		body:        b.String(),
	}
}

func (s *site) adapterNode(m manifest) *page {
	slug := slugOf(m.ID)
	node := "/reference/adapters/" + slug
	prov := providesNames(m.Provides)

	// Two sentences: what it is and what it exchanges; what it costs and demands.
	first := fmt.Sprintf("A built-in %s adapter %s", m.Role, forRecords(m))
	if m.From != "" {
		first += fmt.Sprintf(", traversing from %s records", code(m.From))
	}
	needs := needsSummary(m.Needs)
	switch {
	case len(prov) > 0 && needs != "nothing":
		first += fmt.Sprintf(": it needs %s and provides %d fields.", needs, len(prov))
	case len(prov) > 0:
		first += fmt.Sprintf(": it needs nothing and provides %d fields.", len(prov))
	case needs != "nothing":
		first += fmt.Sprintf(": it needs %s and declares no fields of its own.", needs)
	default:
		first += "."
	}
	var second string
	if m.CostEstimateUSD == nil {
		second = "It publishes no per-record estimate, so plan prints `?`"
	} else if *m.CostEstimateUSD == 0 {
		second = "Plan prices it at $0 per record"
	} else {
		second = fmt.Sprintf("Plan estimates $%.4f per record", *m.CostEstimateUSD)
	}
	switch {
	case len(m.Credentials) > 0:
		second += fmt.Sprintf(", and it demands %s.", credsOf(m))
	case len(m.CredentialsOptional) > 0:
		opt := make([]string, len(m.CredentialsOptional))
		for i, c := range m.CredentialsOptional {
			opt[i] = code(c)
		}
		verb := "is"
		if len(opt) > 1 {
			verb = "are"
		}
		second += fmt.Sprintf(", and it runs without a key, though %s %s optional.", joinAnd(opt), verb)
	default:
		second += ", and it demands no credential."
	}
	lede := []string{first, second}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", m.ID, strings.Join(lede, " "))

	b.WriteString("## Manifest\n\n")
	rows := [][]string{
		{"id", code(m.ID)},
		{"version", fmt.Sprint(m.Version)},
		{"role", m.Role},
		{"entity type", recordsOf(m)},
	}
	if m.From != "" {
		rows = append(rows, []string{"from", code(m.From)})
	}
	if len(m.Relation) > 0 {
		rows = append(rows, []string{"relation", code(strings.TrimSpace(string(m.Relation)))})
	}
	rows = append(rows, []string{"credentials", cell(credsOf(m))})
	rows = append(rows, []string{"cost estimate per record", costOf(m)})
	if m.FreshnessDays > 0 {
		rows = append(rows, []string{"freshness", fmt.Sprintf("%d days (the default `cache:` window)", m.FreshnessDays)})
	}
	if m.Attests {
		rows = append(rows, []string{"attests", "yes: a delivery becomes `sent` when the vendor confirms it"})
	}
	b.WriteString(table([]string{"Key", "Value"}, rows))

	b.WriteString("\n## Needs\n\n")
	switch {
	case len(m.Needs) == 0:
		b.WriteString("Nothing. A source starts a run, so no record precedes it.\n")
	case strings.TrimSpace(string(m.Needs)) == `"dynamic"`:
		b.WriteString("The fields the step's `uses:` line names, and nothing else.\n")
	default:
		b.WriteString("```json\n" + pretty(m.Needs) + "\n```\n")
	}

	b.WriteString("\n## Provides\n\n")
	if len(prov) > 0 {
		b.WriteString("```json\n" + pretty(m.Provides) + "\n```\n")
	} else {
		switch m.Role {
		case "filter":
			b.WriteString("A verdict per record. Fields declared in the step's `provides:` land beside it.\n")
		case "compose", "review":
			b.WriteString("The fields the step's `provides:` line declares.\n")
		case "deliver":
			b.WriteString("Nothing; a delivery row is what it writes.\n")
		default:
			b.WriteString("None declared. What it writes depends on its `with:` config, and plan reports the fields as the step resolves them.\n")
		}
	}

	if props, closed := schemaProps(m.ConfigSchema); len(props) > 0 {
		b.WriteString("\n## Config keys\n\n")
		if closed {
			b.WriteString("The keys `with:` accepts. Plan rejects any other key.\n\n")
		} else {
			b.WriteString("The keys `with:` accepts.\n\n")
		}
		var rows [][]string
		for _, p := range props {
			req := ""
			if p.Required {
				req = "yes"
			}
			rows = append(rows, []string{code(p.Name), cell(p.Type), req, cell(prose(p.Description))})
		}
		b.WriteString(table([]string{"Key", "Type", "Required", "What it sets"}, rows))
	}

	b.WriteString("\n## Example\n\n")
	if frag, from := yamlExample(s.pages, m.ID); frag != "" {
		fmt.Fprintf(&b, "From [%s](%s):\n\n```yaml\n%s\n```\n", from.Name, from.Node, frag)
	} else if frag, name := s.agentExample(m.ID); frag != "" {
		fmt.Fprintf(&b, "From the `%s` example in `gtme help --agent`:\n\n```yaml\n%s\n```\n", name, frag)
	} else {
		fmt.Fprintf(&b, "A minimal step:\n\n```yaml\n  - id: %s\n    use: %s\n```\n", strings.SplitN(m.ID, "/", 2)[len(strings.SplitN(m.ID, "/", 2))-1], m.ID)
	}

	if used := s.examplesUsing(m.ID); len(used) > 0 || len(s.backlinks[node]) > 0 {
		b.WriteString("\n## Used in\n\n")
		for _, l := range s.backlinks[node] {
			fmt.Fprintf(&b, "- [%s](%s)\n", l.Page.Name, l.Page.Node)
		}
		for _, name := range used {
			fmt.Fprintf(&b, "- The `%s` example in `gtme help --agent`\n", name)
		}
	}

	return &page{
		path:        "reference/adapters/" + slug + ".md",
		node:        node,
		name:        m.ID,
		description: fmt.Sprintf("Built-in %s adapter %s: its manifest, needs, provides, config keys, cost, and an example step", m.Role, forRecords(m)),
		audience:    fmt.Sprintf("You're about to put `%s` in a step and want its keys, fields, credentials, and cost before plan tells you.", m.ID),
		learn:       []string{"what the adapter needs and provides", "every `with:` key it accepts", "what it costs and which credential it demands"},
		roles:       []string{"builder", "extender", "agent"},
		body:        b.String(),
	}
}

// agentExample returns the step using the adapter from the first
// `gtme help --agent` example that names it.
func (s *site) agentExample(id string) (fragment, name string) {
	for _, ex := range s.agent.Examples {
		for _, doc := range strings.Split(ex.Yaml, "\n---\n") {
			if frag := yamlItemUsing(doc, id); frag != "" {
				return frag, ex.Name
			}
		}
	}
	return "", ""
}

// examplesUsing names every `gtme help --agent` example whose YAML uses the adapter.
func (s *site) examplesUsing(id string) []string {
	var out []string
	for _, ex := range s.agent.Examples {
		if yamlItemUsing(ex.Yaml, id) != "" {
			out = append(out, ex.Name)
		}
	}
	return out
}
