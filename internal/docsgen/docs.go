package docsgen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// docPage is one authored page under docs/: what the generator reads from
// it (its name, links, `defines:`, and code blocks) — never what it writes.
type docPage struct {
	Path    string // concepts/ledger.md
	Node    string // /concepts/ledger
	Name    string
	Links   []docLink // frontmatter links
	Inline  []string  // inline link targets, in order
	Defines []entry
	Blocks  []codeBlock
	Order   int
}

type docLink struct {
	To          string `yaml:"to"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type codeBlock struct {
	Lang string
	Text string
}

// entry is one glossary term: the term, its one-sentence definition, and the
// page that owns it.
type entry struct {
	term       string
	definition string
	page       string
	pageName   string
}

type frontmatter struct {
	Name    string    `yaml:"name"`
	Order   int       `yaml:"order"`
	Links   []docLink `yaml:"links"`
	Defines []struct {
		Term       string `yaml:"term"`
		Definition string `yaml:"definition"`
	} `yaml:"defines"`
}

var (
	inlineLinkRe = regexp.MustCompile(`\]\((/[^)\s#]*)`)
	fenceBlockRe = regexp.MustCompile("(?s)```(\\w*)\n(.*?)```")
)

func parseDocs(docs map[string]string) ([]*docPage, error) {
	var pages []*docPage
	for path, text := range docs {
		fm, body, err := splitFrontmatter(text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		var f frontmatter
		if err := yaml.Unmarshal([]byte(fm), &f); err != nil {
			return nil, fmt.Errorf("%s: frontmatter: %w", path, err)
		}
		p := &docPage{Path: path, Node: nodeOf(path), Name: f.Name, Links: f.Links, Order: f.Order}
		for _, m := range inlineLinkRe.FindAllStringSubmatch(body, -1) {
			p.Inline = append(p.Inline, m[1])
		}
		for _, m := range fenceBlockRe.FindAllStringSubmatch(body, -1) {
			p.Blocks = append(p.Blocks, codeBlock{Lang: m[1], Text: m[2]})
		}
		for _, d := range f.Defines {
			if strings.TrimSpace(d.Term) == "" || strings.TrimSpace(d.Definition) == "" {
				return nil, fmt.Errorf("%s: defines: every entry needs a term and a definition", path)
			}
			p.Defines = append(p.Defines, entry{term: strings.TrimSpace(d.Term), definition: strings.TrimSpace(d.Definition), page: p.Node, pageName: p.Name})
		}
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool { return pageLess(pages[i], pages[j]) })
	return pages, nil
}

// pageLess orders pages the way a reader meets them: start, then concepts,
// then guides, each by `order`, then everything else by path.
func pageLess(a, b *docPage) bool {
	ra, rb := collectionRank(a.Path), collectionRank(b.Path)
	if ra != rb {
		return ra < rb
	}
	if a.Order != b.Order {
		return a.Order < b.Order
	}
	return a.Path < b.Path
}

func collectionRank(path string) int {
	for i, c := range []string{"start/", "concepts/", "guides/"} {
		if strings.HasPrefix(path, c) {
			return i
		}
	}
	return 3
}

func splitFrontmatter(text string) (fm, body string, err error) {
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("no frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("unterminated frontmatter")
	}
	fm = text[4 : 4+end]
	body = text[4+end+4:]
	return fm, body, nil
}

func nodeOf(path string) string {
	n := "/" + strings.TrimSuffix(path, ".md")
	n = strings.TrimSuffix(n, "/index")
	if n == "/index" {
		return "/"
	}
	return n
}

// collectGlossary gathers every page's `defines:`, refusing two owners for
// one term, and sorts the result alphabetically.
func collectGlossary(pages []*docPage) ([]entry, error) {
	var out []entry
	owner := map[string]string{}
	for _, p := range pages {
		for _, e := range p.Defines {
			key := strings.ToLower(e.term)
			if prev, dup := owner[key]; dup {
				return nil, fmt.Errorf("term %q is defined on both %s and %s; one page owns a term", e.term, prev, p.Path)
			}
			owner[key] = p.Path
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].term) < strings.ToLower(out[j].term)
	})
	return out, nil
}

