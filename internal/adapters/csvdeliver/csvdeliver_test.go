package csvdeliver

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtme-run/gtme/internal/protocol"
)

// Issue #164: a list or object variable lands in the CSV as JSON, the form
// dry-run and simulate print, not Go's %v form; strings are written as-is.
func TestAppendRowWritesNonScalarsAsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	cfg, err := parseConfig(map[string]any{
		"path": path,
		"variables": map[string]any{
			"tools":   "tech_stack",
			"meta":    "extra",
			"name":    "full_name",
			"count":   "headcount",
			"big":     "revenue",
			"active":  "is_active",
			"missing": "not_there",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureHeader(cfg); err != nil {
		t.Fatal(err)
	}
	// Fields as they arrive off the wire: decoded JSON.
	fields := map[string]any{
		"tech_stack": []any{"HubSpot (Marketing automation)", "Salesforce (CRM)", "R&D <ops>"},
		"extra":      map[string]any{"b": 2.0, "a": "x"},
		"full_name":  "Ada Lovelace, \"Countess\"",
		"headcount":  42.0,
		"revenue":    2500000.0,
		"is_active":  true,
	}
	if err := appendRow(cfg, protocol.Key{IdentityKey: "email:ada@example.com"}, fields); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want header + 1", len(rows))
	}
	got := map[string]string{}
	for i, col := range rows[0] {
		got[col] = rows[1][i]
	}
	want := map[string]string{
		"identity_key": "email:ada@example.com",
		"tools":        `["HubSpot (Marketing automation)","Salesforce (CRM)","R&D <ops>"]`,
		"meta":         `{"a":"x","b":2}`,
		"name":         `Ada Lovelace, "Countess"`,
		"count":        "42",
		"big":          "2500000",
		"active":       "true",
		"missing":      "",
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("column %s = %q, want %q", col, got[col], w)
		}
	}
}
