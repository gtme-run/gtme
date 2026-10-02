package e2e

import "testing"

// TestWhenNamesOnlyAFilter: `when: STEP.passed` reads the filter role only
// (ADR-048). Only a filter writes a pass verdict, so a when: naming any
// other step would hold every record; plan refuses it, naming the step, as
// it refuses a review (#88).
func TestWhenNamesOnlyAFilter(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)

	const head = `name: gates
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
        WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 70
  - id: park
    use: group/deliver
    with:
      group: parked
  - id: out
    use: group/deliver
    with:
      group: kept
`
	for _, tc := range []struct{ ref, role string }{
		{"score", "an enrich"},
		{"park", "a deliver"},
		{"source", "a source"},
	} {
		h.write("gates.yaml", head+"    when: "+tc.ref+".passed\n")
		res := h.run("plan", "gates.yaml")
		if res.code != 2 {
			t.Fatalf("when: %s.passed (%s) exit = %d, want 2\nstderr:\n%s", tc.ref, tc.role, res.code, res.stderr)
		}
		contains(t, res.stderr, "when: "+tc.ref+".passed reads the filter role only", "the refusal names the reference")
		contains(t, res.stderr, "is "+tc.role, "the refusal names the role")
		if n := h.queryInt(`SELECT count(*) FROM runs`); n != 0 {
			t.Fatalf("a refused plan ran: %d runs", n)
		}
	}

	h.write("gates.yaml", head+"    when: keep.passed\n")
	h.mustRun("plan", "gates.yaml")
}
