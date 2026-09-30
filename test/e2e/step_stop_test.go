package e2e

// M37 acceptance for ADR-065: a step that stops. Offline, against local
// targets that count every request.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// verdictBinding is a deliver binding whose errors: block the test supplies.
func verdictBinding(errors string) string {
	return `id: test/dest
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
  body:
    email: "{{record.email}}"
    "$variables": true
` + errors + `
idempotency: ledger
cost:
  per: record
  amount_usd: 0
`
}

// scriptedTarget answers every request with status and body, counting
// requests per email, until the test flips it.
type scriptedTarget struct {
	mu      sync.Mutex
	status  int
	body    string
	byEmail map[string]int
	total   int
}

func newScriptedTarget(status int, body string) *scriptedTarget {
	return &scriptedTarget{status: status, body: body, byEmail: map[string]int{}}
}

func (s *scriptedTarget) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	email := regexp.MustCompile(`"email":"([^"]+)"`).FindStringSubmatch(b.String())
	s.mu.Lock()
	s.total++
	if len(email) == 2 {
		s.byEmail[email[1]]++
	}
	status, body := s.status, s.body
	s.mu.Unlock()
	w.WriteHeader(status)
	w.Write([]byte(body))
}

func (s *scriptedTarget) set(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status, s.body = status, body
}

func (s *scriptedTarget) requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}

// sendCounts finds "send: N in, …" and returns its counts by word.
func sendCounts(t *testing.T, stderr string) map[string]int {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.HasPrefix(line, "send: ") || !strings.Contains(line, " in, ") {
			continue
		}
		counts := map[string]int{}
		body := strings.TrimPrefix(line, "send: ")
		if i := strings.Index(body, " — "); i >= 0 {
			body = body[:i]
		}
		for _, part := range strings.Split(body, ", ") {
			fields := strings.SplitN(part, " ", 2)
			if n, err := strconv.Atoi(fields[0]); err == nil && len(fields) == 2 {
				counts[fields[1]] = n
			}
		}
		return counts
	}
	t.Fatalf("no send step line in:\n%s", stderr)
	return nil
}

func runIDOf(t *testing.T, h *harness) string {
	t.Helper()
	ids := h.queryStrings(`SELECT id FROM runs ORDER BY started_at DESC LIMIT 1`)
	if len(ids) == 0 {
		t.Fatal("no run recorded")
	}
	return ids[0]
}

const failRunErrors = `errors:
  "403": { verdict: fail_run, reason: "the destination is full" }`

// TestFailRunStopsTheStep: a fail_run at the first record sends one request
// for 40 records, counts 39 not sent with the reason, exits with the error's
// class, records detail.stopped, and a resume sends the rest once each.
func TestFailRunStopsTheStep(t *testing.T) {
	target := newScriptedTarget(403, `{"message":"full"}`)
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.writeBindingYAML("test-dest", verdictBinding(failRunErrors))
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("test/dest", srv.URL))

	res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "send.yaml")
	if got := target.requests(); got != 1 {
		t.Errorf("requests = %d, want 1 — the step stops at the first fail_run\nstderr:\n%s", got, res.stderr)
	}
	if res.code != 3 {
		t.Errorf("exit = %d, want 3 (a 403 rule with no match or class keeps auth)", res.code)
	}
	line := sendCounts(t, res.stderr)
	if line["in"] != 40 || line["failed"] != 1 || line["not sent"] != 39 {
		t.Errorf("step line = %v, want 40 in, 1 failed, 39 not sent\nstderr:\n%s", line, res.stderr)
	}
	contains(t, res.stderr, "39 not sent — stopped: the destination is full", "the step line names the stop")
	runID := runIDOf(t, h)
	contains(t, res.stderr, "39 records were not sent. Fix the cause, then: gtme run send.yaml --resume "+runID, "the resume command")
	stopped := h.queryStrings(`SELECT json_extract(detail, '$.stopped.verdict') || '|' || json_extract(detail, '$.stopped.reason') || '|' || json_extract(detail, '$.stopped.not_sent')
	  FROM step_events WHERE step_id = 'send' AND identity_id IS NULL AND event = 'failed' AND json_extract(detail, '$.stopped') IS NOT NULL`)
	if len(stopped) != 1 || stopped[0] != "fail_run|the destination is full|39" {
		t.Errorf("detail.stopped = %v, want one fail_run|the destination is full|39", stopped)
	}
	if n := h.queryInt(`SELECT count(*) FROM deliveries`); n != 0 {
		t.Errorf("deliveries = %d after a refused send, want 0", n)
	}

	// gtme runs RUN_ID rebuilds the line from the ledger (ADR-064).
	report := h.run("runs", runID)
	contains(t, report.stderr+report.stdout, "send: 39 not sent — stopped: the destination is full", "gtme runs RUN_ID")

	target.set(200, `{"ok":true}`)
	again := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "send.yaml", "--resume", runID)
	if again.code != 0 {
		t.Fatalf("resume exit = %d\nstderr:\n%s", again.code, again.stderr)
	}
	if n := h.queryInt(`SELECT count(*) FROM deliveries`); n != 40 {
		t.Errorf("deliveries after the resume = %d, want 40", n)
	}
	target.mu.Lock()
	defer target.mu.Unlock()
	if len(target.byEmail) != 40 {
		t.Errorf("distinct emails = %d, want 40", len(target.byEmail))
	}
	// The refused record reached the target twice (refused, then sent on the
	// resume); every other record once.
	twice := 0
	for email, n := range target.byEmail {
		switch {
		case n == 2:
			twice++
		case n != 1:
			t.Errorf("%s reached the target %d times", email, n)
		}
	}
	if twice != 1 {
		t.Errorf("%d records reached the target twice, want only the refused one", twice)
	}
}

