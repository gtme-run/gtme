package e2e

import (
	"encoding/json"
	"testing"
)

// TestShowIdentityPrintsProjection is the M3 acceptance test for `gtme show`
// (SPEC §8, ADR-006): the current-value projection for one identity, with
// --fields narrowing it and --provenance adding source/confidence/run.
func TestShowIdentityPrintsProjection(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", csvToMockYAML)
	h.mustRun("run", "pipeline.yaml")

	res := h.mustRun("show", "jane.doe@acme.com")
	var out map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		t.Fatalf("show output must be JSON: %v\n%s", err, res.stdout)
	}
	if out["entity_type"] != "person" || out["identity_key"] != "jane.doe@acme.com" {
		t.Fatalf("show = %+v", out)
	}
	fields, ok := out["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields was not an object: %+v", out["fields"])
	}
	if fields["full_name"] != "Jane Doe" || fields["mock.score"] == nil {
		t.Errorf("fields = %+v, want full_name (sourced) and mock.score (enriched)", fields)
	}

	// --fields narrows the projection.
	narrow := h.mustRun("show", "jane.doe@acme.com", "--fields", "mock.score")
	var narrowed map[string]any
	if err := json.Unmarshal([]byte(narrow.stdout), &narrowed); err != nil {
		t.Fatalf("show --fields output must be JSON: %v", err)
	}
	nf := narrowed["fields"].(map[string]any)
	if len(nf) != 1 || nf["mock.score"] == nil {
		t.Errorf("narrowed fields = %+v, want exactly {mock.score}", nf)
	}

	// --provenance turns each value into {value, source, confidence, run_id, created_at}.
	prov := h.mustRun("show", "jane.doe@acme.com", "--fields", "mock.score", "--provenance")
	var provenanced map[string]any
	if err := json.Unmarshal([]byte(prov.stdout), &provenanced); err != nil {
		t.Fatalf("show --provenance output must be JSON: %v", err)
	}
	pf := provenanced["fields"].(map[string]any)["mock.score"].(map[string]any)
	for _, key := range []string{"value", "source", "confidence", "run_id", "created_at"} {
		if _, ok := pf[key]; !ok {
			t.Errorf("--provenance field missing %q: %+v", key, pf)
		}
	}
	if src, _ := pf["source"].(string); src == "" {
		t.Errorf("provenance source was empty: %+v", pf)
	}
}

// TestShowUnknownIdentity fails helpfully rather than printing nothing.
func TestShowUnknownIdentity(t *testing.T) {
	h := newHarness(t)
	res := h.run("show", "nobody@nowhere.example")
	if res.code != 2 {
		t.Errorf("exit = %d, want 2", res.code)
	}
	contains(t, res.stderr, "no identity known by key", "stderr")
}

// TestShowRunListsRecords is --run's half of the acceptance test: every
// record the run touched, NDJSON, with --limit capping the count.
func TestShowRunListsRecords(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", csvToMockYAML)
	h.mustRun("run", "pipeline.yaml")

	res := h.mustRun("show", "--run", "last")
	lines := nonEmptyLines(res.stdout)
	if len(lines) != 3 {
		t.Fatalf("--run last printed %d records, want 3:\n%s", len(lines), res.stdout)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		t.Fatalf("each --run line must be JSON: %v", err)
	}
	for _, key := range []string{"entity_type", "identity_key", "state", "fields"} {
		if _, ok := row[key]; !ok {
			t.Errorf("--run record missing %q: %+v", key, row)
		}
	}

	limited := h.mustRun("show", "--run", "last", "--limit", "1")
	if got := len(nonEmptyLines(limited.stdout)); got != 1 {
		t.Errorf("--limit 1 printed %d records", got)
	}
}

// TestShowNeverWrites keeps `gtme show` read-only (SPEC §8, ADR-006): running
// it must not change the ledger's row counts.
func TestShowNeverWrites(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.write("pipeline.yaml", csvToMockYAML)
	h.mustRun("run", "pipeline.yaml")

	before := h.queryInt(`SELECT count(*) FROM field_values`)
	h.mustRun("show", "jane.doe@acme.com", "--provenance")
	h.mustRun("show", "--run", "last")
	after := h.queryInt(`SELECT count(*) FROM field_values`)
	if before != after {
		t.Errorf("field_values went from %d to %d rows — gtme show wrote to the ledger", before, after)
	}
}

// TestShowNamesTheIdentityKeyTier covers ADR-058's identity_key_tier: the
// type file's identity field the key came from, or name_hash for the nh:
// fallback. The hash tier is recognised by its prefix, never by re-running
// a rule — the handle rule would otherwise claim any string.
func TestShowNamesTheIdentityKeyTier(t *testing.T) {
	h := newHarness(t)
	h.write("tiers.csv", "Full Name,Email,Company Website\nJane Doe,jane.doe@acme.com,acme.com\nMia Chen,,initech.dev\n")
	h.write("tiers.yaml", `name: tiers
version: 1
source:
  use: csv/source
  with:
    path: tiers.csv
    columns: { full_name: Full Name, email: Email, company_domain: Company Website }
steps: []
`)
	h.mustRun("run", "tiers.yaml")

	tier := func(key string) string {
		res := h.mustRun("show", key)
		var out map[string]any
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
			t.Fatalf("show %s: not JSON: %v\n%s", key, err, res.stdout)
		}
		s, _ := out["identity_key_tier"].(string)
		return s
	}
	if got := tier("jane.doe@acme.com"); got != "email" {
		t.Errorf("jane's tier = %q, want email", got)
	}
	if got := tier("acme.com"); got != "company_domain" {
		t.Errorf("acme.com's tier = %q, want company_domain", got)
	}
	keys := h.queryStrings(`SELECT identity_key FROM identities WHERE identity_key LIKE 'nh:%' AND entity_type = 'person'`)
	if len(keys) != 1 {
		t.Fatalf("name-hashed people = %v, want exactly Mia", keys)
	}
	if got := tier(keys[0]); got != "name_hash" {
		t.Errorf("Mia's tier = %q, want name_hash", got)
	}
}
