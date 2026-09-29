package runner

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/internal/ledger"
)

// PrintReceipt writes the end-of-run receipt: records in and out per step, cache
// skips, cost per step and total, and cost avoided via cache (SPEC §8). It goes
// to stderr, because stdout is data.
func PrintReceipt(w io.Writer, res *Result) {
	title := res.Status
	switch {
	case res.Simulated:
		title += " (SIMULATED — recorded responses only; nothing sent, nothing persisted)"
	case res.DryRun:
		title += " (dry run — nothing sent)"
	}
	if res.Status == "pending" {
		awaiting := ""
		for _, s := range res.Steps {
			if s.InFlight > 0 && s.Awaiting != "" {
				awaiting = s.Awaiting
				break
			}
		}
		switch {
		case awaiting != "" && res.Interrupted:
			title += fmt.Sprintf(" — interrupted; the rest awaits %s: `gtme answer %s` records, the next `gtme run %s` collects", awaiting, res.Pipeline, res.Pipeline)
		case awaiting != "":
			title += fmt.Sprintf(" — ended awaiting %s: `gtme answer %s` records, the next `gtme run %s` collects", awaiting, res.Pipeline, res.Pipeline)
		default:
			title += " — ended with a step in flight; the next `gtme run` of this pipeline collects"
		}
	}
	// A paid run that produced no records says so (SPEC §8, ADR-053) —
	// information on the receipt, not a new exit code.
	if paidForNothing(res) {
		var spent ledger.CostTotal
		for _, s := range res.Steps {
			spent.Add(s.Cost)
		}
		title += " — 0 records, " + FormatSpent(spent)
	}
	fmt.Fprintf(w, "\nrun %s — %s\n", res.RunID, title)

	PrintTable(w, res.Steps, nil)
	// Failures, with their reasons (SPEC §8: every error names its fix). One
	// line per distinct reason, most frequent first; a bare count in the
	// table would leave a missing key looking like bad data.
	for _, s := range res.Steps {
		if len(s.FailReasons) == 0 {
			continue
		}
		reasons := make([]string, 0, len(s.FailReasons))
		for reason := range s.FailReasons {
			reasons = append(reasons, reason)
		}
		sort.Slice(reasons, func(i, j int) bool {
			if s.FailReasons[reasons[i]] != s.FailReasons[reasons[j]] {
				return s.FailReasons[reasons[i]] > s.FailReasons[reasons[j]]
			}
			return reasons[i] < reasons[j]
		})
		for _, reason := range reasons {
			fmt.Fprintf(w, "%s: %d failed — %s\n", s.ID, s.FailReasons[reason], reason)
		}
	}
	// Declared fields absent at dispatch (SPEC §7, ADR-053): the gap is
	// visible whether or not the operator chose a policy.
	for i := range res.Steps {
		if s := &res.Steps[i]; s.Missing > 0 {
			fmt.Fprintf(w, "%s: %s — dispatched anyway (on_missing: run); set on_missing: skip or fail to hold them\n", s.ID, missingNote(s))
		}
	}

	// Simulation gaps (SPEC §8): a binding without fixtures, or a credentialed
	// process adapter with nothing to serve, is surfaced — never silently passed.
	for _, s := range res.Steps {
		if !s.SimGap {
			continue
		}
		if s.SimGapRecords > 0 {
			fmt.Fprintf(w, "simulation gap: %s (%s) — %d record(s) passed through untouched (no recorded responses to serve)\n",
				s.ID, s.Use, s.SimGapRecords)
		} else {
			fmt.Fprintf(w, "simulation gap: %s (%s) — no recorded responses to serve\n", s.ID, s.Use)
		}
	}

	// Suppression holds (SPEC §8, ADR-021): a chosen contact policy, receipted.
	for _, s := range res.Steps {
		if len(s.Suppressed) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s: %d record(s) suppressed:\n", s.ID, len(s.Suppressed))
		for _, sr := range s.Suppressed {
			fmt.Fprintf(w, "  %s: touched in %q %s ago\n", sr.IdentityKey, sr.Group, sr.Age)
		}
	}
	// The membership terminus (SPEC §8, ADR-021).
	switch {
	case res.TerminusGroup == "":
	case res.DryRun || res.Simulated:
		fmt.Fprintf(w, "group %q: %d record(s) would be added (held back — %s)\n",
			res.TerminusGroup, res.TerminusWould, holdReason(res))
	default:
		fmt.Fprintf(w, "group %q: %d record(s) added\n", res.TerminusGroup, res.TerminusAdded)
	}

	// Traverses (SPEC §8, ADR-054): the children each traverse minted and
	// coalesced, apart from the parents the table reconciles.
	for _, s := range res.Steps {
		if s.Role != adapters.RoleTraverse {
			continue
		}
		fmt.Fprintf(w, "%s: %d parent(s) in, %d out, %d empty — %d traversed (%s), %d already in this run\n",
			s.ID, s.In, s.Out, s.Empty, s.Traversed, s.ChildType, s.Coalesced)
	}
	// Handoffs (SPEC §8, ADR-032): what each group/deliver step committed to
	// its group, or would have.
	for _, s := range res.Steps {
		switch {
		case s.TargetGroup == "":
		case s.GroupWould > 0:
			fmt.Fprintf(w, "%s: %d record(s) would be handed off to group %q (held back — %s)\n",
				s.ID, s.GroupWould, s.TargetGroup, holdReason(res))
		default:
			fmt.Fprintf(w, "%s: %d record(s) handed off to group %q\n", s.ID, s.GroupAdded, s.TargetGroup)
		}
	}
	// Preflight (SPEC §8, ADR-040): the target's side of the story, before
	// anything was sent.
	for _, s := range res.Steps {
		if s.Preflight == "" {
			continue
		}
		names := make([]string, 0, len(s.PreflightChecks))
		for _, c := range s.PreflightChecks {
			mark := "✓"
			if !c.OK {
				mark = "✗"
			}
			names = append(names, mark+" "+c.Name)
		}
		// The destination names what a step configured by id delivers to
		// (SPEC §8, ADR-062).
		dest := ""
		if s.PreflightDestination != "" {
			dest = s.PreflightDestination + " — "
		}
		switch s.Preflight {
		case "ok":
			fmt.Fprintf(w, "%s: preflight ok — %s%d check(s)", s.ID, dest, len(s.PreflightChecks))
		case "blocked":
			fmt.Fprintf(w, "%s: preflight BLOCKED — %s%s", s.ID, dest, s.PreflightReason)
		case "simulated":
			fmt.Fprintf(w, "%s: preflight skipped — %s", s.ID, s.PreflightReason)
		default:
			fmt.Fprintf(w, "%s: preflight inconclusive — %s%s (proceeded)", s.ID, dest, s.PreflightReason)
		}
		if len(names) > 0 {
			fmt.Fprintf(w, " (%s)", strings.Join(names, ", "))
		}
		fmt.Fprintln(w)
	}
	// In flight (SPEC §8, ADR-038): what a deferred step left with the
	// provider, and how to collect it — or, for a human/agent step
	// (ADR-049), who is awaited and the verb that answers.
	for _, s := range res.Steps {
		if s.InFlight == 0 {
			continue
		}
		if s.Awaiting != "" {
			fmt.Fprintf(w, "%s: %d in, %d out — %d awaiting %s; `gtme answer %s` records, the next `gtme run %s` collects (or `gtme show --run %s --pending %s` to read them)\n",
				s.ID, s.In, s.Out, s.InFlight, s.Awaiting, res.Pipeline, res.Pipeline, res.RunID, s.ID)
			continue
		}
		fmt.Fprintf(w, "%s: %d record(s) in flight (%s); the next `gtme run` of this pipeline collects, or `gtme run --resume %s`\n",
			s.ID, s.InFlight, strings.Join(s.Tokens, ", "), res.RunID)
	}
	// Held deliveries (SPEC §8, ADR-060): each may have reached the target
	// before a crash, so none was sent again. They are named, with the one
	// command that releases them, grouped by the run that held them.
	for _, s := range res.Steps {
		byRun := map[string][]string{}
		var runs []string
		for _, h := range s.Unconfirmed {
			if _, ok := byRun[h.RunID]; !ok {
				runs = append(runs, h.RunID)
			}
			byRun[h.RunID] = append(byRun[h.RunID], h.IdentityKey)
		}
		for _, run := range runs {
			keys := byRun[run]
			sort.Strings(keys)
			fmt.Fprintf(w, "%s: %d record(s) may have reached %s before run %s stopped and were not sent again:\n", s.ID, len(keys), s.Use, run)
			for _, k := range keys {
				fmt.Fprintf(w, "  %s\n", k)
			}
			fmt.Fprintf(w, "Check the target, then: gtme run %s --resume %s --resend-unconfirmed\n", pipelineArg(res), run)
			fmt.Fprintln(w, "(or --settle-unconfirmed for the ones it already has; either takes =KEY,… to name some)")
		}
	}
	// Attestation (SPEC §8, ADR-036): accepted is never sent; an attesting
	// adapter's confirmed/contradicted refine it, and every inconclusive
	// delivery is named — accepted, not confirmed.
	for _, s := range res.Steps {
		if !s.Attests {
			continue
		}
		fmt.Fprintf(w, "%s: attested %d confirmed, %d contradicted, %d inconclusive (deliveries are accepted, never sent, until a provider attests)\n",
			s.ID, s.Confirmed, s.Contradicted, len(s.Inconclusive))
		for _, a := range s.Inconclusive {
			fmt.Fprintf(w, "  %s: accepted, not confirmed — %s\n", a.IdentityKey, a.Reason)
		}
	}
	// Deliver records held back by on_missing, each with its reason (SPEC §8).
	for _, s := range res.Steps {
		if len(s.MissingSkips) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s: %d record(s) held back by on_missing:\n", s.ID, len(s.MissingSkips))
		for _, rv := range s.MissingSkips {
			fmt.Fprintf(w, "  %s: missing %s\n", rv.IdentityKey, strings.Join(rv.Missing, ", "))
		}
	}
	// The dry-run approval artifact (SPEC §8, ADR-019): each record's RESOLVED
	// variables, exactly what an armed run would send.
	for _, s := range res.Steps {
		if len(s.DryRun) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s: resolved variables for %d record(s) — review, then run again without --dry-run to arm:\n", s.ID, len(s.DryRun))
		for _, rv := range s.DryRun {
			fmt.Fprintf(w, "  %s\n", rv.IdentityKey)
			targets := make([]string, 0, len(rv.Resolved))
			for t := range rv.Resolved {
				targets = append(targets, t)
			}
			sort.Strings(targets)
			for _, t := range targets {
				fmt.Fprintf(w, "    %s: %q\n", t, rv.Resolved[t])
			}
		}
	}

	fmt.Fprintln(w, TotalLine(res.Steps))
}

