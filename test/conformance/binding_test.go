package conformance

// The binding conformance kit (SPEC §10a, ADR-022): every shipped binding
// parses against spec/binding-schema.json, and the engine run against the
// binding's own conformance fixtures produces the expected canonical,
// registry-valid records — fixture payloads in, canonical records out. The
// same fixtures serve `--simulate` (SPEC §8), which is why keeping them good
// matters twice.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/adapters/adaptertest"
	_ "github.com/gtme-run/gtme/internal/adapters/all"
	"github.com/gtme-run/gtme/internal/binding"
	"github.com/gtme-run/gtme/internal/protocol"
	"github.com/gtme-run/gtme/internal/registry"
)

// registryEntries is the local copy of the registry's vendor entries
// (ADR-059): the binary ships no vendor binding since M33.
const registryEntries = "../fixtures/registry"

// loadShipped loads one registry entry (from the local copy) and its
// fixtures.
func loadShipped(t *testing.T, name string) (*binding.Binding, *binding.FixtureSet) {
	t.Helper()
	b, fixtures, err := binding.LoadFS(os.DirFS(filepath.Join(registryEntries, name)))
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return b, fixtures
}

// TestShippedBindingsParse: the one binding under spec/bindings/ (the
// worked example) and every registry entry in the local copy conform to the
// schema, bridge onto a valid §6 manifest, and name only registry-valid
// fields in their extraction (§4a enforcement extended to bindings).
func TestShippedBindingsParse(t *testing.T) {
	if names := binding.Shipped(); len(names) != 1 {
		t.Errorf("spec/bindings/ = %v, want exactly the one worked example (ADR-059)", names)
	}
	entries, err := os.ReadDir(registryEntries)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	loads := map[string]func() (*binding.Binding, *binding.FixtureSet, error){}
	for _, name := range binding.Shipped() {
		dir, err := binding.ShippedFS(name)
		if err != nil {
			t.Fatal(err)
		}
		loads["spec/bindings/"+name] = func() (*binding.Binding, *binding.FixtureSet, error) { return binding.LoadFS(dir) }
	}
	for _, e := range entries {
		dir := os.DirFS(filepath.Join(registryEntries, e.Name()))
		loads["registry/"+e.Name()] = func() (*binding.Binding, *binding.FixtureSet, error) { return binding.LoadFS(dir) }
	}
	for name, load := range loads {
		b, fixtures, err := load()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if _, err := b.Manifest(); err != nil {
			t.Errorf("%s: manifest bridge: %v", name, err)
		}
		for field := range b.Extract.Fields {
			if err := reg.ValidateName(b.EntityType, field); err != nil {
				t.Errorf("%s: extract field: %v", name, err)
			}
		}
		if fixtures == nil {
			t.Errorf("%s: ships no conformance fixtures — a simulation gap by construction (SPEC §8)", name)
		}
	}
}

// checkRegistryValid asserts a record's canonical fields satisfy the registry
// (§4a layer 2, applied to engine output).
func checkRegistryValid(t *testing.T, entityType string, fields map[string]any) {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range fields {
		if err := reg.CheckValue(entityType, name, normalizeJSON(v)); err != nil {
			t.Errorf("registry: %v", err)
		}
	}
}

func normalizeJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

func TestApolloBindingConformance(t *testing.T) {
	b, fixtures := loadShipped(t, "apollo-search")
	eng := &binding.Engine{B: b, HTTP: fixtures.Doer()}

	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{"query": "vp marketing"},
		Env:    map[string]string{"APOLLO_API_KEY": "k"},
	})
	if err != nil {
		t.Fatalf("engine: %v\nlogs:\n%s", err, adaptertest.Logs(msgs))
	}
	records := adaptertest.Records(msgs)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2\nlogs:\n%s", len(records), adaptertest.Logs(msgs))
	}

	// The masked surface (ADR-043): no email, no linkedin, the vendor's
	// obfuscated last name (it keys the name-hash identity tier), and the
	// has_email pay-signal.
	jane := records[0].Fields
	wantJane := map[string]any{
		"apollo.id": "5f3b0c1a2d", "first_name": "Jane", "last_name": "D.",
		"title": "VP Marketing", "company_name": "Acme Inc",
		"apollo.has_email": true,
	}
	if !reflect.DeepEqual(normalizeJSON(jane), normalizeJSON(wantJane)) {
		t.Errorf("jane = %#v\nwant   %#v", normalizeJSON(jane), normalizeJSON(wantJane))
	}
	checkRegistryValid(t, "person", jane)

	bob := records[1].Fields
	if bob["apollo.has_email"] != false || bob["company_name"] != "Grove Labs" {
		t.Errorf("bob = %#v", bob)
	}
	if _, has := bob["email"]; has {
		t.Errorf("masked search must never emit an email: %v", bob["email"])
	}
	checkRegistryValid(t, "person", bob)

	costs := adaptertest.Costs(msgs)
	if len(costs) != 1 || costs[0].Provider != "apollo" || costs[0].Amount() != 0 {
		t.Errorf("costs = %#v", costs)
	}
}

