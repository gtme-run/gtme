package runner

import (
	"context"
	"fmt"

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

// heldSelection checks --resend-unconfirmed and --settle-unconfirmed against
// what the run holds (SPEC §8, ADR-064) and returns the deliveries to
// settle. A key list names identity keys, as the receipt does; a key the run
// does not hold, or one given to both flags, refuses before anything is
// written. Without a list a flag takes every held record.
func (r *runner) heldSelection(ctx context.Context, runID string, o Options) ([]ledger.HeldDelivery, error) {
	if !o.ResendUnconfirmed && !o.SettleUnconfirmed {
		return nil, nil
	}
	held, err := r.l.HeldByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	holds := map[string]bool{}
	for _, h := range held {
		holds[h.IdentityKey] = true
	}
	keys := func(flag string, list []string) (map[string]bool, error) {
		if len(list) == 0 {
			return nil, nil
		}
		set := map[string]bool{}
		for _, k := range list {
			if !holds[k] {
				return nil, &RefusedError{fmt.Sprintf("%s: run %s holds no unconfirmed delivery for %s", flag, runID, k)}
			}
			set[k] = true
		}
		return set, nil
	}
	resendKeys, err := keys("--resend-unconfirmed", o.ResendKeys)
	if err != nil {
		return nil, err
	}
	settleKeys, err := keys("--settle-unconfirmed", o.SettleKeys)
	if err != nil {
		return nil, err
	}
	// With both flags, a flag without a list takes the records the other
	// did not name — settle some, send the rest — and no record may be
	// named by both. Both without lists would claim every record twice.
	if o.ResendUnconfirmed && o.SettleUnconfirmed {
		switch {
		case resendKeys == nil && settleKeys == nil:
			return nil, &RefusedError{"--resend-unconfirmed and --settle-unconfirmed both name every held record; give one of them =KEY,… to split them"}
		case resendKeys == nil:
			resendKeys = map[string]bool{}
			for k := range holds {
				if !settleKeys[k] {
					resendKeys[k] = true
				}
			}
		case settleKeys == nil:
			settleKeys = map[string]bool{}
			for k := range holds {
				if !resendKeys[k] {
					settleKeys[k] = true
				}
			}
		default:
			for k := range settleKeys {
				if resendKeys[k] {
					return nil, &RefusedError{fmt.Sprintf("%s is named by both --resend-unconfirmed and --settle-unconfirmed; name each held record once", k)}
				}
			}
		}
	}
	r.resendKeys = resendKeys
	var settle []ledger.HeldDelivery
	if o.SettleUnconfirmed {
		for _, h := range held {
			if settleKeys == nil || settleKeys[h.IdentityKey] {
				settle = append(settle, h)
			}
		}
	}
	return settle, nil
}

// settleHeld marks each delivery settled and records a settled step event
// at the deliver step that targets it (ADR-064). Nothing is sent.
func (r *runner) settleHeld(ctx context.Context, settle []ledger.HeldDelivery) error {
	for _, h := range settle {
		if err := r.l.SettleDelivery(ctx, h.Target, h.Scope, h.Idempotency); err != nil {
			return err
		}
		step := h.Target
		for i := range r.plan.Steps {
			if st := &r.plan.Steps[i]; st.IsDeliver && st.Target() == h.Target && deliveryScope(st) == h.Scope {
				step = st.ID
				break
			}
		}
		if err := r.l.LogStepEvent(ctx, r.prov(step), h.IdentityID, ledger.EventSettled,
			map[string]any{"target": h.Target, "scope": h.Scope}); err != nil {
			return err
		}
	}
	if n := len(settle); n > 0 {
		fmt.Fprintf(r.stderr, "settled %d held deliver%s: found at the target, not sent\n", n, map[bool]string{true: "y", false: "ies"}[n == 1])
	}
	return nil
}
