package runner

import (
	"context"

	"github.com/gtme-run/gtme/internal/ledger"
	"github.com/gtme-run/gtme/internal/planner"
)

// Record outcomes (SPEC §8, ADR-064): the receipt column a per-record step
// event counts in, carried as detail.outcome so `gtme runs RUN_ID` counts
// each record exactly as the live receipt did.
const (
	OutcomeOut              = "out"
	OutcomeEmpty            = "empty"
	OutcomeFiltered         = "filtered"
	OutcomeSkipped          = "skipped"
	OutcomeFailed           = "failed"
	OutcomeCached           = "cached"
	OutcomeAlreadyDelivered = "already_delivered"
	OutcomeSimulated        = "simulated"
	OutcomeHeldDry          = "held_dry"
)

// gated records a record a when: or membership gate held back at a step
// (ADR-064): it was eligible, so it counts in in, and the ledger says so.
func (r *runner) gated(ctx context.Context, st *planner.Step, identityID, gate string) error {
	if err := r.l.LogStepEvent(ctx, r.prov(st.ID), identityID, ledger.EventGated,
		map[string]any{"gate": gate}); err != nil {
		return err
	}
	r.bump(st, func(s *StepStat) { s.Gated++ })
	return nil
}
