// Package instantly is the instantly/add-to-campaign deliver adapter: it adds a
// person to an Instantly campaign, with the composed lines as custom variables
// (SPEC §10.6).
//
// This adapter puts people into a live sending sequence. Everything about it is
// therefore conservative: the campaign must already exist and is named by its
// id, never its display name (a renamed campaign is the same destination,
// SPEC §10.6, ADR-062), and delivery is gated by the runner's idempotency
// table so a re-run cannot add the same person twice.
package instantly

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/internal/httpx"
	"github.com/gtme-run/gtme/internal/protocol"
)

// ID is the adapter id.
const ID = "instantly/add-to-campaign"

// leadFields maps every variables: target name that fills a lead-body
// field to that field (SPEC §10 item 6, ADR-066): the seven fields
// Instantly's create-lead API documents, in snake_case or in the camelCase
// its sequence tags use. Any other target is a custom variable.
var leadFields = map[string]string{
	"first_name": "first_name", "firstName": "first_name",
	"last_name": "last_name", "lastName": "last_name",
	"company_name": "company_name", "companyName": "company_name",
	"job_title": "job_title", "jobTitle": "job_title",
	"personalization": "personalization",
	"website":         "website",
	"phone":           "phone",
}

// leadFieldTags are the sequence tags preflight checks are filled (ADR-066
// (4)): the tag Instantly's editor inserts for a lead field. {{jobTitle}}
// is left out — the tag is not confirmed, and preflight blocks only on a
// readable fact.
var leadFieldTags = []struct{ tag, field string }{
	{"{{firstName}}", "first_name"},
	{"{{lastName}}", "last_name"},
	{"{{companyName}}", "company_name"},
	{"{{personalization}}", "personalization"},
	{"{{website}}", "website"},
	{"{{phone}}", "phone"},
}

// leadField returns the lead-body field a target fills, or "" for a custom
// variable.
func leadField(target string) string { return leadFields[target] }

// contractError is a variables: mapping the adapter cannot honour: exit 2,
// the contract class (SPEC §8), before any request.
type contractError struct{ msg string }

func (e *contractError) Error() string { return e.msg }
func (e *contractError) ExitCode() int { return 2 }

//go:embed manifest.json
var manifestJSON []byte

// Manifest is the adapter's manifest.json (SPEC §6). The adapter is not
// built into gtme (ADR-063): cmd/gtme-instantly runs it as a process
// adapter, and the release ships this manifest beside that executable.
func Manifest() []byte { return manifestJSON }

// Adapter delivers leads to Instantly. HTTP is the seam tests stub.
type Adapter struct {
	HTTP httpx.Doer
}

type config struct {
	Campaign         string
	SkipIfInCampaign bool
	// Variables is the egress mapping (ADR-018): target merge-field name →
	// ledger field. Injected by the runner from the step-level variables: key.
	// No merge field is hard-coded (SPEC §10.6).
	Variables map[string]string
	BaseURL   string
}

func parseConfig(raw map[string]any) (config, error) {
	c := config{SkipIfInCampaign: true, BaseURL: DefaultBaseURL}
	c.Campaign, _ = raw["campaign"].(string)
	c.Campaign = strings.TrimSpace(c.Campaign)
	if c.Campaign == "" {
		return c, fmt.Errorf("instantly/add-to-campaign: config.campaign is required")
	}
	if !IsCampaignID(c.Campaign) {
		return c, fmt.Errorf("instantly/add-to-campaign: campaign %q is not a campaign id; "+
			"the adapter takes the campaign's id (a lowercase UUID, in the campaign's URL in Instantly), not its name", c.Campaign)
	}
	if v, ok := raw["skip_if_in_campaign"].(bool); ok {
		c.SkipIfInCampaign = v
	}
	if vars, ok := raw["variables"].(map[string]any); ok {
		c.Variables = map[string]string{}
		for target, field := range vars {
			f, ok := field.(string)
			if !ok || strings.TrimSpace(f) == "" {
				return c, fmt.Errorf("instantly/add-to-campaign: variables: %q must map to a field name", target)
			}
			c.Variables[target] = f
		}
		// One lead field, one target (ADR-066 (3)).
		byField := map[string][]string{}
		for target := range c.Variables {
			if f := leadField(target); f != "" {
				byField[f] = append(byField[f], target)
			}
		}
		for f, targets := range byField {
			if len(targets) > 1 {
				sort.Strings(targets)
				return c, &contractError{fmt.Sprintf("instantly/add-to-campaign: variables: %s all fill the lead's %s; "+
					"keep one", strings.Join(targets, " and "), f)}
			}
		}
	}
	if v, ok := raw["base_url"].(string); ok && v != "" {
		c.BaseURL = v
	}
	return c, nil
}