// backlink is one authored page that links a target node.
type backlink struct {
	Page        *docPage
	Description string
}

// backlinks maps every link target (path, no fragment) to the pages linking
// it, in reading order; a frontmatter link's description travels with it.
func backlinks(pages []*docPage) map[string][]backlink {
	out := map[string][]backlink{}
	for _, p := range pages {
		seen := map[string]bool{}
		for _, l := range p.Links {
			t := strings.SplitN(l.To, "#", 2)[0]
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out[t] = append(out[t], backlink{Page: p, Description: l.Description})
		}
		for _, t := range p.Inline {
			if seen[t] {
				continue
			}
			seen[t] = true
			out[t] = append(out[t], backlink{Page: p})
		}
	}
	return out
}

// shellExample finds the first authored `sh` block whose first command is
// the verb, for a CLI page's example.
func shellExample(pages []*docPage, verb string) (block string, from *docPage) {
	for _, p := range pages {
		for _, b := range p.Blocks {
			if b.Lang != "sh" && b.Lang != "bash" {
				continue
			}
			first := firstLine(b.Text)
			if first == "gtme "+verb || strings.HasPrefix(first, "gtme "+verb+" ") {
				return dedent(b.Text), p
			}
		}
	}
	return "", nil
}

// dedent strips the indentation a fenced block carries inside a list item
// and any trailing blank lines.
func dedent(s string) string {
	lines := strings.Split(strings.TrimRight(s, " \n"), "\n")
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := indentOf(l); common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return strings.Join(lines, "\n")
	}
	for i, l := range lines {
		if len(l) >= common {
			lines[i] = l[common:]
		}
	}
	return strings.Join(lines, "\n")
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

// yamlExample finds the first authored `yaml` block using the adapter and
// returns the step (or source) item that names it.
func yamlExample(pages []*docPage, id string) (fragment string, from *docPage) {
	for _, p := range pages {
		for _, b := range p.Blocks {
			if b.Lang != "yaml" && b.Lang != "yml" {
				continue
			}
			if frag := yamlItemUsing(dedent(b.Text), id); frag != "" {
				return frag, p
			}
		}
	}
	return "", nil
}

// yamlItemUsing cuts the list item or mapping that holds `use: <id>` out of
// a YAML document by indentation: up to the `- ` or `key:` line two columns
// left of the use line, down to the next line at or left of that column.
func yamlItemUsing(doc, id string) string {
	lines := strings.Split(doc, "\n")
	useRe := regexp.MustCompile(`^(\s*)use:\s*` + regexp.QuoteMeta(id) + `\s*(#.*)?$`)
	for i, l := range lines {
		m := useRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		u := len(m[1])
		start := i
		for start > 0 {
			prev := lines[start-1]
			ind := indentOf(prev)
			if strings.TrimSpace(prev) == "" {
				break
			}
			if ind < u {
				if ind == u-2 && (strings.HasPrefix(strings.TrimSpace(prev), "- ") || strings.HasSuffix(strings.TrimSpace(strings.SplitN(prev, "#", 2)[0]), ":")) {
					start--
				}
				break
			}
			start--
		}
		end := i
		for end+1 < len(lines) {
			next := lines[end+1]
			if strings.TrimSpace(next) == "" {
				// Keep a blank only if the item continues after it.
				if end+2 < len(lines) && indentOf(lines[end+2]) >= u && strings.TrimSpace(lines[end+2]) != "" {
					end++
					continue
				}
				break
			}
			if indentOf(next) < u {
				break
			}
			end++
		}
		return strings.TrimRight(strings.Join(lines[start:end+1], "\n"), "\n")
	}
	return ""
}

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