// paidForNothing reports a run that spent money and sourced no records
// (SPEC §8, ADR-053).
func paidForNothing(res *Result) bool {
	sourced, spent := 0, 0.0
	for _, s := range res.Steps {
		if s.Role == adapters.RoleSource {
			sourced += s.Out + s.Empty
		}
		spent += s.Cost.Total()
	}
	return sourced == 0 && spent > 0
}

func holdReason(res *Result) string {
	if res.Simulated {
		return "simulated run"
	}
	return "dry run"
}

func money(v float64) string {
	if v == 0 {
		return "$0"
	}
	return fmt.Sprintf("$%.4f", v)
}

// FormatCost renders a total with its basis (SPEC §8, ADR-046): a purely
// measured total prints bare; a purely estimated one `$X (estimated)`; a
// mixed run splits — `$X ($Y measured + $Z estimated)`. A total with no
// estimated rows at all (nothing spent, or every dollar measured) is bare.
// `gtme runs` prints the same string from the ledger.
func FormatCost(c ledger.CostTotal) string {
	switch {
	case c.Estimates == 0:
		return money(c.Total())
	case c.Measured == 0:
		return money(c.Estimated) + " (estimated)"
	default:
		return fmt.Sprintf("%s (%s measured + %s estimated)", money(c.Total()), money(c.Measured), money(c.Estimated))
	}
}

