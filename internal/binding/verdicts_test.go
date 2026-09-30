package binding

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/adapters/adaptertest"
	"github.com/gtme-run/gtme/internal/httpx"
	"github.com/gtme-run/gtme/internal/protocol"
)

// bodyDoer answers every request with one status and body.
type bodyDoer struct {
	status int
	body   string
	calls  int
}

func (d *bodyDoer) Do(req *http.Request) (*http.Response, error) {
	d.calls++
	return &http.Response{
		StatusCode: d.status,
		Status:     http.StatusText(d.status),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(d.body)),
		Request:    req,
	}, nil
}

func runVerdict(t *testing.T, tail string, doer httpx.Doer, noAccepts bool) ([]protocol.Message, error) {
	t.Helper()
	b, err := Parse([]byte(errorsBinding + tail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return adaptertest.Run(t, &Engine{B: b, HTTP: doer}, adaptertest.Input{
		Records:   []protocol.Message{adaptertest.Record("jane@acme.com", map[string]any{"email": "jane@acme.com"})},
		NoAccepts: noAccepts,
	})
}

// oneError asserts exactly one ERROR, keyed to the record, and returns it.
func oneError(t *testing.T, msgs []protocol.Message, verdict string) protocol.Message {
	t.Helper()
	errs := adaptertest.Errors(msgs)
	if len(errs) != 1 {
		t.Fatalf("ERROR messages = %+v, want one %s", errs, verdict)
	}
	e := errs[0]
	if e.Verdict != verdict {
		t.Errorf("verdict = %q, want %q", e.Verdict, verdict)
	}
	if e.Key == nil || e.Key.IdentityKey != "jane@acme.com" {
		t.Errorf("ERROR key = %+v, want the record's", e.Key)
	}
	if e.Reason == "" {
		t.Error("ERROR carries no reason")
	}
	if len(adaptertest.Records(msgs)) != 0 {
		t.Errorf("a record named by ERROR was also answered: %+v", adaptertest.Records(msgs))
	}
	return e
}

// ADR-065: the engine reports every verdict it applies as ERROR. fail_record
// and skip name the record and the session carries on.
func TestFailRecordAndSkipAreReportedAsError(t *testing.T) {
	for _, verdict := range []string{"fail_record", "skip"} {
		t.Run(verdict, func(t *testing.T) {
			msgs, err := runVerdict(t, `
errors:
  "404": { verdict: `+verdict+`, reason: not on file }
`, &bodyDoer{status: 404, body: `{"error":"nope"}`}, false)
			if err != nil {
				t.Fatalf("session failed: %v; %s carries on", err, verdict)
			}
			if e := oneError(t, msgs, verdict); e.Reason != "not on file" {
				t.Errorf("reason = %q, want the rule's", e.Reason)
			}
		})
	}
}

// A runner that does not list ERROR in accepts cannot be relied on to act on
// it, so the engine fails the session instead (SPEC §5).
func TestNoAcceptsFallsBackToFailingTheSession(t *testing.T) {
	msgs, err := runVerdict(t, `
errors:
  "404": { verdict: fail_record, reason: not on file }
`, &bodyDoer{status: 404, body: `{}`}, true)
	if err == nil {
		t.Fatal("session succeeded without accepts; a fail_record must fail it")
	}
	if len(adaptertest.Errors(msgs)) != 0 {
		t.Errorf("ERROR sent to a runner that did not accept it: %+v", adaptertest.Errors(msgs))
	}
}

// fail_run: ERROR fail_run for the record, then the session ends with the
// classified error, so its exit code is the run's.
func TestFailRunIsReportedThenEndsTheSession(t *testing.T) {
	msgs, err := runVerdict(t, `
errors:
  "403": { verdict: fail_run, reason: the destination is full }
`, &bodyDoer{status: 403, body: `{"message":"full"}`}, false)
	if err == nil {
		t.Fatal("session succeeded; fail_run ends it")
	}
	oneError(t, msgs, "fail_run")
	if code := httpx.ExitCodeFor(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (a 403 rule without match or class keeps auth)", code)
	}
}

// A retry that runs out fails the record, reported as retry (SPEC §10a), and
// the session carries on.
func TestRetryExhaustedIsReportedAsRetry(t *testing.T) {
	noSleep(t)
	doer := &bodyDoer{status: 409, body: `{}`}
	msgs, err := runVerdict(t, `
errors:
  "409": { verdict: retry, reason: index still building }
retry:
  max_attempts: 2
`, doer, false)
	if err != nil {
		t.Fatalf("session failed: %v; an exhausted retry fails the record, not the session", err)
	}
	if doer.calls != 2 {
		t.Errorf("calls = %d, want max_attempts = 2", doer.calls)
	}
	e := oneError(t, msgs, "retry")
	if !strings.Contains(e.Reason, "index still building") || !strings.Contains(e.Reason, "2 attempt") {
		t.Errorf("reason = %q, want the rule's reason and the attempt count", e.Reason)
	}
}

const matchRules = `
errors:
  "403":
    - match: "Lead limit reached"
      verdict: fail_run
      reason: the workspace is full
    - match: "invalid key"
      verdict: fail_run
      class: auth
      reason: the key was rejected
    - verdict: fail_record
      reason: forbidden
`

// A match rule applies only when its text is in the body; otherwise the
// next rule does. A matched rule without class exits 1; class: auth exits 3.
func TestMatchRulesAndClass(t *testing.T) {
	cases := []struct {
		body, verdict, reason string
		code                  int
	}{
		{`{"message":"Lead limit reached. Remaining uploads: 0"}`, "fail_run", "the workspace is full", 1},
		{`{"message":"invalid key"}`, "fail_run", "the key was rejected", 3},
		{`{"message":"nope"}`, "fail_record", "forbidden", 0},
	}
	for _, c := range cases {
		t.Run(c.verdict+"/"+c.reason, func(t *testing.T) {
			msgs, err := runVerdict(t, matchRules, &bodyDoer{status: 403, body: c.body}, false)
			if e := oneError(t, msgs, c.verdict); e.Reason != c.reason {
				t.Errorf("reason = %q, want %q", e.Reason, c.reason)
			}
			if c.code == 0 {
				if err != nil {
					t.Errorf("session failed: %v", err)
				}
				return
			}
			if code := httpx.ExitCodeFor(err); code != c.code {
				t.Errorf("exit code = %d, want %d (err %v)", code, c.code, err)
			}
		})
	}
}

// class: auth on a record-level verdict still reports the record, then ends
// the session as an auth failure, so the runner stops the step (SPEC §10a).
func TestClassAuthOnFailRecordEndsTheSessionAsAuth(t *testing.T) {
	msgs, err := runVerdict(t, `
errors:
  "400":
    - match: "invalid API key"
      verdict: fail_record
      class: auth
      reason: the key was rejected
`, &bodyDoer{status: 400, body: `{"error":"invalid API key"}`}, false)
	oneError(t, msgs, "fail_record")
	if code := httpx.ExitCodeFor(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (err %v)", code, err)
	}
}

// A rule after one with no match can never apply, and the binding is
// refused at load (spec/binding-schema.json, SPEC §10a).
func TestUnreachableErrorRuleIsRefused(t *testing.T) {
	_, err := Parse([]byte(errorsBinding + `
errors:
  "403":
    - verdict: fail_record
    - match: "Lead limit"
      verdict: fail_run
`))
	if err == nil || !strings.Contains(err.Error(), "can never apply") {
		t.Errorf("err = %v, want the unreachable rule refused", err)
	}
	if _, err := Parse([]byte(errorsBinding + `
errors:
  "403":
    - match: "Lead limit"
      verdict: fail_run
    - verdict: fail_record
  "404": { verdict: skip }
`)); err != nil {
		t.Errorf("a list ending in a matchless rule, beside the object form, is refused: %v", err)
	}
	if _, err := Parse([]byte(errorsBinding + `
errors:
  "403": { verdict: fail_run, class: quota }
`)); err == nil {
		t.Error("an unknown class loaded")
	}
}
