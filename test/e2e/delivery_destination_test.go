package e2e

// Delivery dedupe is per destination (ADR-044): a record sent to one
// destination is still a fresh decision for another, and the same
// destination under a new display name is the same destination.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestHTTPDeliverScopesToTheURL: two pipelines deliver the same records
// through http/deliver to two different URLs. The second URL has never
// seen them, so it must receive them.
func TestHTTPDeliverScopesToTheURL(t *testing.T) {
	var a, b atomic.Int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Add(1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.Add(1)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srvB.Close()

	h := newHarness(t)
	h.write("contacts.csv", campaignZeroCSV)
	pipeline := func(name, url string) string {
		y := strings.Replace(outFloorYAML, "name: out-floor", "name: "+name, 1)
		y = strings.Replace(y, "%s\n", "http/deliver\n", 1)
		return strings.Replace(y, "%s", `      url: "`+url+`/hook"`, 1)
	}
	h.write("a.yaml", pipeline("to-a", srvA.URL))
	h.write("b.yaml", pipeline("to-b", srvB.URL))

	h.mustRun("run", "a.yaml")
	if got := a.Load(); got != 2 {
		t.Fatalf("webhook A received %d, want 2", got)
	}
	res := h.mustRun("run", "b.yaml")
	if got := b.Load(); got != 2 {
		t.Errorf("webhook B received %d, want 2 — it has never seen these records\nstderr:\n%s", got, res.stderr)
	}

	// A second run to the same URL withholds both: already delivered, which
	// is neither a cache hit nor money saved (ADR-062).
	again := h.mustRun("run", "b.yaml")
	if got := b.Load(); got != 2 {
		t.Errorf("webhook B received %d after a re-run, want 2", got)
	}
	contains(t, again.stderr, "0 cached, 0 filtered, 0 failed, 2 already delivered", "re-run to the same URL")
	if strings.Contains(again.stderr, "avoided via cache") {
		t.Errorf("an already-delivered skip reported as cost avoided:\n%s", again.stderr)
	}
}

// fakeInstantly is the slice of the Instantly v2 API the adapter touches,
// with a campaign whose display name the test can change.
type fakeInstantly struct {
	mu    sync.Mutex
	name  string
	leads int
}

const fakeCampaignID = "0198a0b1-2c3d-4e5f-8a9b-0c1d2e3f4a5b"

func (f *fakeInstantly) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/v2/campaigns":
		json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
			{"id": fakeCampaignID, "name": f.name, "status": 1}}})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v2/campaigns/"):
		json.NewEncoder(w).Encode(map[string]any{"id": fakeCampaignID, "name": f.name, "status": 1})
	case r.Method == "POST" && r.URL.Path == "/api/v2/leads":
		f.leads++
		fmt.Fprintf(w, `{"id":"lead-%d","campaign":%q}`, f.leads, fakeCampaignID)
	default:
		http.NotFound(w, r)
	}
}

// TestInstantlyRenameIsTheSameCampaign: renaming a campaign in Instantly,
// and the pipeline with it, does not make it a new destination.
func TestInstantlyRenameIsTheSameCampaign(t *testing.T) {
	fake := &fakeInstantly{name: "Q3 VP Marketing"}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	h := newHarness(t)
	h.write("contacts.csv", campaignZeroCSV)
	pipeline := func(campaign string) string {
		y := strings.Replace(outFloorYAML, "%s\n", "instantly/add-to-campaign\n", 1)
		return strings.Replace(y, "%s", `      campaign: "`+campaign+`"
      base_url: "`+srv.URL+`"`, 1)
	}
	env := []string{"INSTANTLY_API_KEY=test-key"}

	h.write("p.yaml", pipeline("Q3 VP Marketing"))
	if res := h.runWithEnv(env, "", "run", "p.yaml"); res.code != 0 {
		t.Fatalf("first run exit = %d\nstderr:\n%s", res.code, res.stderr)
	}
	if fake.leads != 2 {
		t.Fatalf("leads after the first run = %d, want 2", fake.leads)
	}

	fake.mu.Lock()
	fake.name = "Q3 VP Marketing (renamed)"
	fake.mu.Unlock()
	h.write("p.yaml", pipeline("Q3 VP Marketing (renamed)"))
	res := h.runWithEnv(env, "", "run", "p.yaml")
	if res.code != 0 {
		t.Fatalf("second run exit = %d\nstderr:\n%s", res.code, res.stderr)
	}
	if fake.leads != 2 {
		t.Errorf("leads after the rename = %d, want 2 — the same campaign got the same people again\nstderr:\n%s", fake.leads, res.stderr)
	}
}