// FormatSpent renders "$X spent" with its basis after the verb — `$4.10
// spent (estimated)` — the phrase SPEC §8's paid-zero-record mark uses on
// the receipt and in `gtme runs`.
func FormatSpent(c ledger.CostTotal) string {
	switch {
	case c.Estimates == 0:
		return money(c.Total()) + " spent"
	case c.Measured == 0:
		return money(c.Estimated) + " spent (estimated)"
	default:
		return fmt.Sprintf("%s spent (%s measured + %s estimated)", money(c.Total()), money(c.Measured), money(c.Estimated))
	}
}

func dash(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprint(n)
}

// Summary is a one-line description of a result, for logs.
func Summary(res *Result) string {
	parts := make([]string, 0, len(res.Steps))
	for _, s := range res.Steps {
		parts = append(parts, fmt.Sprintf("%s=%d", s.ID, s.Out))
	}
	return strings.Join(parts, " ")
}

// pipelineArg is the pipeline file for a command the receipt prints: the one
// the operator ran, else one named for the pipeline.
func pipelineArg(res *Result) string {
	if res.PipelinePath != "" {
		return res.PipelinePath
	}
	return res.Pipeline + ".yaml"
}

// PrintTable writes the receipt's table and its already-delivered lines
// (SPEC §8). The live receipt and `gtme runs RUN_ID` both print through it
// (ADR-064), so the two cannot drift. inUnknown marks steps whose in is a
// floor: a run recorded before ADR-064 never logged gated records.
func PrintTable(w io.Writer, steps []StepStat, inUnknown map[string]bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "step\tadapter\tin\tout\tempty\tcached\tfiltered\tfailed\tcost\tavoided")
	for _, s := range steps {
		avoided := "-"
		if s.CacheSkips > 0 {
			switch {
			case s.AvoidedUnknown && s.AvoidedUSD == 0:
				avoided = "?"
			case s.AvoidedUnknown:
				avoided = fmt.Sprintf("$%.4f+?", s.AvoidedUSD)
			default:
				avoided = fmt.Sprintf("$%.4f", s.AvoidedUSD)
			}
		}
		in := fmt.Sprint(s.In)
		if inUnknown[s.ID] {
			in += "+?"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%d\t%s\t%s\t%s\t%s\n",
			s.ID, s.Use, in, s.Out, dash(s.Empty), s.CacheSkips,
			dash(s.Filtered), dash(s.Failed), money(s.Cost.Total()), avoided)
	}
	tw.Flush()
	// Withheld because the destination already has them (SPEC §8, ADR-062):
	// outside the cached column and the avoided total.
	for _, s := range steps {
		if s.AlreadyDelivered > 0 {
			fmt.Fprintf(w, "%s: %d already delivered\n", s.ID, s.AlreadyDelivered)
		}
	}
}

// TotalLine is the receipt's last line: what was spent, with its basis, and
// what the cache avoided (SPEC §8, ADR-046).
func TotalLine(steps []StepStat) string {
	var totalCost ledger.CostTotal
	var totalAvoided float64
	avoidedUnknown := false
	totalSkips := 0
	for _, s := range steps {
		totalSkips += s.CacheSkips
		if s.AvoidedUnknown {
			avoidedUnknown = true
		}
		totalCost.Add(s.Cost)
		totalAvoided += s.AvoidedUSD
	}
	total := fmt.Sprintf("total: %s spent", FormatCost(totalCost))
	if totalSkips > 0 {
		amount := fmt.Sprintf("$%.4f", totalAvoided)
		if avoidedUnknown {
			// Some skipped adapters publish no cost_estimate_usd, so the saving is a
			// floor, not a total (SPEC §8).
			amount += "+?"
		}
		total += fmt.Sprintf(", %s avoided via cache (%d records skipped)", amount, totalSkips)
	}
	return total
}