// Run implements adapters.Adapter.
func (a *Adapter) Run(ctx context.Context, p adapters.Ports) error {
	r := protocol.NewReader(p.In)
	w := protocol.NewWriter(p.Out)

	var (
		cfg        config
		opened     bool
		campaignID string
		apiKey     = p.Getenv("INSTANTLY_API_KEY")
		delivered  int
		// accepts: this runner acts on ERROR (SPEC §5, ADR-065).
		accepts bool
	)

	for {
		m, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		switch m.Type {
		case protocol.TypeOpen:
			cfg, err = parseConfig(m.Config)
			if err != nil {
				return err
			}
			if apiKey == "" {
				return &httpx.Error{Kind: httpx.KindAuth, Provider: "instantly", Msg: "INSTANTLY_API_KEY is not set"}
			}
			// The campaign is named by its id (ADR-062): no lookup.
			campaignID = cfg.Campaign
			opened = true
			accepts = m.AcceptsType(protocol.TypeError)
			if m.Preflight {
				// A preflight session (SPEC §5, ADR-040): check the live
				// campaign against what this step sends; send nothing.
				destination, status, reason, checks := a.preflight(ctx, cfg, apiKey, campaignID)
				if err := w.Write(protocol.PreflightTo(destination, status, reason, checks)); err != nil {
					return err
				}
				return w.Write(protocol.End())
			}
			if err := w.Write(protocol.Schema([]byte(`{"type":"object","properties":{}}`))); err != nil {
				return err
			}
			if err := w.Write(protocol.Log("info", "instantly: campaign "+campaignID)); err != nil {
				return err
			}

		case protocol.TypeRecord:
			if !opened {
				return fmt.Errorf("instantly/add-to-campaign: received a record before OPEN")
			}
			if m.Key == nil {
				return fmt.Errorf("instantly/add-to-campaign: received a record with no key")
			}
			sent, created, err := a.addLead(ctx, cfg, apiKey, campaignID, m.Fields)
			if err != nil {
				// A full workspace refuses every later lead too: stop the
				// step (SPEC §10 item 6, §8 "A step that stops", ADR-065),
				// then exit 1 so a runner that predates ERROR still fails
				// the session.
				if accepts && isLeadLimit(err) {
					if werr := w.Write(protocol.Error(m.Key, protocol.VerdictFailRun, "instantly: "+leadLimitReason)); werr != nil {
						return werr
					}
					if werr := w.Write(protocol.End()); werr != nil {
						return werr
					}
				}
				return err
			}
			delivered++
			// An empty RECORD is the acknowledgement: delivered, nothing learned.
			if err := w.Write(protocol.Record(*m.Key, map[string]any{}, nil)); err != nil {
				return err
			}
			// Attestation (ADR-036): re-read what Instantly stored and report
			// the three-way verdict. A 2xx is never a delivery.
			status, reason := a.attest(ctx, cfg, apiKey, sent, created)
			if err := w.Write(protocol.Attest(*m.Key, status, reason)); err != nil {
				return err
			}
			if err := w.Write(protocol.Cost(m.Key, "instantly", 0, map[string]any{"leads": 1})); err != nil {
				return err
			}

		case protocol.TypeEnd:
			// Input complete; keep reading until EOF.
		}
	}

	if !opened {
		return fmt.Errorf("instantly/add-to-campaign: stream ended before OPEN")
	}
	if err := w.Write(protocol.Log("info", fmt.Sprintf("instantly: added %d leads", delivered))); err != nil {
		return err
	}
	return w.Write(protocol.End())
}

