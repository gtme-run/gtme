package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// The "(* required)" legend explains a star; it prints only when a star is
// there to explain (issue #97).
func TestSchemaSummaryLegend(t *testing.T) {
	none := schemaSummary(json.RawMessage(`{"properties":{"email":{},"city":{}}}`))
	if strings.Contains(none, "*") {
		t.Errorf("no required fields, but got %q", none)
	}
	some := schemaSummary(json.RawMessage(`{"required":["email"],"properties":{"email":{},"city":{}}}`))
	if !strings.Contains(some, "email*") || !strings.HasSuffix(some, "(* required)") {
		t.Errorf("one required field, got %q", some)
	}
}