// TestApolloEnrichConformance: the paid half of the split (ADR-043) — one
// masked record in, the revealed surface out, one per-credit cost row.
func TestApolloEnrichConformance(t *testing.T) {
	b, fixtures := loadShipped(t, "apollo-enrich")
	eng := &binding.Engine{B: b, HTTP: fixtures.Doer()}

	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{},
		Records: []protocol.Message{adaptertest.Record("verify",
			map[string]any{"apollo.id": "5f3b0c1a2d", "first_name": "Jane", "last_name": "D."})},
		Env: map[string]string{"APOLLO_API_KEY": "k"},
	})
	if err != nil {
		t.Fatalf("engine: %v\nlogs:\n%s", err, adaptertest.Logs(msgs))
	}
	records := adaptertest.Records(msgs)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1\nlogs:\n%s", len(records), adaptertest.Logs(msgs))
	}
	got := records[0].Fields
	for field, want := range map[string]any{
		"email": "jane.doe@acme.com", "email_status": "verified",
		"last_name": "Doe", "full_name": "Jane Doe",
		"linkedin_url":   "https://www.linkedin.com/in/jane-doe",
		"company_domain": "acme.com", "company_employees": float64(120),
	} {
		if normalizeJSON(got[field]) != normalizeJSON(want) {
			t.Errorf("%s = %#v, want %#v", field, got[field], want)
		}
	}
	checkRegistryValid(t, "person", got)

	costs := adaptertest.Costs(msgs)
	if len(costs) != 1 || costs[0].Provider != "apollo" || costs[0].Amount() != 0.01 {
		t.Errorf("costs = %#v", costs)
	}
}

// TestHarvestBindingConformance: harvest/profile@2 (ADR-059) — one call,
// role_history through each:, the cost from config at its default.
func TestHarvestBindingConformance(t *testing.T) {
	b, fixtures := loadShipped(t, "harvest-profile")
	eng := &binding.Engine{B: b, HTTP: fixtures.Doer()}

	// Looked up by internal-form URL: the resolved public linkedin_url must be
	// emitted (ADR-020's recovery path, skip_if_input lets it through).
	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{},
		Records: []protocol.Message{adaptertest.Record("in/acwaa123",
			map[string]any{"linkedin_internal_url": "https://www.linkedin.com/in/ACwAA123"})},
		Env: map[string]string{"HARVEST_API_KEY": "k"},
	})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	records := adaptertest.Records(msgs)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1\nlogs:\n%s", len(records), adaptertest.Logs(msgs))
	}
	got := records[0].Fields
	want := map[string]any{
		"linkedin_url":    "https://www.linkedin.com/in/jane-doe",
		"headline":        "VP Marketing at Acme — demand gen, lifecycle, and the occasional spreadsheet",
		"about":           "I run marketing at Acme.",
		"location":        "Austin, Texas, United States",
		"current_role":    "VP Marketing",
		"current_company": "Acme Inc",
		"follower_count":  float64(4210),
		"role_history": []any{
			"VP Marketing at Acme Inc (2022–present)",
			"Director of Demand Gen at Initech (2019–2022)",
		},
	}
	if !reflect.DeepEqual(normalizeJSON(got), normalizeJSON(want)) {
		t.Errorf("fields = %#v\nwant    %#v", normalizeJSON(got), normalizeJSON(want))
	}
	if _, has := got["open_to_work"]; has {
		t.Error("open_to_work false should be absent (sentinel)")
	}
	checkRegistryValid(t, "person", got)

	costs := adaptertest.Costs(msgs)
	if len(costs) != 1 || costs[0].Amount() != 0.012 || costs[0].Provider != "harvest" {
		t.Errorf("costs = %#v", costs)
	}

	// Looked up by public URL: the already-known linkedin_url is not re-emitted
	// (skip_if_input).
	eng2 := &binding.Engine{B: b, HTTP: fixtures.Doer()}
	msgs, err = adaptertest.Run(t, eng2, adaptertest.Input{
		Config: map[string]any{},
		Records: []protocol.Message{adaptertest.Record("in/jane-doe",
			map[string]any{"linkedin_url": "https://www.linkedin.com/in/jane-doe"})},
		Env: map[string]string{"HARVEST_API_KEY": "k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	records = adaptertest.Records(msgs)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if _, has := records[0].Fields["linkedin_url"]; has {
		t.Error("linkedin_url re-emitted despite skip_if_input")
	}
}

func TestAttioBindingConformance(t *testing.T) {
	b, _ := loadShipped(t, "attio-assert")
	stub := &adaptertest.Stub{Routes: map[string]adaptertest.Response{
		"PUT /v2/objects/people/records": {Body: `{"data":{"id":{"record_id":"rec_1"}}}`},
	}}
	eng := &binding.Engine{B: b, HTTP: stub}

	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{
			"variables": map[string]any{"name": "full_name"},
		},
		Records: []protocol.Message{adaptertest.Record("jane.doe@acme.com", map[string]any{
			"email": "jane.doe@acme.com", "full_name": "Jane Doe",
		})},
		Env: map[string]string{"ATTIO_API_KEY": "k"},
	})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	if records := adaptertest.Records(msgs); len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	call := stub.Calls[0]
	if got := call.URL; !contains1(got, "matching_attribute=email_addresses") {
		t.Errorf("url = %q, want matching_attribute query", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(call.Body), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"data": map[string]any{"values": map[string]any{
		"email_addresses": []any{"jane.doe@acme.com"},
		"name":            "Jane Doe",
	}}}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("request body = %#v\nwant %#v", body, want)
	}
}