// attest re-reads the created lead and compares every non-blank field sent
// against what is stored (SPEC §6, ADR-036). confirmed: all present and
// equal. contradicted: a readable value says a field did not persist — the
// hard fail. inconclusive: the re-read failed, the create returned no id, or
// the shape carried no readable value for a sent field — reported ok with a
// warning, because the lead exists and will be mailed regardless, and a
// false "failed" invites a duplicate re-send by hand.
func (a *Adapter) attest(ctx context.Context, cfg config, apiKey string, sent leadRequest, created leadResponse) (string, string) {
	if strings.TrimSpace(created.ID) == "" {
		return protocol.AttestInconclusive, "the create response carried no lead id to re-read"
	}
	stored, err := a.getLead(ctx, cfg, apiKey, created.ID)
	if err != nil {
		return protocol.AttestInconclusive, "re-read failed: " + err.Error()
	}
	return compareLead(sent, stored)
}

// compareLead is the pure half of attest: sent vs stored, field by field.
// A field the response does not carry is unreadable — inconclusive, never
// contradicted by absence; a field it carries with a different value is
// contradicted.
func compareLead(sent leadRequest, stored storedLead) (string, string) {
	type check struct{ name, want string }
	checks := []check{
		{"email", strings.ToLower(sent.Email)},
		{"first_name", sent.FirstName},
		{"last_name", sent.LastName},
		{"company_name", sent.CompanyName},
		{"job_title", sent.JobTitle},
		{"personalization", sent.Personalization},
		{"website", sent.Website},
		{"phone", sent.Phone},
	}
	var unreadable []string
	for _, c := range checks {
		if c.want == "" {
			continue
		}
		got, ok := stored.field(c.name)
		if !ok {
			unreadable = append(unreadable, c.name)
			continue
		}
		if c.name == "email" {
			got = strings.ToLower(got)
		}
		if strings.TrimSpace(got) != strings.TrimSpace(c.want) {
			return protocol.AttestContradicted, fmt.Sprintf("%s: sent %q, stored %q", c.name, c.want, got)
		}
	}
	names := make([]string, 0, len(sent.CustomVariables))
	for name := range sent.CustomVariables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want := sent.CustomVariables[name]
		got, ok := stored.variable(name)
		switch {
		case !ok:
			unreadable = append(unreadable, "custom variable "+name)
		case strings.TrimSpace(got) != strings.TrimSpace(want):
			return protocol.AttestContradicted, fmt.Sprintf("custom variable %s: sent %q, stored %q", name, want, got)
		}
	}
	if len(unreadable) > 0 {
		return protocol.AttestInconclusive, "the re-read carried no readable value for " + strings.Join(unreadable, ", ")
	}
	return protocol.AttestConfirmed, "every field sent is present in the stored lead"
}

