package binding

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gtme-run/gtme/internal/adapters/adaptertest"
	"github.com/gtme-run/gtme/internal/httpx"
	"github.com/gtme-run/gtme/internal/protocol"
)

// errorsBinding is a per-record enrich binding with a free tail for the
// errors: and retry: blocks under test.
const errorsBinding = `
id: vendor/lookup
version: 1
role: enrich
entity_type: person
needs:
  type: object
  required: [email]
  properties:
    email: { type: string }
provides:
  type: object
  additionalProperties: false
  properties:
    title: { type: string }
config_schema:
  type: object
  additionalProperties: false
  properties:
    base_url:
      type: string
      default: "https://api.vendor.test"
request:
  method: GET
  url: "{{config.base_url}}/lookup"
  query:
    email: "{{record.email}}"
extract:
  records: person
  fields:
    title: title
`

// seqDoer answers each request with the next status in line (the last one
// repeats), so a test can script "fail, then succeed".
type seqDoer struct {
	statuses []int
	calls    int
}

func (d *seqDoer) Do(req *http.Request) (*http.Response, error) {
	i := d.calls
	if i >= len(d.statuses) {
		i = len(d.statuses) - 1
	}
	d.calls++
	status := d.statuses[i]
	body := `{"error":"nope"}`
	if status < 400 {
		body = `{"person":{"title":"VP Marketing"}}`
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func runErrors(t *testing.T, tail string, doer httpx.Doer) ([]protocol.Message, error) {
	t.Helper()
	b, err := Parse([]byte(errorsBinding + tail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return adaptertest.Run(t, &Engine{B: b, HTTP: doer}, adaptertest.Input{
		Records: []protocol.Message{adaptertest.Record("jane@acme.com", map[string]any{"email": "jane@acme.com"})},
	})
}

func noSleep(t *testing.T) {
	t.Helper()
	old := httpx.RetryBase
	httpx.RetryBase = 0
	t.Cleanup(func() { httpx.RetryBase = old })
}

// TestFailRunVerdictStopsWithTheRulesReason (#162): a fail_run rule stops the
// run, says the binding's reason, and keeps the status's exit-code class.
func TestFailRunVerdictStopsWithTheRulesReason(t *testing.T) {
	noSleep(t)
	doer := &seqDoer{statuses: []int{401}}
	msgs, err := runErrors(t, `
errors:
  "401": { verdict: fail_run, reason: invalid API key }
`, doer)
	if err == nil {
		t.Fatalf("run succeeded; want fail_run to stop it\nlogs:\n%s", adaptertest.Logs(msgs))
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("error %q does not carry the rule's reason", err)
	}
	if code := httpx.ExitCodeFor(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (auth)", code)
	}
}

// TestRetryVerdictRetriesAStatusTheEngineWouldNot (#162): a 409 is not
// retryable by default; a retry rule makes it so, and a later success lands.
func TestRetryVerdictRetriesAStatusTheEngineWouldNot(t *testing.T) {
	noSleep(t)
	doer := &seqDoer{statuses: []int{409, 409, 200}}
	msgs, err := runErrors(t, `
errors:
  "409": { verdict: retry, reason: index still building }
retry:
  max_attempts: 4
`, doer)
	if err != nil {
		t.Fatalf("engine: %v\nlogs:\n%s", err, adaptertest.Logs(msgs))
	}
	if doer.calls != 3 {
		t.Errorf("calls = %d, want 3 (two retried 409s, then the 200)", doer.calls)
	}
	if recs := adaptertest.Records(msgs); len(recs) != 1 || recs[0].Fields["title"] != "VP Marketing" {
		t.Errorf("records = %+v, want the one the 200 answered", recs)
	}
}

// TestRetryVerdictExhaustedFailsWithTheReason (#162): retries stop at
// max_attempts, and the run then fails naming the rule's reason.
func TestRetryVerdictExhaustedFailsWithTheReason(t *testing.T) {
	noSleep(t)
	doer := &seqDoer{statuses: []int{409}}
	_, err := runErrors(t, `
errors:
  "4xx": { verdict: retry, reason: index still building }
retry:
  max_attempts: 2
`, doer)
	if err == nil {
		t.Fatal("run succeeded; want it to fail once retries are exhausted")
	}
	if doer.calls != 2 {
		t.Errorf("calls = %d, want max_attempts = 2", doer.calls)
	}
	if !strings.Contains(err.Error(), "index still building") {
		t.Errorf("error %q does not carry the rule's reason", err)
	}
}

// recordSleeps swaps httpx's sleep for one that records each wait and
// returns at once.
func recordSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var waits []time.Duration
	old := httpx.Sleep
	httpx.Sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	t.Cleanup(func() { httpx.Sleep = old })
	return &waits
}

// TestRetryBackoffSecondsIsTheBaseDelay (#163): backoff_seconds is the
// first wait, doubling per retry, in place of httpx's own 1s base.
func TestRetryBackoffSecondsIsTheBaseDelay(t *testing.T) {
	waits := recordSleeps(t)
	doer := &seqDoer{statuses: []int{503}}
	_, err := runErrors(t, `
retry:
  max_attempts: 3
  backoff_seconds: 2.5
`, doer)
	if err == nil {
		t.Fatal("run succeeded against a steady 503")
	}
	want := []time.Duration{2500 * time.Millisecond, 5 * time.Second}
	if !reflect.DeepEqual(*waits, want) {
		t.Errorf("waits = %v, want %v", *waits, want)
	}
}

// TestRetryBackoffSecondsZeroMeansNoWait (#163): an explicit 0 is a
// declared value (the schema's minimum), not "unset".
func TestRetryBackoffSecondsZeroMeansNoWait(t *testing.T) {
	waits := recordSleeps(t)
	doer := &seqDoer{statuses: []int{503}}
	_, _ = runErrors(t, `
retry:
  max_attempts: 3
  backoff_seconds: 0
`, doer)
	want := []time.Duration{0, 0}
	if !reflect.DeepEqual(*waits, want) {
		t.Errorf("waits = %v, want %v", *waits, want)
	}
}
