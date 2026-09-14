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
// provenance carries; a field written by a text step of another pipeline
// cannot be traced and counts as fetched — the safe reading. Iterates to a
// fixed point, since a text step may read another's output.
func (r *runner) textFetched(fetched []string, rec ledger.Record) []string {
	set := map[string]bool{}
	for _, f := range fetched {
		set[f] = true
	}
	type textValue struct {
		name string
		sig  string
	}
	var candidates []textValue
	for name, v := range rec.Values {
		if set[name] {
			continue
		}
		src := strings.TrimSpace(v.Source)
		if !strings.HasPrefix(src, "text/") {
			continue
		}
		_, sig, _ := strings.Cut(src, "#")
		candidates = append(candidates, textValue{name: name, sig: sig})
	}
	if len(candidates) == 0 {
		return fetched
	}
	for changed := true; changed; {
		changed = false
		for _, c := range candidates {
			if set[c.name] {
				continue
			}
			st := r.textStepBySignature(c.sig)
			if st == nil {
				set[c.name] = true
				changed = true
				continue
			}
			for _, read := range append(append([]string(nil), st.Uses...), st.Of) {
				if read != "" && set[read] {
					set[c.name] = true
					changed = true
					break
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for _, c := range candidates {
		if set[c.name] {
			out = append(out, c.name)
		}
	}
	return append(fetched, out...)
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
