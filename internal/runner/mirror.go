package runner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gtme-run/gtme/internal/ledger"
)

// The receipt from the ledger (SPEC §8, ADR-064): `gtme runs RUN_ID` prints
// the live receipt's table, rebuilt from step_events and costs. It is the
// run's net outcome — each record counted once per step, by its latest
// outcome across every session of the run — so a run resumed three times
// reads as one run.

// Mirror is a run's receipt rows as the ledger records them.
type Mirror struct {
	Steps []StepStat
	// Legacy marks a run recorded before ADR-064: its columns are inferred
	// from each event and its reason, gated records were never recorded
	// (in may undercount where a step gates), and avoided is unknown.
	Legacy bool
	// GatedUnknown names the steps whose in is short by an unknown number
	// of gated records (Legacy runs only).
	GatedUnknown map[string]bool
}

// recordedStep is what the run's config snapshot says about a step.
type recordedStep struct {
	ID      string   `json:"id"`
	Use     string   `json:"use"`
	When    string   `json:"when"`
	Require []string `json:"require"`
	Exclude []string `json:"exclude"`
}

// childEvents are a traverse step's events about the children it minted:
// those records are the next segment's, not this step's inputs.
var childEvents = map[string]bool{"traversed": true, "coalesced": true}

// LedgerSteps rebuilds a run's receipt rows (ADR-064).
func LedgerSteps(ctx context.Context, l *ledger.Ledger, run ledger.Run) (Mirror, error) {
	var cfg struct {
		Source recordedStep   `json:"source"`
		Steps  []recordedStep `json:"steps"`
	}
	_ = json.Unmarshal([]byte(run.ConfigJSON), &cfg)
	if cfg.Source.ID == "" {
		cfg.Source.ID = "source"
	}
	events, err := l.RunStepEvents(ctx, run.ID)
	if err != nil {
		return Mirror{}, err
	}
	costs, err := l.CostsByStep(ctx, run.ID)
	if err != nil {
		return Mirror{}, err
	}

	m := Mirror{Legacy: true, GatedUnknown: map[string]bool{}}
	type latest struct {
		outcome string
		detail  map[string]any
	}
	seen := map[string]map[string]bool{}    // step → records with any event there
	final := map[string]map[string]latest{} // step → record → latest outcome
	var srcOut, srcFailed int
	// A step that stopped records why, and how many it never sent, on its
	// step-level failed event (SPEC §8, ADR-065). The latest stop counts,
	// less the records a later session of the run (a resume) first reached.
	type stopRecord struct {
		reason  string
		notSent int
		resumed int
	}
	stops := map[string]*stopRecord{}
	for _, e := range events {
		if e.IdentityID == "" && e.Event == "failed" {
			if s, ok := e.Detail["stopped"].(map[string]any); ok {
				reason, _ := s["reason"].(string)
				n, _ := s["not_sent"].(float64)
				stops[e.StepID] = &stopRecord{reason: reason, notSent: int(n)}
			}
		}
		if stop := stops[e.StepID]; stop != nil && e.IdentityID != "" && !seen[e.StepID][e.IdentityID] {
			stop.resumed++
		}
		if e.StepID == cfg.Source.ID {
			switch {
			case e.IdentityID == "" && e.Event == "done":
				if n, ok := e.Detail["records"].(float64); ok {
					srcOut = int(n)
				}
			case e.IdentityID == "" && e.Event == "failed" && e.Detail["reason"] != nil:
				srcFailed++ // a dropped record (SPEC §5), not the adapter's own death
			}
			continue
		}
		if e.IdentityID == "" || childEvents[e.Event] || e.Detail["child"] == true {
			continue
		}
		if seen[e.StepID] == nil {
			seen[e.StepID] = map[string]bool{}
			final[e.StepID] = map[string]latest{}
		}
		seen[e.StepID][e.IdentityID] = true
		if e.Event == ledger.EventGated {
			m.Legacy = false
		}
		outcome, recorded := e.Detail["outcome"].(string)
		if recorded {
			m.Legacy = false
		} else {
			outcome = inferOutcome(e)
		}
		if outcome != "" {
			final[e.StepID][e.IdentityID] = latest{outcome: outcome, detail: e.Detail}
		}
	}

	source := StepStat{ID: cfg.Source.ID, Use: cfg.Source.Use, Out: srcOut, Failed: srcFailed, Cost: costs[cfg.Source.ID]}
	m.Steps = append(m.Steps, source)

	// Steps after a failure never ran, and the live receipt omits them: a
	// finished run lists every step, an unfinished one up to the last step
	// the ledger saw.
	last := len(cfg.Steps) - 1
	if run.Status != ledger.StatusDone && run.Status != ledger.StatusPending {
		for last >= 0 && seen[cfg.Steps[last].ID] == nil && costs[cfg.Steps[last].ID].Total() == 0 {
			last--
		}
	}
	for _, rs := range cfg.Steps[:last+1] {
		s := StepStat{ID: rs.ID, Use: rs.Use, In: len(seen[rs.ID]), Cost: costs[rs.ID]}
		if stop := stops[rs.ID]; stop != nil && stop.notSent > stop.resumed {
			// The records never sent have no event there, yet were eligible:
			// in counts them, and the line names them (ADR-065).
			s.NotSent = stop.notSent - stop.resumed
			s.In += s.NotSent
			s.StopReason = stop.reason
		}
		for _, f := range final[rs.ID] {
			switch f.outcome {
			case OutcomeOut:
				s.Out++
			case OutcomeEmpty:
				s.Empty++
			case OutcomeFiltered:
				s.Filtered++
			case OutcomeFailed:
				s.Failed++
			case OutcomeSkipped:
				s.Skipped++
			case OutcomeAlreadyDelivered:
				s.AlreadyDelivered++
			case OutcomeCached:
				s.CacheSkips++
				// A reused filter judgment that failed is filtered too, as the
				// live line counts it.
				if f.detail["pass"] == false {
					s.Filtered++
				}
				if v, ok := f.detail["avoided_usd"].(float64); ok {
					s.AvoidedUSD += v
				} else {
					s.AvoidedUnknown = true
				}
			}
		}
		if m.Legacy && (rs.When != "" || len(rs.Require) > 0 || len(rs.Exclude) > 0) {
			m.GatedUnknown[rs.ID] = true
		}
		m.Steps = append(m.Steps, s)
	}
	return m, nil
}

// inferOutcome reads a column from an event written before ADR-064, the way
// the receipt read it: by event, verdict and reason.
func inferOutcome(e ledger.StepEvent) string {
	reason, _ := e.Detail["reason"].(string)
	switch e.Event {
	case "failed":
		return OutcomeFailed
	case "simulated":
		return OutcomeSimulated
	case "dry_run":
		return OutcomeHeldDry
	case "skipped_cache":
		if ledger.AlreadyDeliveredReason(reason) {
			return OutcomeAlreadyDelivered
		}
		return OutcomeCached
	case "done":
		switch {
		case e.Detail["skipped"] == true,
			strings.HasPrefix(reason, "suppressed:"),
			e.Detail["pass"] == false && strings.HasPrefix(reason, "missing "):
			return OutcomeSkipped
		case e.Detail["pass"] == false:
			return OutcomeFiltered
		}
		if n, ok := e.Detail["fields"].(float64); ok && n == 0 {
			return OutcomeEmpty
		}
		if n, ok := e.Detail["children"].(float64); ok && n == 0 {
			return OutcomeEmpty
		}
		return OutcomeOut
	}
	return ""
}