// At concurrency 4 the sessions already open finish: at most 4 requests,
// and the line still reconciles.
func TestFailRunStopsTheStepAtConcurrency4(t *testing.T) {
	target := newScriptedTarget(403, `{"message":"full"}`)
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.writeBindingYAML("test-dest", verdictBinding(failRunErrors))
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("test/dest", srv.URL))

	res := h.runWithEnv([]string{"GTME_CONCURRENCY=4"}, "", "run", "send.yaml")
	if got := target.requests(); got > 4 || got < 1 {
		t.Errorf("requests = %d, want 1..4\nstderr:\n%s", got, res.stderr)
	}
	line := sendCounts(t, res.stderr)
	if line["in"] != 40 || line["failed"]+line["not sent"] != 40 || line["failed"] != target.requests() {
		t.Errorf("step line = %v with %d requests; want failed = requests and failed + not sent = 40", line, target.requests())
	}
}

// fail_record and skip on a deliver binding: every record is sent and
// refused, and none is recorded as delivered.
func TestRecordVerdictsWriteNoDelivery(t *testing.T) {
	for _, tc := range []struct {
		verdict string
		word    string
		code    int
	}{
		{"fail_record", "failed", 0},
		{"skip", "skipped", 0},
	} {
		t.Run(tc.verdict, func(t *testing.T) {
			target := newScriptedTarget(403, `{"message":"no"}`)
			srv := httptest.NewServer(target)
			defer srv.Close()
			h := newHarness(t)
			h.writeBindingYAML("test-dest", verdictBinding(`errors:
  "403": { verdict: `+tc.verdict+`, reason: "refused" }`))
			h.write("people.csv", fortyCSV())
			h.write("send.yaml", sendYAML("test/dest", srv.URL))
			res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "send.yaml")
			if got := target.requests(); got != 40 {
				t.Errorf("requests = %d, want 40", got)
			}
			if n := h.queryInt(`SELECT count(*) FROM deliveries`); n != 0 {
				t.Errorf("deliveries = %d, want 0 — a refused record is never a send", n)
			}
			line := sendCounts(t, res.stderr)
			if line[tc.word] != 40 || line["out"] != 0 {
				t.Errorf("step line = %v, want 40 %s, 0 out\nstderr:\n%s", line, tc.word, res.stderr)
			}
			if res.code != tc.code {
				t.Errorf("exit = %d, want %d\nstderr:\n%s", res.code, tc.code, res.stderr)
			}
			// Record failures do not fail the run (§5, §8; M37 as amended).
			if st := h.queryStrings(`SELECT status FROM runs`); len(st) != 1 || st[0] != "done" {
				t.Errorf("run status = %v, want done", st)
			}
		})
	}
}

