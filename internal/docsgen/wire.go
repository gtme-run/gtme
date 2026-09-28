package docsgen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var wireSchemaRe = regexp.MustCompile("`spec/schemas/(msg-[a-z-]+\\.schema\\.json)`")

// wireProtocolPage renders reference/wire-protocol.md from the message
// schemas, in the order spec/wire/README.md's schema table lists them, with
// the first stream of the golden transcript as the example.
func (s *site) wireProtocolPage(r repoFiles) (*page, error) {
	files := r.glob("spec/schemas/msg-*.schema.json")
	if len(files) == 0 {
		return nil, fmt.Errorf("spec/schemas: no msg-*.schema.json")
	}
	transcripts := r.glob("spec/wire/*.ndjson")
	if len(transcripts) == 0 {
		return nil, fmt.Errorf("spec/wire: no transcript to show as the example")
	}
	repo, err := r.repoURL()
	if err != nil {
		return nil, err
	}

	// Order: the README's schema table (the order an exchange meets them),
	// then any schema it does not list, by name.
	var order []string
	seen := map[string]bool{}
	for _, m := range wireSchemaRe.FindAllStringSubmatch(string(r["spec/wire/README.md"]), -1) {
		p := "spec/schemas/" + m[1]
		if _, ok := r[p]; ok && !seen[p] {
			seen[p] = true
			order = append(order, p)
		}
	}
	for _, p := range files {
		if !seen[p] {
			order = append(order, p)
		}
	}

	type message struct {
		file, name, dir, desc string
		required              []string
		schema                *obj
	}
	var msgs []message
	allType := true
	for _, p := range order {
		schema, err := decodeObj(r[p])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		name, dir := schemaHeadline(schema.str("title"))
		m := message{file: p, name: name, dir: dir, desc: schema.str("description"), schema: schema}
		hasType := false
		if xs, ok := schema.get("required").([]any); ok {
			for _, x := range xs {
				if fmt.Sprint(x) == "type" {
					hasType = true
					continue
				}
				m.required = append(m.required, fmt.Sprint(x))
			}
		}
		allType = allType && hasType
		msgs = append(msgs, m)
	}

	var b strings.Builder
	b.WriteString("# Wire protocol\n\n")
	fmt.Fprintf(&b, "The %d messages a process adapter and the runner exchange, one JSON object per line, from `spec/schemas/msg-*.schema.json`.", len(msgs))
	if allType {
		b.WriteString(" Every message carries `type`, its name, so the required fields listed leave it out.")
	}
	b.WriteString(" " + dottedNote + "\n\n")
	var rows [][]string
	for _, m := range msgs {
		req := "none"
		if len(m.required) > 0 {
			parts := make([]string, len(m.required))
			for i, x := range m.required {
				parts[i] = code(x)
			}
			req = strings.Join(parts, ", ")
		}
		rows = append(rows, []string{code(m.name), cell(m.dir), descCell(firstSentence(m.desc)), req})
	}
	b.WriteString(table([]string{"Message", "Direction", "When it's sent", "Required fields"}, rows))

	for _, m := range msgs {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n", m.name, m.dir)
		if m.desc != "" {
			b.WriteString(prose(strings.Join(strings.Fields(m.desc), " ")) + "\n\n")
		}
		fmt.Fprintf(&b, "Schema: `%s`.\n\n", m.file)
		f := &flattener{root: m.schema}
		b.WriteString(keyTable(f.rows(m.schema, "")))
	}

	// The example: the first transcript, whole and verbatim, one session per
	// adapter in the order they appear.
	tpath := transcripts[0]
	var lines, streams []string
	seenStream := map[string]bool{}
	for _, line := range strings.Split(string(r[tpath]), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var env struct {
			Stream  string `json:"stream"`
			Adapter string `json:"adapter"`
		}
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			return nil, fmt.Errorf("%s: %w", tpath, err)
		}
		if !seenStream[env.Stream] {
			seenStream[env.Stream] = true
			streams = append(streams, fmt.Sprintf("the `%s` session with `%s`", env.Stream, env.Adapter))
		}
		lines = append(lines, line)
	}
	b.WriteString("\n## Example\n\n")
	fmt.Fprintf(&b, "`%s`, recorded from real adapters: %s. Each line wraps the message in `msg`; `stream`, `dir`, and `adapter` say which session it belongs to, which way it went, and the adapter on the other end ([on GitHub](%s/blob/main/%s)).\n\n", tpath, joinAnd(streams), repo, tpath)
	b.WriteString("```json\n" + strings.Join(lines, "\n") + "\n```\n")
	b.WriteString(seeAlsoList([][2]string{{"Binding manifest", "/reference/binding-manifest"}, {"Adapter catalog", "/reference/adapters"}, {"Conformance kit and fixtures", "/reference/conformance"}}))
	b.WriteString(s.usedIn("/reference/wire-protocol"))
	return &page{
		path:        "reference/wire-protocol.md",
		node:        "/reference/wire-protocol",
		name:        "Wire protocol",
		description: "Every message a process adapter and the runner exchange, its direction, when it's sent, and its fields, generated from spec/schemas/msg-*.schema.json",
		audience:    "You're writing a process adapter, or reading one's output, and need each message's direction and fields.",
		learn:       []string{"every message type and which side sends it", "the fields each message carries and which are required", "what a recorded exchange looks like, line by line"},
		roles:       []string{"extender", "agent"},
		body:        b.String(),
	}, nil
}
