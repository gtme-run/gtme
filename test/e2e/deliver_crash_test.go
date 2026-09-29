package e2e

// M34 acceptance for ADR-060: a deliver step's sends that were in flight
// when the process died are held, not sent again — offline, against a local
// target that counts every request by its Idempotency-Key.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// slowTarget is a delivery target that takes a while per request and counts
// arrivals by Idempotency-Key, so a test can crash gtme mid-step and then
// prove nothing arrived twice.
type slowTarget struct {
	mu      sync.Mutex
	arrived []string
	delay   time.Duration
}

func (s *slowTarget) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.arrived = append(s.arrived, r.Header.Get("Idempotency-Key"))
	s.mu.Unlock()
	select {
	case <-time.After(s.delay):
	case <-r.Context().Done():
	}
	w.Write([]byte(`{"ok":true}`))
}

func (s *slowTarget) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.arrived)
}

// perKey counts arrivals per key since the given arrival index.
func (s *slowTarget) perKey(since int) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for _, k := range s.arrived[since:] {
		out[k]++
	}
	return out
}

func (s *slowTarget) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for s.count() < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d arrivals (have %d)", n, s.count())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fortyCSV is 40 people for a deliver step to send.
func fortyCSV() string {
	var b strings.Builder
	b.WriteString("email,full_name\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "p%02d@example.com,Person %02d\n", i, i)
	}
	return b.String()
}

func sendYAML(use, url string) string {
	return `name: send
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: send
    use: ` + use + `
    with:
      url: "` + url + `/hook"
    variables:
      name: full_name
      contact: email
    idempotency: email
`
}

// crashMidSend starts an armed 40-record send at concurrency 4, waits for
// 12 arrivals, and delivers sig to gtme.
func crashMidSend(t *testing.T, h *harness, target *slowTarget, sig syscall.Signal) string {
	t.Helper()
	cmd, stderr := h.startGtme([]string{"GTME_CONCURRENCY=4"}, "run", "send.yaml")
	target.waitFor(t, 12)
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatalf("signal: %v", err)
	}
	cmd.Wait()
	return stderr.String()
}

func unconfirmedKeys(h *harness) []string {
	return h.queryStrings(`SELECT i.identity_key FROM deliveries d JOIN identities i ON i.id = d.identity_id
	  WHERE d.status = 'unconfirmed' ORDER BY i.identity_key`)
}

func assertOnce(t *testing.T, target *slowTarget) {
	t.Helper()
	for k, n := range target.perKey(0) {
		if k == "" {
			t.Errorf("%d request(s) arrived without an Idempotency-Key", n)
		}
		if n > 1 {
			t.Errorf("Idempotency-Key %s arrived %d times", k, n)
		}
	}
}

