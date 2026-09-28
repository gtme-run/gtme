package docsgen

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The single reference pages: one file each under docs/reference/, each a
// function of one spec artifact (or one directory of them) read at
// generation time. Their slugs are the outline's; rewriteOutline marks each
// `generated`.
var singleSlugs = []string{"pipeline-yaml", "bundles", "plugin-skills", "binding-manifest", "wire-protocol", "conformance"}

// repoFiles is Inputs.Repo with the lookups the renderers share.
type repoFiles map[string][]byte

func (r repoFiles) need(p string) ([]byte, error) {
	b, ok := r[p]
	if !ok {
		return nil, fmt.Errorf("%s: missing; the reference page built from it cannot be generated", p)
	}
	return b, nil
}

// glob returns the repo paths matching a slash pattern, sorted.
func (r repoFiles) glob(pattern string) []string {
	var out []string
	for p := range r {
		if ok, _ := path.Match(pattern, p); ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// repoURL is the repository's GitHub URL, from the plugin manifest.
func (r repoFiles) repoURL() (string, error) {
	b, err := r.need("plugin/.claude-plugin/plugin.json")
	if err != nil {
		return "", err
	}
	var m struct {
		Repository string `json:"repository"`
	}
	if err := json.Unmarshal(b, &m); err != nil || m.Repository == "" {
		return "", fmt.Errorf("plugin/.claude-plugin/plugin.json: no repository URL")
	}
	return strings.TrimSuffix(m.Repository, "/"), nil
}

func (s *site) singlePages(repo map[string][]byte) ([]*page, error) {
	r := repoFiles(repo)
	var out []*page
	for _, f := range []func(repoFiles) (*page, error){
		s.pipelineYAMLPage, s.bundlesPage, s.pluginSkillsPage,
		s.bindingManifestPage, s.wireProtocolPage, s.conformancePage,
	} {
		p, err := f(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// seeAlsoList renders a short list of related reference nodes.
func seeAlsoList(items [][2]string) string {
	var b strings.Builder
	b.WriteString("\n## See also\n\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- [%s](%s)\n", it[0], it[1])
	}
	return b.String()
}

const dottedNote = "A dotted key sits inside the key before the dot, and an upper-case part such as `FIELD` stands for a name you choose."

// ---------------------------------------------------------------------------
// pipeline.yaml keys

func (s *site) pipelineYAMLPage(r repoFiles) (*page, error) {
	const src = "spec/schemas/pipeline.schema.json"
	raw, err := r.need(src)
	if err != nil {
		return nil, err
	}
	schema, err := decodeObj(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	hello, err := r.need("examples/hello.yaml")
	if err != nil {
		return nil, err
	}
	f := &flattener{root: schema, label: func(ref string) string {
		if ref == "#/definitions/step" {
			return "step (see Step keys)"
		}
		return ""
	}}
	top := f.rows(schema, "")
	step := f.rows(schema.child("definitions").child("step"), "")

	var b strings.Builder
	b.WriteString("# pipeline.yaml keys\n\n")
	fmt.Fprintf(&b, "Every key `%s` allows in a pipeline file: the top-level keys first, then the keys of a step, which `source:` and every entry of `steps:` share. %s\n\n", src, dottedNote)
	b.WriteString(prose(schema.str("description")) + "\n\n")
	b.WriteString("## Top-level keys\n\n")
	b.WriteString(keyTable(top))
	b.WriteString("\n## Step keys\n\n")
	b.WriteString(keyTable(step))

	var doc struct {
		Steps []struct {
			ID string `yaml:"id"`
		} `yaml:"steps"`
	}
	if err := yaml.Unmarshal(hello, &doc); err != nil {
		return nil, fmt.Errorf("examples/hello.yaml: %w", err)
	}
	ids := make([]string, 0, len(doc.Steps))
	for _, st := range doc.Steps {
		ids = append(ids, code(st.ID))
	}
	b.WriteString("\n## Example\n\n")
	fmt.Fprintf(&b, "From `examples/hello.yaml`, without its header comment: a source and %d steps, %s.\n\n", len(ids), joinAnd(ids))
	b.WriteString("```yaml\n" + stripHeaderComment(string(hello)) + "\n```\n")
	b.WriteString(seeAlsoList([][2]string{{"`gtme plan`", "/reference/cli/plan"}, {"Adapter catalog", "/reference/adapters"}, {"Canonical field registry", "/reference/fields"}}))
	b.WriteString(s.usedIn("/reference/pipeline-yaml"))
	return &page{
		path:        "reference/pipeline-yaml.md",
		node:        "/reference/pipeline-yaml",
		name:        "pipeline.yaml keys",
		description: "Every key a pipeline file accepts, top level and per step, with its type and whether it's required, generated from spec/schemas/pipeline.schema.json",
		audience:    "You're writing or reading a pipeline file and need the exact key, what it takes, and where it's allowed.",
		learn:       []string{"every top-level key and every step key", "which keys are required and what type each takes", "a whole pipeline file that uses them"},
		roles:       []string{"builder", "agent"},
		body:        b.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// Binding manifest

func (s *site) bindingManifestPage(r repoFiles) (*page, error) {
	const src, msrc = "spec/binding-schema.json", "spec/schemas/manifest.schema.json"
	raw, err := r.need(src)
	if err != nil {
		return nil, err
	}
	schema, err := decodeObj(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	mraw, err := r.need(msrc)
	if err != nil {
		return nil, err
	}
	mschema, err := decodeObj(mraw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msrc, err)
	}
	examples := r.glob("spec/bindings/*/binding.yaml")
	if len(examples) == 0 {
		return nil, fmt.Errorf("spec/bindings: no binding.yaml to show as the example")
	}
	example := examples[0]

	f := &flattener{root: schema}
	mf := &flattener{root: mschema}
	props, mprops := schema.child("properties"), mschema.child("properties")
	req := map[string]bool{}
	if xs, ok := schema.get("required").([]any); ok {
		for _, x := range xs {
			req[fmt.Sprint(x)] = true
		}
	}

	// A block is a key only a binding has, with keys of its own; the rest
	// is the manifest surface a binding shares with a process adapter.
	var surface [][]string
	var blocks []string
	for _, name := range props.keys {
		p := props.child(name)
		if !mprops.has(name) && f.complex(p) {
			blocks = append(blocks, name)
			continue
		}
		required := "no"
		if req[name] {
			required = "yes"
		}
		for _, row := range f.row(name, name, p, required) {
			desc := row.Description
			shared := "binding only"
			if mprops.has(name) {
				shared = "same key"
				if desc == "" && !strings.Contains(row.Key, ".") {
					if md := mprops.child(name).str("description"); md != "" {
						desc = "Process manifest: " + md
					}
				}
			}
			surface = append(surface, []string{code(row.Key), cell(row.Type), row.Required, descCell(desc), shared})
		}
	}

	var b strings.Builder
	b.WriteString("# Binding manifest\n\n")
	fmt.Fprintf(&b, "Every key `%s` allows in a `binding.yaml`, block by block, with the keys it shares with a process adapter's `manifest.json` (`%s`). %s\n\n", src, msrc, dottedNote)
	b.WriteString(prose(schema.str("description")) + "\n\n")
	b.WriteString("## Manifest surface\n\n")
	b.WriteString("The keys that say what the binding is. `Process manifest` says whether a process adapter's manifest has the same key; where the binding schema gives no description, the process manifest's is shown.\n\n")
	b.WriteString(table([]string{"Key", "Type", "Required", "Description", "Process manifest"}, surface))

	var only [][]string
	for _, name := range mprops.keys {
		if props.has(name) {
			continue
		}
		p := mprops.child(name)
		only = append(only, []string{code(name), cell(mf.typeOf(p)), descCell(p.str("description"))})
	}
	if len(only) > 0 {
		b.WriteString("\n## Process manifest keys a binding does not take\n\n")
		b.WriteString(table([]string{"Key", "Type", "Description"}, only))
	}

	for _, name := range blocks {
		p := props.child(name)
		fmt.Fprintf(&b, "\n## %s\n\n", capitalize(strings.ReplaceAll(name, "_", " ")))
		required := "optional"
		if req[name] {
			required = "required"
		}
		fmt.Fprintf(&b, "The `%s` block, %s.", name, required)
		if d := p.str("description"); d != "" {
			b.WriteString(" " + prose(d))
		}
		b.WriteString("\n\n")
		rows := f.children(name, name, f.resolve(p))
		b.WriteString(keyTable(rows))
	}

	yamlText, err := r.need(example)
	if err != nil {
		return nil, err
	}
	b.WriteString("\n## Example\n\n")
	fmt.Fprintf(&b, "From `%s`, without its header comment:\n\n", example)
	b.WriteString("```yaml\n" + stripHeaderComment(string(yamlText)) + "\n```\n")
	b.WriteString(seeAlsoList([][2]string{{"Adapter catalog", "/reference/adapters"}, {"Wire protocol", "/reference/wire-protocol"}, {"Conformance kit and fixtures", "/reference/conformance"}, {"`gtme adapters`", "/reference/cli/adapters"}}))
	b.WriteString(s.usedIn("/reference/binding-manifest"))
	return &page{
		path:        "reference/binding-manifest.md",
		node:        "/reference/binding-manifest",
		name:        "Binding manifest",
		description: "Every key of a binding.yaml, block by block, and which keys it shares with a process adapter's manifest, generated from spec/binding-schema.json",
		audience:    "You're writing a binding for a vendor gtme doesn't ship and need the exact key, its type, and what the engine does with it.",
		learn:       []string{"every key a binding accepts, block by block", "which keys a binding shares with a process adapter's manifest", "a complete binding to start from"},
		roles:       []string{"extender", "agent"},
		body:        b.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// Bundles

func (s *site) bundlesPage(r repoFiles) (*page, error) {
	const src = "spec/bundle-manifest.json"
	raw, err := r.need(src)
	if err != nil {
		return nil, err
	}
	schema, err := decodeObj(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	f := &flattener{root: schema}
	var b strings.Builder
	b.WriteString("# Bundles\n\n")
	fmt.Fprintf(&b, "Every key of a campaign bundle's `manifest.json`, from `%s`. %s\n\n", src, dottedNote)
	b.WriteString(prose(schema.str("description")) + "\n\n")
	b.WriteString("## Keys\n\n")
	b.WriteString(keyTable(f.rows(schema, "")))
	// The example: the first single-folder pattern bundle under bundles/.
	if ex := r.glob("bundles/*/manifest.json"); len(ex) > 0 {
		b.WriteString("\n## Example\n\n")
		fmt.Fprintf(&b, "From `%s`, a pattern bundle frozen from a run:\n\n", ex[0])
		b.WriteString("```json\n" + strings.TrimRight(string(r[ex[0]]), "\n") + "\n```\n")
	}
	b.WriteString(seeAlsoList([][2]string{{"`gtme freeze`", "/reference/cli/freeze"}, {"`gtme run`", "/reference/cli/run"}, {"Conformance kit and fixtures", "/reference/conformance"}}))
	b.WriteString(s.usedIn("/reference/bundles"))
	return &page{
		path:        "reference/bundles.md",
		node:        "/reference/bundles",
		name:        "Bundles",
		description: "Every key of a campaign bundle's manifest.json, whether it's required, and a real one from a pattern bundle, generated from spec/bundle-manifest.json",
		audience:    "You're reading, checking, or sharing a frozen campaign bundle and need what its manifest.json records.",
		learn:       []string{"every key of a bundle manifest", "which keys are required", "a real manifest from a pattern bundle"},
		roles:       []string{"operator", "builder", "agent"},
		body:        b.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// Plugin skills

func (s *site) pluginSkillsPage(r repoFiles) (*page, error) {
	raw, err := r.need("plugin/.claude-plugin/plugin.json")
	if err != nil {
		return nil, err
	}
	var plugin struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &plugin); err != nil || plugin.Name == "" {
		return nil, fmt.Errorf("plugin/.claude-plugin/plugin.json: no name")
	}
	repo, err := r.repoURL()
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, p := range r.glob("plugin/skills/*/SKILL.md") {
		fm, _, err := splitFrontmatter(string(r[p]))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		var skill struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(fm), &skill); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if skill.Name == "" {
			return nil, fmt.Errorf("%s: no name", p)
		}
		rows = append(rows, []string{
			code("/" + plugin.Name + ":" + skill.Name),
			fmt.Sprintf("[%s](%s/blob/main/%s)", code(skill.Name), repo, p),
			descCell(skill.Description),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("plugin/skills: no SKILL.md")
	}
	var b strings.Builder
	b.WriteString("# Plugin skills\n\n")
	fmt.Fprintf(&b, "The %d skills in the `%s` plugin for Claude Code, from each `plugin/skills/*/SKILL.md`. Claude Code fires a skill on its own when a request matches its description, and you can call it by its slash command.\n\n", len(rows), plugin.Name)
	b.WriteString(table([]string{"Command", "Skill", "Fires when"}, rows))
	b.WriteString("\n## Install\n\n")
	if from := s.pageWithBlock("/plugin install"); from != nil {
		fmt.Fprintf(&b, "[%s](%s) has the plugin install.\n", from.Name, from.Node)
	} else {
		fmt.Fprintf(&b, "The plugin install is in [plugin/README.md](%s/blob/main/plugin/README.md).\n", repo)
	}
	b.WriteString(seeAlsoList([][2]string{{"CLI", "/reference/cli"}, {"`gtme help`", "/reference/cli/help"}}))
	b.WriteString(s.usedIn("/reference/plugin-skills"))
	return &page{
		path:        "reference/plugin-skills.md",
		node:        "/reference/plugin-skills",
		name:        "Plugin skills",
		description: "Each skill in gtme's Claude Code plugin, its slash command, and when it fires, generated from plugin/skills",
		audience:    "You use gtme from Claude Code and want to know which slash command does what, or when a skill fires on its own.",
		learn:       []string{"each skill's slash command", "when each skill fires", "where the plugin install is documented"},
		roles:       []string{"operator", "builder", "extender", "agent"},
		body:        b.String(),
	}, nil
}

// stripHeaderComment drops a YAML file's leading comment block (the prose
// its author wrote above the document; a page shows the document) and the
// blank lines after it. A comment line at column 0 would otherwise read as
// a heading to the docs lint.
func stripHeaderComment(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && (strings.HasPrefix(lines[i], "#") || strings.TrimSpace(lines[i]) == "") {
		i++
	}
	return strings.TrimRight(strings.Join(lines[i:], "\n"), "\n")
}

// pageWithBlock finds the first authored page with a code block containing s.
func (s *site) pageWithBlock(needle string) *docPage {
	for _, p := range s.pages {
		for _, b := range p.Blocks {
			if strings.Contains(b.Text, needle) {
				return p
			}
		}
	}
	return nil
}
