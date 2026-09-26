package docsgen

import (
	"fmt"
	"strings"
)

// cliPages renders reference/cli/index.md and one node per verb.
func (s *site) cliPages() []*page {
	verbs := groupVerbs(s.agent.Verbs)
	out := []*page{s.cliIndex(verbs)}
	for _, v := range verbs {
		out = append(out, s.cliNode(v))
	}
	return out
}

func (s *site) exitCodeTable() string {
	rows := make([][]string, 0, len(s.agent.ExitCodes))
	for _, e := range s.agent.ExitCodes {
		rows = append(rows, []string{fmt.Sprint(e.Code), cell(e.Means)})
	}
	return table([]string{"Code", "Means"}, rows)
}

func (s *site) cliIndex(verbs []verb) *page {
	var b strings.Builder
	b.WriteString("# CLI\n\n")
	fmt.Fprintf(&b, "gtme has %d verbs, in the order `gtme help --agent` prints them. Each verb's own page has its forms, its flags, an example from the docs, and where the docs use it. Everything a verb prints for a person goes to stderr; stdout carries data, so a script or an agent can read it.\n\n", len(verbs))
	var rows [][]string
	for _, v := range verbs {
		rows = append(rows, []string{fmt.Sprintf("[%s](/reference/cli/%s)", code("gtme "+v.Name), v.Name), cell(prose(short(v.Forms[0].Does)))})
	}
	b.WriteString(table([]string{"Verb", "Does"}, rows))
	b.WriteString("\n## Exit codes\n\nEvery verb exits with one of these, so a script can branch on the number.\n\n")
	b.WriteString(s.exitCodeTable())
	return &page{
		path:        "reference/cli/index.md",
		node:        "/reference/cli",
		name:        "CLI",
		description: "Every gtme verb in one table, each linking its own page, with the exit codes they all share, generated from gtme help --agent",
		audience:    "You know what you want gtme to do and need the verb, or a script needs the exit code to branch on.",
		learn:       []string{"every verb and what it does", "the exit codes every verb shares", "where each verb's own page is"},
		roles:       []string{"operator", "builder", "agent"},
		body:        b.String(),
	}
}

func (s *site) cliNode(v verb) *page {
	node := "/reference/cli/" + v.Name
	var b strings.Builder
	fmt.Fprintf(&b, "# gtme %s\n\n", v.Name)
	b.WriteString(prose(sentence(v.Forms[0].Does)) + "\n\n")

	b.WriteString("## Forms\n\n```\n")
	for _, f := range v.Forms {
		b.WriteString(f.Usage + "\n")
	}
	b.WriteString("```\n\n")
	if len(v.Forms) > 1 {
		var rows [][]string
		for _, f := range v.Forms {
			rows = append(rows, []string{code(f.Usage), cell(prose(f.Does))})
		}
		b.WriteString(table([]string{"Form", "Does"}, rows))
		b.WriteString("\n")
	}

	if flags := flagsOf(v); len(flags) > 0 {
		b.WriteString("## Flags\n\n")
		multi := len(v.Forms) > 1
		header := []string{"Flag", "Takes", "What it does"}
		if multi {
			header = []string{"Flag", "Takes", "Form", "What it does"}
		}
		var rows [][]string
		for _, f := range flags {
			takes := "nothing"
			if f.Arg != "" {
				takes = code(f.Arg)
			}
			does := "See the form."
			if f.Does != "" {
				does = cell(prose(sentence(f.Does)))
			}
			row := []string{code(f.Name), takes}
			if multi {
				row = append(row, code(f.Form))
			}
			rows = append(rows, append(row, does))
		}
		b.WriteString(table(header, rows))
		b.WriteString("\n")
	}

	b.WriteString("It exits with one of the [exit codes](/reference/cli#exit-codes) every verb shares.\n\n")
	b.WriteString("## Example\n\n")
	if ex, from := shellExample(s.pages, v.Name); ex != "" {
		fmt.Fprintf(&b, "From [%s](%s):\n\n```sh\n%s\n```\n", from.Name, from.Node, ex)
	} else {
		fmt.Fprintf(&b, "The first form with every optional part left out:\n\n```\n%s\n```\n", required(v.Forms[0].Usage))
	}
	b.WriteString(s.usedIn(node))

	return &page{
		path:        "reference/cli/" + v.Name + ".md",
		node:        node,
		name:        "gtme " + v.Name,
		description: short(v.Forms[0].Does),
		audience:    fmt.Sprintf("You're about to type `gtme %s` and want its forms and its flags.", v.Name),
		learn:       []string{fmt.Sprintf("every form of `gtme %s` and what each does", v.Name), "each flag and what it changes", "where the docs use it"},
		roles:       []string{"operator", "builder", "agent"},
		body:        b.String(),
	}
}

// usedIn lists the authored pages that link a node, or nothing.
func (s *site) usedIn(node string) string {
	links := s.backlinks[node]
	if len(links) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Used in\n\n")
	for _, l := range links {
		if l.Description != "" {
			fmt.Fprintf(&b, "- [%s](%s): %s\n", l.Page.Name, l.Page.Node, l.Description)
		} else {
			fmt.Fprintf(&b, "- [%s](%s)\n", l.Page.Name, l.Page.Node)
		}
	}
	return b.String()
}