// TestSimulateGap: a binding without fixtures, asked to simulate, surfaces the
// gap instead of touching the network or silently passing (SPEC §8).
func TestSimulateGap(t *testing.T) {
	b, _ := loadShipped(t, "harvest-profile")
	eng := &binding.Engine{B: b, Fixtures: nil, HTTP: nil} // no fixtures, no network

	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{},
		Records: []protocol.Message{adaptertest.Record("in/jane-doe",
			map[string]any{"linkedin_url": "https://www.linkedin.com/in/jane-doe"})},
		Env: map[string]string{binding.SimulateEnv: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if records := adaptertest.Records(msgs); len(records) != 0 {
		t.Errorf("a fixtureless simulation produced records: %#v", records)
	}
	if logs := adaptertest.Logs(msgs); !contains1(logs, "simulation gap") {
		t.Errorf("logs = %q, want a simulation-gap warning", logs)
	}
}

// TestSimulateServesFixtures: with fixtures present and GTME_SIMULATE set, the
// engine answers from fixtures without any HTTP seam at all.
func TestSimulateServesFixtures(t *testing.T) {
	b, fixtures := loadShipped(t, "harvest-profile")
	eng := &binding.Engine{B: b, Fixtures: fixtures, HTTP: nil}

	msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
		Config: map[string]any{},
		Records: []protocol.Message{adaptertest.Record("in/acwaa123",
			map[string]any{"linkedin_internal_url": "https://www.linkedin.com/in/ACwAA123"})},
		Env: map[string]string{binding.SimulateEnv: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	records := adaptertest.Records(msgs)
	if len(records) != 1 || records[0].Fields["headline"] == "" {
		t.Fatalf("simulated records = %#v", records)
	}
}

// TestHarvestRecentPostsConformance: harvest/recent-posts keeps at most
// posts_limit non-empty posts, content before text (ADR-059).
func TestHarvestRecentPostsConformance(t *testing.T) {
	b, fixtures := loadShipped(t, "harvest-recent-posts")
	for _, tc := range []struct {
		config map[string]any
		want   int
	}{{map[string]any{}, 3}, {map[string]any{"posts_limit": float64(1)}, 1}} {
		eng := &binding.Engine{B: b, HTTP: fixtures.Doer()}
		msgs, err := adaptertest.Run(t, eng, adaptertest.Input{
			Config: tc.config,
			Records: []protocol.Message{adaptertest.Record("in/jane-doe",
				map[string]any{"linkedin_url": "https://www.linkedin.com/in/jane-doe"})},
			Env: map[string]string{"HARVEST_API_KEY": "k"},
		})
		if err != nil {
			t.Fatal(err)
		}
		records := adaptertest.Records(msgs)
		if len(records) != 1 {
			t.Fatalf("records = %d, want 1", len(records))
		}
		posts, _ := records[0].Fields["recent_posts"].([]any)
		if len(posts) != tc.want || posts[0] != "We cut our CAC in half by killing three channels. Here is what we kept." {
			t.Errorf("config %v: recent_posts = %#v, want %d", tc.config, posts, tc.want)
		}
		checkRegistryValid(t, "person", records[0].Fields)
	}
}

func contains1(haystack, needle string) bool { return strings.Contains(haystack, needle) }
