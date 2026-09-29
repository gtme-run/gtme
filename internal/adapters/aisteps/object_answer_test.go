package aisteps

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/protocol"
)

// judgeProvides is the shape of issue #186's judge: an ai/review grading a
// company with a grade and a reason.
var judgeProvides = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"qualify.grade":  map[string]any{"type": "string", "enum": []any{"A", "B", "C"}},
		"qualify.reason": map[string]any{"type": "string"},
	},
	"required": []any{"qualify.grade", "qualify.reason"},
}

// TestReviewAcceptsUnambiguousObjectAnswers is issue #186: at batch_size 1 the
// judge sometimes answers with its one element as a bare object, or with the
// array wrapped under a single key. Both are unambiguous, so they are
// accepted on the first call instead of paying for a retry.
func TestReviewAcceptsUnambiguousObjectAnswers(t *testing.T) {
	cases := []struct{ name, answer string }{
		{"bare object", `{"identity_key":"a@x.com","qualify.grade":"B","qualify.reason":"Mid-size team, no stated need."}`},
		{"wrapped array", "```json\n" + `{"results":[{"identity_key":"a@x.com","qualify.grade":"B","qualify.reason":"Mid-size team, no stated need."}]}` + "\n```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A second answer that would also fail proves no retry was needed.
			engine := &scriptEngine{answers: []string{tc.answer, "not json"}}
			a := &Adapter{Mode: modeReview, Engine: engine}
			msgs, err := drive(t, a, map[string]any{"template": "Grade the fit.", "of": "title", "provides": judgeProvides}, "a@x.com")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if engine.callCount != 1 {
				t.Errorf("engine calls = %d, want 1 (no retry)", engine.callCount)
			}
			var got int
			for _, m := range msgs {
				switch m.Type {
				case protocol.TypeRecord:
					got++
					if m.Fields["qualify.grade"] != "B" {
						t.Errorf("fields = %v", m.Fields)
					}
				case protocol.TypeLog:
					if strings.Contains(fmt.Sprint(m), "retrying") {
						t.Errorf("unexpected retry log: %v", m)
					}
				}
			}
			if got != 1 {
				t.Errorf("records = %d, want 1", got)
			}
		})
	}
}

// TestParseRejectsAmbiguousObjects: a bare object stands for the batch only
// when the batch has one record, and a wrapper counts only when it has a
// single key holding an array. Anything else still fails validation and is
// retried as before.
func TestParseRejectsAmbiguousObjects(t *testing.T) {
	two := []record{
		{key: protocol.Key{EntityType: "person", IdentityKey: "a@x.com"}},
		{key: protocol.Key{EntityType: "person", IdentityKey: "b@x.com"}},
	}
	one := two[:1]
	a := &Adapter{Mode: modeFilter}
	cases := []struct {
		name    string
		records []record
		text    string
	}{
		{"bare object for two records", two, `{"identity_key":"a@x.com","pass":true}`},
		{"wrapper with two keys", one, `{"results":[{"identity_key":"a@x.com","pass":true}],"note":"x"}`},
		{"wrapper holding an object", one, `{"result":{"identity_key":"a@x.com","pass":true}}`},
		{"object with no identity_key", one, `{"pass":true}`},
		{"prose", one, `Verdict: pass`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.parse(tc.text, shape{}, tc.records)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "not a JSON array") {
				t.Errorf("error = %q, want it to say the response is not a JSON array", err)
			}
		})
	}
}

// TestSystemPromptRequiresTheArrayForOneRecord: the contract names the
// one-record case, the one issue #186 saw answered as a bare object.
func TestSystemPromptRequiresTheArrayForOneRecord(t *testing.T) {
	a := &Adapter{Mode: modeReview}
	cfg, err := parseConfig(map[string]any{"template": "x", "of": "title", "provides": judgeProvides})
	if err != nil {
		t.Fatal(err)
	}
	sh, err := a.shapeFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sys := a.systemPrompt(sh, cfg); !strings.Contains(sys, "even when there is only one record") {
		t.Errorf("system prompt does not require the array for one record:\n%s", sys)
	}
}
