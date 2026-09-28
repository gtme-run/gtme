package docsgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strings"
)

// conformanceSet is one set of files the conformance tests load. markers
// are source fragments that show a test (or a helper it calls) reading the
// set; the page lists every test whose code contains one.
type conformanceSet struct {
	glob    string
	markers []string
}

var conformanceSets = []conformanceSet{
	{"spec/schemas/msg-*.schema.json", []string{`"msg-`, `specPath("schemas")`}},
	{"spec/wire/*.ndjson", []string{`specPath("wire"`}},
	{"spec/schemas/pipeline.schema.json", []string{`"pipeline.schema.json"`, `specPath("schemas")`}},
	{"spec/schemas/manifest.schema.json", []string{`"manifest.schema.json"`, `specPath("schemas")`}},
	{"spec/schemas/field-registry.schema.json", []string{`"field-registry.schema.json"`, `specPath("schemas")`}},
	{"spec/fields/*.json", []string{`specPath("fields"`}},
	{"spec/ledger.sql", []string{`"ledger.sql"`}},
	{"spec/acceptance/*.yaml", []string{`specPath("acceptance"`}},
	{"spec/bindings/*/binding.yaml", []string{`"spec", "bindings"`, `binding.Shipped(`}},
	{"spec/bindings/*/fixtures/*", []string{`"spec", "bindings"`, `binding.Shipped(`}},
	{"spec/bundle-manifest.json", []string{`"bundle-manifest.json"`}},
	{"test/fixtures/registry/*/binding.yaml", []string{`registryEntries`}},
}

// goTest is one Test function in test/conformance and the source text it
// can reach: its body plus the package functions and values it names.
type goTest struct {
	Name, File, Doc string
	reach           string
}

// conformanceTests parses test/conformance/*.go (stdlib go/parser; nothing
// is compiled or run) into its Test functions, in file then source order.
func conformanceTests(r repoFiles) ([]goTest, error) {
	fset := token.NewFileSet()
	bodies := map[string]string{} // package-level func or value -> source
	uses := map[string][]string{} // func -> identifiers it names
	var tests []goTest
	for _, p := range r.glob("test/conformance/*.go") {
		src := r[p]
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		text := func(n ast.Node) string {
			return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil || d.Body == nil {
					continue
				}
				name := d.Name.Name
				bodies[name] = text(d.Body)
				ast.Inspect(d.Body, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						uses[name] = append(uses[name], id.Name)
					}
					return true
				})
				if strings.HasPrefix(name, "Test") && strings.HasSuffix(p, "_test.go") {
					doc := ""
					if d.Doc != nil {
						doc = strings.Join(strings.Fields(d.Doc.Text()), " ")
						if rest, ok := strings.CutPrefix(doc, name+": "); ok {
							doc = capitalize(rest)
						}
					}
					tests = append(tests, goTest{Name: name, File: p, Doc: doc})
				}
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					if vs, ok := sp.(*ast.ValueSpec); ok {
						for _, n := range vs.Names {
							bodies[n.Name] = text(vs)
						}
					}
				}
			}
		}
	}
	for i := range tests {
		var b strings.Builder
		seen := map[string]bool{}
		queue := []string{tests[i].Name}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if seen[n] {
				continue
			}
			seen[n] = true
			if body, ok := bodies[n]; ok {
				b.WriteString(body + "\n")
				queue = append(queue, uses[n]...)
			}
		}
		tests[i].reach = b.String()
	}
	return tests, nil
}

// specReadmeRows reads spec/README.md's table: path pattern -> what it fixes.
func specReadmeRows(readme string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(readme, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		pat := strings.Trim(strings.TrimSpace(cells[1]), "`")
		out = append(out, [2]string{pat, strings.TrimSpace(cells[2])})
	}
	return out
}

// holds says what a file set is: spec/README.md's line for it, else the
// schema's own title, else nothing.
func holds(r repoFiles, set conformanceSet, files []string, readme [][2]string) string {
	sample := files[0]
	for _, row := range readme {
		pat := "spec/" + row[0]
		for strings.Contains(pat, "<") {
			i, j := strings.Index(pat, "<"), strings.Index(pat, ">")
			if j < i {
				break
			}
			pat = pat[:i] + "*" + pat[j+1:]
		}
		if ok, _ := path.Match(pat, sample); ok {
			return row[1]
		}
	}
	if strings.HasSuffix(sample, ".json") && len(files) == 1 {
		if o, err := decodeObj(r[sample]); err == nil && o.str("title") != "" {
			return o.str("title")
		}
	}
	return ""
}