func TestKilledDeliverHoldsInFlightSends(t *testing.T) {
	target := &slowTarget{delay: 200 * time.Millisecond}
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("http/deliver", srv.URL))

	crashMidSend(t, h, target, syscall.SIGKILL)
	runID := h.queryStrings(`SELECT id FROM runs`)[0]
	if n := h.queryInt(`SELECT count(*) FROM step_events WHERE event = 'dispatched'`); n >= 40 {
		t.Errorf("dispatched = %d; only the sends that started may be dispatched", n)
	}

	finished := h.queryInt(`SELECT count(*) FROM deliveries`)
	res := h.mustRun("run", "send.yaml", "--resume", runID)
	assertOnce(t, target)
	held := unconfirmedKeys(h)
	if len(held) < 1 || len(held) > 4 {
		t.Fatalf("unconfirmed rows = %d (%v), want 1..4 — one per open session\nstderr:\n%s", len(held), held, res.stderr)
	}
	contains(t, res.stderr, fmt.Sprintf("send: %d in, %d out", 40-finished, 40-finished-len(held)), "stderr")
	contains(t, res.stderr, fmt.Sprintf(", %d unconfirmed", len(held)), "stderr")
	contains(t, res.stderr, held[0], "receipt names the held records")
	contains(t, res.stderr, "Check the target, then: gtme run send.yaml --resume "+runID+" --resend-unconfirmed", "stderr")
	if n := h.queryInt(`SELECT count(*) FROM deliveries WHERE status = 'accepted'`); n != 40-len(held) {
		t.Errorf("accepted deliveries = %d, want %d", n, 40-len(held))
	}
	receipt := h.mustRun("runs", runID)
	contains(t, receipt.stderr, fmt.Sprintf("held:     %d unconfirmed at http/deliver", len(held)), "gtme runs RUN_ID")

	// A fresh run of the same pipeline sends none of them either.
	before := target.count()
	fresh := h.mustRun("run", "send.yaml")
	if got := target.count() - before; got != 0 {
		t.Errorf("a fresh run sent %d request(s), want 0", got)
	}
	contains(t, fresh.stderr, fmt.Sprintf(", %d unconfirmed", len(held)), "fresh run")
	contains(t, fresh.stderr, "--resume "+runID+" --resend-unconfirmed", "fresh run names the run that held them")

	// The finished run still has something to resume: the release. Without
	// the flag it refuses and says how.
	refused := h.run("run", "send.yaml", "--resume", runID)
	if refused.code != 2 {
		t.Errorf("resuming the done run without the flag: exit = %d, want 2", refused.code)
	}
	contains(t, refused.stderr, fmt.Sprintf("is done; it holds %d unconfirmed", len(held)), "stderr")

	// The release sends exactly those, with the same keys, and they turn accepted.
	before = target.count()
	h.mustRun("run", "send.yaml", "--resume", runID, "--resend-unconfirmed")
	sent := target.perKey(before)
	if len(sent) != len(held) {
		t.Errorf("the release sent %d distinct key(s), want %d", len(sent), len(held))
	}
	if n := len(unconfirmedKeys(h)); n != 0 {
		t.Errorf("unconfirmed rows after the release = %d, want 0", n)
	}
	if n := h.queryInt(`SELECT count(*) FROM deliveries WHERE status = 'accepted'`); n != 40 {
		t.Errorf("accepted deliveries = %d, want 40", n)
	}
	// The keys are the same across runs: every released key arrived before
	// or was never sent, and nothing else arrived.
	all := target.perKey(0)
	if len(all) != 40 {
		t.Errorf("distinct keys = %d, want 40 (one per record, stable across runs)", len(all))
	}
}

func TestInterruptedDeliverHoldsAtFinish(t *testing.T) {
	target := &slowTarget{delay: 200 * time.Millisecond}
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("http/deliver", srv.URL))

	out := crashMidSend(t, h, target, syscall.SIGINT)
	held := unconfirmedKeys(h)
	if len(held) < 1 || len(held) > 4 {
		t.Fatalf("unconfirmed rows at finish = %d, want 1..4 without a resume\nstderr:\n%s", len(held), out)
	}
	contains(t, out, fmt.Sprintf(", %d unconfirmed", len(held)), "stderr")
	contains(t, out, fmt.Sprintf("send: %d record(s) may have reached http/deliver", len(held)), "receipt")
	contains(t, out, "gtme: runner: send: interrupted", "stderr")
	if n := h.queryInt(`SELECT count(*) FROM runs WHERE status = 'failed'`); n != 1 {
		t.Errorf("failed runs = %d, want 1", n)
	}

	runID := h.queryStrings(`SELECT id FROM runs`)[0]
	h.mustRun("run", "send.yaml", "--resume", runID)
	assertOnce(t, target)
	if n := len(unconfirmedKeys(h)); n != len(held) {
		t.Errorf("unconfirmed after resume = %d, want %d", n, len(held))
	}
}

// nativeDeliverBinding is a deliver binding whose target upserts, so a send
// repeated after a crash is harmless and ADR-060 re-sends it.
const nativeDeliverBinding = `id: test/upsert
version: 1
role: deliver
entity_type: person
needs:
  dynamic: true
  type: object
  required: [email]
  properties:
    email: { type: string }
config_schema:
  type: object
  additionalProperties: false
  properties:
    url: { type: string }
    variables:
      type: object
      additionalProperties: { type: string, minLength: 1 }
request:
  method: POST
  url: "{{config.url}}"
  headers:
    Idempotency-Key: "{{record.email}}"
  body:
    "$variables": true
idempotency: native
cost:
  per: record
  amount_usd: 0
`