// preflight runs the four checks ADR-040 names against the live campaign:
// it exists and is Active; its sequence has at least the step count the
// copy assumes (the highest _step_N suffix among the variables: targets);
// every custom-variable target appears as {{name}} in some step; no variant
// lacks one; and every lead-field tag the sequence uses has a target filling
// its field (ADR-066).
// A readable failure is blocked; a target that cannot be read is
// inconclusive — never a block on a guess.
func (a *Adapter) preflight(ctx context.Context, cfg config, apiKey, campaignID string) (string, string, string, []protocol.Check) {
	destination := "campaign " + campaignID
	detail, err := a.getCampaign(ctx, cfg, apiKey, campaignID)
	if err != nil {
		return destination, protocol.PreflightInconclusive, "campaign could not be read: " + err.Error(), nil
	}
	// The receipt names the campaign the id points at (SPEC §5, ADR-062).
	if name := detail.name(); name != "" {
		destination = fmt.Sprintf("campaign %q (%s)", name, campaignID)
	}
	var checks []protocol.Check
	var blocked []string
	var unreadable []string

	if active, ok := detail.active(); !ok {
		unreadable = append(unreadable, "status")
	} else {
		checks = append(checks, protocol.Check{Name: "campaign active", OK: active, Detail: fmt.Sprintf("status %v", detail.Fields["status"])})
		if !active {
			blocked = append(blocked, "campaign is not Active")
		}
	}

	targets := make([]string, 0, len(cfg.Variables))
	for t := range cfg.Variables {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	assumed := assumedSteps(targets)

	steps, ok := detail.steps()
	if !ok {
		unreadable = append(unreadable, "sequence")
	} else {
		if assumed > 0 {
			okSteps := len(steps) >= assumed
			checks = append(checks, protocol.Check{Name: "sequence step count", OK: okSteps,
				Detail: fmt.Sprintf("copy assumes %d step(s), sequence has %d", assumed, len(steps))})
			if !okSteps {
				blocked = append(blocked, fmt.Sprintf("the copy assumes %d step(s) but the sequence has %d", assumed, len(steps)))
			}
		}
		for _, t := range targets {
			if leadField(t) != "" {
				continue // lead-body fields, not template merge fields
			}
			placeholder := "{{" + t + "}}"
			referenced := false
			for _, variants := range steps {
				for _, body := range variants {
					if strings.Contains(body, placeholder) {
						referenced = true
					}
				}
			}
			checks = append(checks, protocol.Check{Name: "variable " + t + " referenced", OK: referenced})
			if !referenced {
				blocked = append(blocked, fmt.Sprintf("no sequence step references %s", placeholder))
				continue
			}
			// A step's own copy (<x>_step_N) must be in every variant of
			// step N, or one variant sends with a hole where the email was.
			// Other variables are decoration; a variant may omit them.
			if n := stepIndex(t); n > 0 && n <= len(steps) && len(steps[n-1]) > 1 {
				for _, body := range steps[n-1] {
					if !strings.Contains(body, placeholder) {
						checks = append(checks, protocol.Check{Name: fmt.Sprintf("every variant of step %d carries %s", n, t), OK: false})
						blocked = append(blocked, fmt.Sprintf("step %d has a variant without %s", n, placeholder))
						break
					}
				}
			}
		}
		// The other direction (ADR-066 (4)): a lead-field tag the sequence
		// uses must have a target filling that field, or every lead sends
		// with a hole where the tag stands.
		filled := map[string]bool{}
		for t := range cfg.Variables {
			if f := leadField(t); f != "" {
				filled[f] = true
			}
		}
		for _, lt := range leadFieldTags {
			used := false
			for _, variants := range steps {
				for _, body := range variants {
					if strings.Contains(body, lt.tag) {
						used = true
					}
				}
			}
			if !used {
				continue
			}
			checks = append(checks, protocol.Check{Name: "sequence tag " + lt.tag + " filled", OK: filled[lt.field]})
			if !filled[lt.field] {
				blocked = append(blocked, fmt.Sprintf("the sequence uses %s but no variables: target fills %s: add `%s: <field>`",
					lt.tag, lt.field, lt.field))
			}
		}
	}

	switch {
	case len(blocked) > 0:
		return destination, protocol.PreflightBlocked, strings.Join(blocked, "; "), checks
	case len(unreadable) > 0:
		return destination, protocol.PreflightInconclusive, "the campaign response carried no readable " + strings.Join(unreadable, ", "), checks
	default:
		return destination, protocol.PreflightOK, "", checks
	}
}

// stepIndex reads the N of a <x>_step_N target, 0 when it has none.
func stepIndex(target string) int {
	i := strings.LastIndex(target, "_step_")
	if i < 0 {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(target[i+len("_step_"):], "%d", &n); err != nil {
		return 0
	}
	return n
}

// assumedSteps is the highest _step_N suffix among the variable targets —
// how many sequence steps the copy was written for.
func assumedSteps(targets []string) int {
	max := 0
	for _, t := range targets {
		if n := stepIndex(t); n > max {
			max = n
		}
	}
	return max
}