const matchErrors = `errors:
  "403":
    - match: "Lead limit reached"
      verdict: fail_run
      reason: "the workspace is full"
    - match: "invalid key"
      verdict: fail_run
      class: auth
      reason: "the key was rejected"
    - verdict: fail_record
      reason: "forbidden"`

// A match rule applies only when its text is in the body; otherwise the
// next rule does. A matched rule's run exits 1, or 3 with class: auth.
func TestMatchRulesPickTheVerdictAndTheExitCode(t *testing.T) {
	for _, tc := range []struct {
		body, want string
		code, reqs int
	}{
		{`{"message":"Lead limit reached. Remaining uploads: 0"}`, "stopped: the workspace is full", 1, 1},
		{`{"message":"invalid key"}`, "stopped: the key was rejected", 3, 1},
		{`{"message":"nope"}`, "forbidden", 0, 40},
	} {
		t.Run(tc.want, func(t *testing.T) {
			target := newScriptedTarget(403, tc.body)
			srv := httptest.NewServer(target)
			defer srv.Close()
			h := newHarness(t)
			h.writeBindingYAML("test-dest", verdictBinding(matchErrors))
			h.write("people.csv", fortyCSV())
			h.write("send.yaml", sendYAML("test/dest", srv.URL))
			res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "send.yaml")
			if res.code != tc.code {
				t.Errorf("exit = %d, want %d\nstderr:\n%s", res.code, tc.code, res.stderr)
			}
			if got := target.requests(); got != tc.reqs {
				t.Errorf("requests = %d, want %d", got, tc.reqs)
			}
			contains(t, res.stderr, tc.want, "the rule that applied")
		})
	}
}

// A 401 with no rule is an auth failure: one request, exit 3, the rest not
// sent.
func TestAuthFailureStopsTheStep(t *testing.T) {
	target := newScriptedTarget(401, `{"message":"bad key"}`)
	srv := httptest.NewServer(target)
	defer srv.Close()
	h := newHarness(t)
	h.writeBindingYAML("test-dest", verdictBinding(""))
	h.write("people.csv", fortyCSV())
	h.write("send.yaml", sendYAML("test/dest", srv.URL))
	res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "send.yaml")
	if got := target.requests(); got != 1 {
		t.Errorf("requests = %d, want 1\nstderr:\n%s", got, res.stderr)
	}
	if res.code != 3 {
		t.Errorf("exit = %d, want 3", res.code)
	}
	if line := sendCounts(t, res.stderr); line["not sent"] != 39 {
		t.Errorf("step line = %v, want 39 not sent", line)
	}
}

// fillingInstantly is the Instantly fake with a workspace lead limit.
type fillingInstantly struct {
	mu       sync.Mutex
	capacity int
	emails   map[string]int
	refused  int
}

func (f *fillingInstantly) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v2/campaigns/"):
		fmt.Fprintf(w, `{"id":%q,"name":"Filling","status":1}`, fakeCampaignID)
	case r.Method == "POST" && r.URL.Path == "/api/v2/leads":
		added := 0
		for _, n := range f.emails {
			added += n
		}
		if added >= f.capacity {
			f.refused++
			w.WriteHeader(403)
			fmt.Fprint(w, `{"statusCode":403,"error":"Forbidden","message":"Lead limit reached. Remaining uploads: 0"}`)
			return
		}
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		email := regexp.MustCompile(`"email":"([^"]+)"`).FindStringSubmatch(b.String())
		if len(email) == 2 {
			f.emails[email[1]]++
		}
		fmt.Fprintf(w, `{"id":"lead-%d","campaign":%q}`, added+1, fakeCampaignID)
	default:
		http.NotFound(w, r)
	}
}

