package e2e

import (
	"strings"
	"testing"
)

// M36 acceptance for ADR-064: `gtme runs RUN_ID` prints the live receipt's
// table, rebuilt from the ledger.

const mirrorCSV = `full_name,email,title
Jane Doe,jane@acme.com,VP Marketing
Bob Stone,bob@globex.io,Head of Growth
Carol Ray,carol@initech.dev,Engineer
Dana Park,dana@contoso.com,VP Sales
,eli@umbrella.co,VP Ops
Fay Chen,fay@wayne.io,Designer
`

const mirrorYAML = `name: mirror
version: 1
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: score
    use: demo/enrich
    with:
      cost_per_record_usd: 0.01
  - id: keep
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head%')
  - id: out
    use: csv/deliver
    exclude: [dnc]
    with:
      path: out.csv
    variables:
      name: full_name
      email: email
    idempotency: email
`

// receiptBlock is the table, its already-delivered lines and the total
// line, from either the live receipt or `gtme runs RUN_ID`.
func receiptBlock(t *testing.T, stderr string) string {
	t.Helper()
	lines := strings.Split(stderr, "\n")
	var out []string
	in := false
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "step ") && strings.Contains(l, "adapter"):
			in = true
		case strings.HasPrefix(l, "total:"):
			out = append(out, l)
			return strings.Join(out, "\n")
		}
		if in && !strings.HasPrefix(l, " ") && (strings.Contains(l, "  ") || strings.HasSuffix(l, "already delivered")) {
			out = append(out, strings.TrimRight(l, " "))
		}
	}
	t.Fatalf("no receipt table in:\n%s", stderr)
	return ""
}

func TestRunsMirrorsTheReceipt(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", mirrorCSV)
	h.write("mirror.yaml", mirrorYAML)
	h.mustRun("groups", "add", "dnc", "--type", "person")

	first := h.mustRun("run", "mirror.yaml")
	id1 := h.queryStrings(`SELECT id FROM runs ORDER BY id`)[0]
	if got, want := receiptBlock(t, h.mustRun("runs", id1).stderr), receiptBlock(t, first.stderr); got != want {
		t.Errorf("first run: gtme runs differs from the live receipt\ngot:\n%s\nwant:\n%s", got, want)
	}

	// The second run: score is cached, Jane is excluded by a membership
	// gate, the delivered ones are already delivered, and the nameless
	// record is held by on_missing.
	h.mustRun("groups", "add", "dnc", "jane@acme.com")
	h.write("people.csv", mirrorCSV+"Gus Lee,gus@hooli.com,VP Sales\n")
	second := h.mustRun("run", "mirror.yaml")
	ids := h.queryStrings(`SELECT id FROM runs ORDER BY id`)
	id2 := ids[len(ids)-1]
	live := receiptBlock(t, second.stderr)
	for _, want := range []string{"avoided via cache", "already delivered"} {
		if !strings.Contains(live, want) {
			t.Fatalf("the second run's receipt lacks %q; the scenario is not exercising it:\n%s", want, second.stderr)
		}
	}
	if got := receiptBlock(t, h.mustRun("runs", id2).stderr); got != live {
		t.Errorf("second run: gtme runs differs from the live receipt\ngot:\n%s\nwant:\n%s", got, live)
	}
	// Byte-stable (#127): the records: line lists states in step order.
	stable := h.mustRun("runs", id2).stderr
	for i := 0; i < 5; i++ {
		if again := h.mustRun("runs", id2).stderr; again != stable {
			t.Fatalf("gtme runs RUN_ID is not byte-stable:\n%s\n---\n%s", stable, again)
		}
	}
	contains(t, stable, "records: 7 (score=2 keep=1 out=4)", "records line in step order")
	if n := h.queryInt(`SELECT count(*) FROM step_events WHERE run_id = ? AND event = 'gated'`, id2); n != 1 {
		t.Errorf("gated events = %d, want 1 (Jane, excluded)", n)
	}
}

// TestRunsMirrorIsNetAcrossSessions: a run killed once and resumed reads as
// one run — each record counted once per step, by its latest outcome.
func TestRunsMirrorIsNetAcrossSessions(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.writeAdapter("hang-enrich", hangManifest, hangScript)
	marker := h.work + "/hung"
	h.write("hang.yaml", `name: interruptible
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: hang
    use: hang-enrich
    with:
      hang_on: bob@globex.io
      marker: `+marker+`
`)
	cmd, _ := h.startGtme([]string{"GTME_CONCURRENCY=1"}, "run", "hang.yaml")
	waitForFile(t, marker)
	cmd.Process.Kill()
	cmd.Wait()
	id := h.queryStrings(`SELECT id FROM runs`)[0]
	h.mustRun("run", "hang.yaml", "--resume", id)

	block := receiptBlock(t, h.mustRun("runs", id).stderr)
	want := "hang    hang-enrich  3   3    -      0       -         -       $0    -"
	if !strings.Contains(block, want) {
		t.Errorf("the resumed run's hang row is not net across sessions; want %q in:\n%s", want, block)
	}
}

// TestRunsMirrorOfALegacyRun: a run recorded before ADR-064 has no outcome
// on its events and no gated events; its receipt infers the columns and
// prints ? where the ledger never knew.
func TestRunsMirrorOfALegacyRun(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", mirrorCSV)
	h.write("mirror.yaml", mirrorYAML)
	h.mustRun("groups", "add", "dnc", "--type", "person")
	h.mustRun("run", "mirror.yaml")
	h.mustRun("groups", "add", "dnc", "jane@acme.com")
	h.mustRun("run", "mirror.yaml")
	ids := h.queryStrings(`SELECT id FROM runs ORDER BY id`)
	id := ids[len(ids)-1]

	// Make the second run look as M35 recorded it.
	l := h.open()
	for _, q := range []string{
		`DELETE FROM step_events WHERE event = 'gated'`,
		`UPDATE step_events SET detail = json_remove(json_remove(detail, '$.outcome'), '$.avoided_usd') WHERE detail IS NOT NULL`,
	} {
		if _, err := l.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()

	res := h.mustRun("runs", id)
	block := receiptBlock(t, res.stderr)
	// Jane's gate was never recorded: out's in is 3 known plus a floor.
	rows := map[string][]string{}
	for _, l := range strings.Split(block, "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			rows[f[0]] = f
		}
	}
	if f := rows["score"]; len(f) < 10 || f[5] != "6" || f[9] != "?" {
		t.Errorf("legacy score row = %v, want 6 cached and avoided ?", f)
	}
	if f := rows["out"]; len(f) < 3 || f[2] != "3+?" {
		t.Errorf("legacy out row = %v, want in 3+?", f)
	}
	contains(t, block, "out: 2 already delivered", "legacy receipt")
	contains(t, res.stderr, "in+? is a floor", "legacy note")
}
