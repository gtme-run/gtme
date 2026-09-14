package runner

// text/* steps (SPEC §10 item 10, ADR-057): the template renderer. A
// compose with no model and no one behind it — the runner renders the
// step's template once per record over config.* and record.* and writes
// the result as the one field the step provides. Deterministic and free,
// so it runs identically armed and under --simulate; cached by the
// judgment cache like any participant (an unchanged record under an
// unchanged template is skipped_cache); provenance `text/compose @ #<sig>`.

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtme-run/gtme/internal/ledger"
	"github.com/gtme-run/gtme/internal/planner"
	"github.com/gtme-run/gtme/internal/template"
)

// runTextStep renders every eligible record.
func (r *runner) runTextStep(ctx context.Context, st *planner.Step, work []*item) error {
	if len(st.Provides) != 1 {
		return fmt.Errorf("runner: %s: text/compose provides exactly one field (got %d) — the planner should have refused this", st.ID, len(st.Provides))
	}
	field := st.Provides[0]
	source := r.source(st)
	config := template.Config(st.Config)
	for _, it := range work {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.l.LogStepEvent(ctx, r.prov(st.ID), it.identityID, "claimed", nil); err != nil {
			return err
		}
		out, err := template.Render(st.Template, config, it.fields)
		if err != nil {
			if err := r.failItem(ctx, st, it, err.Error()); err != nil {
				return err
			}
			continue
		}
		out = strings.TrimSpace(out)
		if out == "" {
			// An empty render writes nothing and the record continues
			// (ADR-057); a later step's needs decide what that means.
			if err := r.advance(ctx, st, it, map[string]any{"fields": 0, "empty_render": true}, nil); err != nil {
				return err
			}
			continue
		}
		fields := map[string]any{field: out}
		if err := st.ValidateProvides(fields); err != nil {
			if err := r.failItem(ctx, st, it, err.Error()); err != nil {
				return err
			}
			continue
		}
		if err := r.checkRegistry(it.key.EntityType, fields); err != nil {
			if err := r.failItem(ctx, st, it, err.Error()); err != nil {
				return err
			}
			continue
		}
		if _, err := r.l.WriteFieldMapAbout(ctx, it.identityID, source, r.prov(st.ID), fields, nil, it.referent); err != nil {
			return err
		}
		it.output = true
		if err := r.advance(ctx, st, it, map[string]any{"fields": 1}, fields); err != nil {
			return err
		}
	}
	r.printStepLine(st)
	return nil
}

// textFetched applies ADR-035's fence transitively (ADR-057): a field a
// text/* step wrote counts as externally fetched when any field that step
// read does. The writing step is found in this plan by the signature its
// provenance carries; what it read is projected from the ledger — the
// judging step's own projection holds only its uses:, never the page the
// text step read — and judged the same way, so a text step reading another
// text step's output is followed too (to a bounded depth). A text value
// whose step is not in this plan cannot be traced and counts as fetched —
// the safe reading.
func (r *runner) textFetched(ctx context.Context, identityID string, fetched []string, rec ledger.Record) ([]string, error) {
	out := append([]string(nil), fetched...)
	seen := map[string]bool{}
	for _, f := range fetched {
		seen[f] = true
	}
	for name, v := range rec.Values {
		if seen[name] || !strings.HasPrefix(strings.TrimSpace(v.Source), "text/") {
			continue
		}
		tainted, err := r.textValueFetched(ctx, identityID, v, 0)
		if err != nil {
			return nil, err
		}
		if tainted {
			out = append(out, name)
		}
	}
	return out, nil
}

// maxTextDepth bounds the chain of text steps the taint is followed through.
const maxTextDepth = 8

// textValueFetched reports whether a text/* value was rendered from any
// externally fetched input.
func (r *runner) textValueFetched(ctx context.Context, identityID string, v ledger.Value, depth int) (bool, error) {
	_, sig, _ := strings.Cut(strings.TrimSpace(v.Source), "#")
	st := r.textStepBySignature(sig)
	if st == nil || depth >= maxTextDepth {
		return true, nil
	}
	reads := append([]string(nil), st.Uses...)
	if st.Of != "" {
		reads = append(reads, st.Of)
	}
	if len(reads) == 0 {
		return false, nil
	}
	inputs, err := r.l.Project(ctx, identityID, ledger.Projection{Fields: reads})
	if err != nil {
		return false, err
	}
	for _, in := range inputs.Values {
		src := strings.TrimSpace(in.Source)
		if r.fetchedSource(src) {
			return true, nil
		}
		if strings.HasPrefix(src, "text/") {
			tainted, err := r.textValueFetched(ctx, identityID, in, depth+1)
			if err != nil || tainted {
				return tainted, err
			}
		}
	}
	return false, nil
}

// textStepBySignature finds this plan's text/* step with a signature.
func (r *runner) textStepBySignature(sig string) *planner.Step {
	if sig == "" {
		return nil
	}
	for i := range r.plan.Steps {
		st := &r.plan.Steps[i]
		if st.IsText() && r.judgmentSignature(st) == sig {
			return st
		}
	}
	return nil
}