// TestInstantlyStopsWhenTheWorkspaceIsFull: the fake fills after 5 leads; the
// adapter adds 5, fails one, reports the rest not sent, and a resume once
// there is room adds them without adding anyone twice.
func TestInstantlyStopsWhenTheWorkspaceIsFull(t *testing.T) {
	fake := &fillingInstantly{capacity: 5, emails: map[string]int{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	h := newHarness(t)
	var csv strings.Builder
	csv.WriteString("email,full_name\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&csv, "p%02d@example.com,Person %02d\n", i, i)
	}
	h.write("people.csv", csv.String())
	h.write("send.yaml", `name: send
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: send
    use: instantly/add-to-campaign
    with:
      campaign: "`+fakeCampaignID+`"
      base_url: "`+srv.URL+`"
    variables:
      first_name: full_name
    idempotency: email
`)
	env := []string{"INSTANTLY_API_KEY=test-key", "GTME_CONCURRENCY=1"}
	res := h.runWithEnv(env, "", "run", "send.yaml")
	if res.code != 1 {
		t.Errorf("exit = %d, want 1 (a full plan is a provider error)\nstderr:\n%s", res.code, res.stderr)
	}
	if len(fake.emails) != 5 || fake.refused != 1 {
		t.Errorf("added %d, refused %d; want 5 added, 1 refused\nstderr:\n%s", len(fake.emails), fake.refused, res.stderr)
	}
	line := sendCounts(t, res.stderr)
	if line["out"] != 5 || line["failed"] != 1 || line["not sent"] != 4 {
		t.Errorf("step line = %v, want 5 out, 1 failed, 4 not sent\nstderr:\n%s", line, res.stderr)
	}
	contains(t, res.stderr, "stopped: instantly: the workspace's lead limit is reached", "the stop names the plan")

	fake.mu.Lock()
	fake.capacity = 100
	fake.mu.Unlock()
	again := h.runWithEnv(env, "", "run", "send.yaml", "--resume", runIDOf(t, h))
	if again.code != 0 {
		t.Fatalf("resume exit = %d\nstderr:\n%s", again.code, again.stderr)
	}
	if len(fake.emails) != 10 {
		t.Errorf("leads after the resume = %d, want 10", len(fake.emails))
	}
	for email, n := range fake.emails {
		if n != 1 {
			t.Errorf("%s added %d times", email, n)
		}
	}
}

const errorEnrichManifest = `{
  "id": "error-enrich",
  "version": 1,
  "role": "enrich",
  "entity_type": "person",
  "needs": {"type":"object","required":["email"],"properties":{"email":{"type":"string"}}},
  "provides": {"type":"object","additionalProperties":false,"properties":{"headline":{"type":"string"}}}
}`

// errorEnrichScript fails p01 with ERROR fail_record when the runner accepts
// it, and answers everyone else.
const errorEnrichScript = `#!/usr/bin/env python3
import json, sys
PROVIDES = {"type":"object","additionalProperties":False,"properties":{"headline":{"type":"string"}}}
accepts = False
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    msg = json.loads(line)
    if msg.get("type") == "OPEN":
        accepts = "ERROR" in (msg.get("accepts") or [])
        print(json.dumps({"type":"SCHEMA","provides":PROVIDES}), flush=True)
    elif msg.get("type") == "RECORD":
        if msg["key"]["identity_key"] == "p01@example.com":
            if not accepts:
                sys.exit(1)
            print(json.dumps({"type":"ERROR","key":msg["key"],"verdict":"fail_record","reason":"no such person"}), flush=True)
            continue
        print(json.dumps({"type":"RECORD","key":msg["key"],"fields":{"headline":"fixture"}}), flush=True)
    elif msg.get("type") == "END":
        break
print(json.dumps({"type":"END"}), flush=True)
`

// A process adapter's ERROR fail_record fails that record while the others
// in the same session advance.
func TestProcessAdapterFailRecord(t *testing.T) {
	h := newHarness(t)
	h.writeAdapter("error-enrich", errorEnrichManifest, errorEnrichScript)
	var csv strings.Builder
	csv.WriteString("email,full_name\n")
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&csv, "p%02d@example.com,Person %02d\n", i, i)
	}
	h.write("people.csv", csv.String())
	h.write("p.yaml", `name: enrich
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: look
    use: error-enrich
`)
	res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "p.yaml")
	contains(t, res.stderr, "look: 5 in, 4 out, 0 cached, 0 filtered, 1 failed", "one record failed, the rest advanced")
	contains(t, res.stderr, "look: 1 failed — no such person", "the reason")
}