func (s *site) conformancePage(r repoFiles) (*page, error) {
	tests, err := conformanceTests(r)
	if err != nil {
		return nil, err
	}
	if len(tests) == 0 {
		return nil, fmt.Errorf("test/conformance: no Test functions")
	}
	repo, err := r.repoURL()
	if err != nil {
		return nil, err
	}
	readme := specReadmeRows(string(r["spec/README.md"]))

	loads := map[string][]string{} // test -> sets
	var setRows [][]string
	for _, set := range conformanceSets {
		files := r.glob(set.glob)
		if len(files) == 0 {
			continue
		}
		var by []string
		for _, t := range tests {
			for _, m := range set.markers {
				if strings.Contains(t.reach, m) {
					by = append(by, code(t.Name))
					loads[t.Name] = append(loads[t.Name], code(set.glob))
					break
				}
			}
		}
		if len(by) == 0 {
			continue
		}
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = path.Base(f)
			if strings.HasSuffix(set.glob, "/binding.yaml") || strings.Contains(set.glob, "/*/fixtures/") {
				names[i] = strings.TrimPrefix(f, strings.SplitN(set.glob, "*", 2)[0])
			}
		}
		sort.Strings(names)
		setCell := code(set.glob)
		if strings.Contains(set.glob, "*") {
			setCell += fmt.Sprintf(" (%d: %s)", len(files), cell(strings.Join(names, ", ")))
		}
		setRows = append(setRows, []string{setCell, descCell(holds(r, set, files, readme)), strings.Join(by, ", ")})
	}

	var testRows [][]string
	for _, t := range tests {
		l := "—"
		if len(loads[t.Name]) > 0 {
			l = strings.Join(loads[t.Name], ", ")
		}
		testRows = append(testRows, []string{code(t.Name), code(path.Base(t.File)), l, descCell(t.Doc)})
	}

	var b strings.Builder
	b.WriteString("# Conformance kit and fixtures\n\n")
	fmt.Fprintf(&b, "The files under `spec/` and `test/fixtures/` that a test holds the code to, and the %d tests in [test/conformance](%s/tree/main/test/conformance) that load them. `go test ./test/conformance/` runs them alone, and `make check` runs them with the rest of the suite.\n\n", len(tests), repo)
	b.WriteString("## File sets\n\n")
	b.WriteString("What each set holds comes from `spec/README.md`, or from the schema's title. A set is listed under every test whose code, or a helper it calls, reads it.\n\n")
	b.WriteString(table([]string{"Files", "Holds", "Loaded by"}, setRows))
	b.WriteString("\n## Tests\n\n")
	b.WriteString("What each test checks is its doc comment.\n\n")
	b.WriteString(table([]string{"Test", "File", "Kit files it reads", "Checks"}, testRows))
	fmt.Fprintf(&b, "\n[spec/wire/README.md](%s/blob/main/spec/wire/README.md) says how a wire transcript is recorded and which schema each of its lines must meet.\n", repo)
	b.WriteString(seeAlsoList([][2]string{{"Wire protocol", "/reference/wire-protocol"}, {"Binding manifest", "/reference/binding-manifest"}, {"pipeline.yaml keys", "/reference/pipeline-yaml"}, {"Ledger schema and views", "/reference/ledger-schema"}}))
	b.WriteString(s.usedIn("/reference/conformance"))
	return &page{
		path:        "reference/conformance.md",
		node:        "/reference/conformance",
		name:        "Conformance kit and fixtures",
		description: "The spec files and fixtures the code is tested against, what each set holds, and which conformance test loads it, generated from test/conformance",
		audience:    "You're changing the spec, a schema, a transcript, or a binding's fixtures and need to know which test will hold you to it.",
		learn:       []string{"what each spec file set holds", "which conformance test loads each set", "what each conformance test checks"},
		roles:       []string{"extender", "agent"},
		body:        b.String(),
	}, nil
}