func TestNativeTargetIsResentAfterACrash(t *testing.T) {
	target := &slowTarget{delay: 200 * time.Millisecond}
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.writeBindingYAML("test-upsert", nativeDeliverBinding)
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("test/upsert", srv.URL))

	crashMidSend(t, h, target, syscall.SIGKILL)
	runID := h.queryStrings(`SELECT id FROM runs`)[0]
	unanswered := h.queryInt(`SELECT count(*) FROM step_events d WHERE d.event = 'dispatched'
	  AND NOT EXISTS (SELECT 1 FROM step_events e WHERE e.identity_id = d.identity_id AND e.event = 'done')`)
	if unanswered < 1 {
		t.Fatalf("no send was in flight at the kill")
	}
	res := h.mustRun("run", "send.yaml", "--resume", runID)
	if n := len(unconfirmedKeys(h)); n != 0 {
		t.Errorf("unconfirmed rows = %d, want 0 at a native target\nstderr:\n%s", n, res.stderr)
	}
	if n := h.queryInt(`SELECT count(*) FROM deliveries`); n != 40 {
		t.Errorf("deliveries = %d, want 40", n)
	}
	if strings.Contains(res.stderr, "unconfirmed") {
		t.Errorf("a native target holds nothing\nstderr:\n%s", res.stderr)
	}
}

// TestPlainRunAfterACrashHoldsToo: a plain run after kill -9 — no resume —
// must not send what the dead run had in flight either (ADR-060: no later
// run sends it by habit). It holds them under the dead run's id.
func TestPlainRunAfterACrashHoldsToo(t *testing.T) {
	target := &slowTarget{delay: 200 * time.Millisecond}
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("http/deliver", srv.URL))

	crashMidSend(t, h, target, syscall.SIGKILL)
	dead := h.queryStrings(`SELECT id FROM runs`)[0]
	res := h.mustRun("run", "send.yaml")
	assertOnce(t, target)
	held := unconfirmedKeys(h)
	if len(held) < 1 || len(held) > 4 {
		t.Fatalf("unconfirmed rows = %d, want 1..4\nstderr:\n%s", len(held), res.stderr)
	}
	if n := h.queryInt(`SELECT count(*) FROM deliveries WHERE status = 'unconfirmed' AND run_id = ?`, dead); n != len(held) {
		t.Errorf("held under the dead run = %d, want %d", n, len(held))
	}
	contains(t, res.stderr, "--resume "+dead+" --resend-unconfirmed", "the release names the dead run")
}

// TestInterruptedRunShowsUnansweredSends is #189 (ADR-064): before any run
// holds them, `gtme runs` counts a dead run's unanswered sends in in
// flight, and `gtme runs RUN_ID` names them per target, writing nothing.
func TestInterruptedRunShowsUnansweredSends(t *testing.T) {
	target := &slowTarget{delay: 200 * time.Millisecond}
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("http/deliver", srv.URL))

	crashMidSend(t, h, target, syscall.SIGKILL)
	id := h.queryStrings(`SELECT id FROM runs`)[0]
	open := h.queryInt(`SELECT count(*) FROM step_events d WHERE d.event = 'dispatched'
	  AND NOT EXISTS (SELECT 1 FROM step_events e WHERE e.identity_id = d.identity_id AND e.event IN ('done','failed'))`)
	if open < 1 {
		t.Fatalf("no send was in flight at the kill")
	}
	before := ledgerContent(t, h)
	list := h.mustRun("runs")
	var row []string
	for _, l := range strings.Split(list.stderr, "\n") {
		if strings.HasPrefix(l, id) {
			row = strings.Fields(l)
		}
	}
	if len(row) < 6 || row[2] != "interrupted" || row[5] != fmt.Sprint(open) {
		t.Errorf("gtme runs row = %v, want interrupted with %d in flight", row, open)
	}
	receipt := h.mustRun("runs", id)
	contains(t, receipt.stderr, fmt.Sprintf("send: %d sent to http/deliver with no answer before the run stopped", open), "gtme runs RUN_ID")
	if after := ledgerContent(t, h); after != before {
		t.Errorf("gtme runs wrote to the ledger")
	}
}
