package instantly

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/adapters/adaptertest"
	"github.com/gtme-run/gtme/internal/httpx"
	"github.com/gtme-run/gtme/internal/protocol"
)

// ADR-066: every lead field the create-lead API documents maps into the
// lead body, in snake_case or in the camelCase the sequence tags use.
func TestLeadFieldsTakeEitherSpelling(t *testing.T) {
	stub := &adaptertest.Stub{Routes: routes(t)}
	_, err := adaptertest.Run(t, &Adapter{HTTP: stub}, adaptertest.Input{
		Config: map[string]any{
			"campaign": campaignID, "base_url": "https://instantly.test",
			"variables": map[string]any{
				"firstName":   "first_name",
				"lastName":    "last_name",
				"companyName": "company_name",
				"jobTitle":    "title",
				"website":     "company_domain",
				"phone":       "phone",
				"ps_line":     "ps_line",
			},
		},
		Env: map[string]string{"INSTANTLY_API_KEY": "secret"},
		Records: lead("jane.doe@acme.com", map[string]any{
			"email": "jane.doe@acme.com", "first_name": "Jane", "last_name": "Doe",
			"company_name": "Acme Inc", "title": "VP Marketing", "company_domain": "acme.com",
			"phone": "+1 555 0100", "ps_line": "PS: the CAC math checks out.",
		}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body leadRequest
	if err := json.Unmarshal([]byte(stub.Calls[0].Body), &body); err != nil {
		t.Fatalf("lead body: %v\n%s", err, stub.Calls[0].Body)
	}
	if body.FirstName != "Jane" || body.LastName != "Doe" || body.CompanyName != "Acme Inc" ||
		body.JobTitle != "VP Marketing" || body.Website != "acme.com" || body.Phone != "+1 555 0100" {
		t.Errorf("lead body = %+v, want every lead field filled", body)
	}
	if len(body.CustomVariables) != 1 || body.CustomVariables["ps_line"] == "" {
		t.Errorf("custom variables = %v, want only ps_line — a lead field is never a custom variable", body.CustomVariables)
	}
	for _, raw := range []string{`"job_title"`, `"website"`, `"phone"`} {
		if !strings.Contains(stub.Calls[0].Body, raw) {
			t.Errorf("request body has no %s key: %s", raw, stub.Calls[0].Body)
		}
	}
}

func TestSnakeCaseJobTitleReachesTheLeadBody(t *testing.T) {
	stub := &adaptertest.Stub{Routes: routes(t)}
	_, err := adaptertest.Run(t, &Adapter{HTTP: stub}, adaptertest.Input{
		Config: map[string]any{"campaign": campaignID, "base_url": "https://instantly.test",
			"variables": map[string]any{"job_title": "title"}},
		Env:     map[string]string{"INSTANTLY_API_KEY": "secret"},
		Records: lead("jane.doe@acme.com", map[string]any{"email": "jane.doe@acme.com", "title": "VP Marketing"}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var body leadRequest
	_ = json.Unmarshal([]byte(stub.Calls[0].Body), &body)
	if body.JobTitle != "VP Marketing" || len(body.CustomVariables) != 0 {
		t.Errorf("lead body = %+v", body)
	}
}

// ADR-066 (3): two targets naming one lead field refuse the session with
// exit 2 before any request.
func TestOneLeadFieldOneTarget(t *testing.T) {
	stub := &adaptertest.Stub{Routes: routes(t)}
	_, err := adaptertest.Run(t, &Adapter{HTTP: stub}, adaptertest.Input{
		Config: map[string]any{"campaign": campaignID, "base_url": "https://instantly.test",
			"variables": map[string]any{"first_name": "first_name", "firstName": "full_name"}},
		Env:     map[string]string{"INSTANTLY_API_KEY": "secret"},
		Records: lead("jane.doe@acme.com", map[string]any{"email": "jane.doe@acme.com", "first_name": "Jane", "full_name": "Jane Doe"}),
	})
	if err == nil {
		t.Fatal("want the session refused")
	}
	if code := httpx.ExitCodeFor(err); code != 2 {
		t.Errorf("exit = %d, want 2 (contract error): %v", code, err)
	}
	if !strings.Contains(err.Error(), "first_name") || !strings.Contains(err.Error(), "firstName") {
		t.Errorf("error = %v, want it to name both targets", err)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("calls = %+v, want none", stub.Calls)
	}
}

// ADR-066 (4): a lead-field tag the sequence uses must be filled.
func TestPreflightChecksLeadFieldTags(t *testing.T) {
	campaign := func(subject, body string) string {
		return `{"id":"` + campaignID + `","name":"Q3","status":1,"sequences":[{"steps":[` +
			`{"type":"email","variants":[{"subject":` + quote(subject) + `,"body":` + quote(body) + `}]}]}]}`
	}
	cases := []struct {
		name    string
		body    string
		vars    map[string]any
		status  string
		reason  string
		without string
	}{
		{"unfilled tag", campaign("Hi {{firstName}}", "{{ps_line}}"), map[string]any{"ps_line": "ps_line"},
			protocol.PreflightBlocked, "{{firstName}}", ""},
		{"filled by snake_case", campaign("Hi {{firstName}}", "{{ps_line}}"), map[string]any{"first_name": "first_name", "ps_line": "ps_line"},
			protocol.PreflightOK, "", ""},
		{"filled by camelCase", campaign("Hi {{firstName}} at {{companyName}}", "{{ps_line}}"),
			map[string]any{"firstName": "first_name", "company_name": "company_name", "ps_line": "ps_line"},
			protocol.PreflightOK, "", ""},
		{"unfilled website in body", campaign("hello", "See {{website}}"), map[string]any{},
			protocol.PreflightBlocked, "{{website}}", ""},
		{"jobTitle is not checked", campaign("Hi", "As {{jobTitle}}, {{ps_line}}"), map[string]any{"ps_line": "ps_line"},
			protocol.PreflightOK, "", ""},
		{"custom variable still checked", campaign("Hi {{firstName}}", "body"), map[string]any{"first_name": "first_name", "opener": "x"},
			protocol.PreflightBlocked, "no sequence step references {{opener}}", "{{firstName}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := routes(t)
			r["GET /api/v2/campaigns/"+campaignID] = adaptertest.Response{Body: tc.body}
			got := runPreflight(t, r, tc.vars)
			if len(got) != 1 || got[0].Status != tc.status || !strings.Contains(got[0].Reason, tc.reason) {
				t.Fatalf("preflight = %+v, want %s naming %q", got, tc.status, tc.reason)
			}
			if tc.without != "" && strings.Contains(got[0].Reason, tc.without) {
				t.Errorf("reason %q should not name %s", got[0].Reason, tc.without)
			}
		})
	}
}

// Attest compares the new lead fields like the old (ADR-066 (5)).
func TestAttestComparesTheNewLeadFields(t *testing.T) {
	sent := leadRequest{Email: "jane.doe@acme.com", Website: "acme.com", Phone: "+1 555 0100", JobTitle: "VP"}
	stored := storedLead{Fields: map[string]any{"email": "jane.doe@acme.com", "website": "acme.com", "phone": "+1 555 0100", "job_title": "VP"}}
	if st, why := compareLead(sent, stored); st != protocol.AttestConfirmed {
		t.Errorf("confirmed case = %s (%s)", st, why)
	}
	stored.Fields["website"] = "other.com"
	if st, why := compareLead(sent, stored); st != protocol.AttestContradicted || !strings.Contains(why, "website") {
		t.Errorf("contradicted case = %s (%s)", st, why)
	}
	delete(stored.Fields, "website")
	if st, why := compareLead(sent, stored); st != protocol.AttestInconclusive || !strings.Contains(why, "website") {
		t.Errorf("unreadable case = %s (%s)", st, why)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
