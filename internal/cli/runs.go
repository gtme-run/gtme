package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/gtme-run/gtme/internal/ledger"
	"github.com/gtme-run/gtme/internal/runner"
)

// cmdRuns lists runs, or prints one run's receipt (SPEC §8).
func cmdRuns(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	limit := fs.Int("limit", 20, "how many runs to list")
	positional, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return fail(ExitValidation, "usage: gtme runs [RUN_ID|last]")
	}

	l, err := openLedger(ctx)
	if err != nil {
		return err
	}
	defer l.Close()

	if len(positional) == 0 {
		runs, err := l.ListRuns(ctx, *limit)
		if err != nil {
			return fail(ExitOther, "%v", err)
		}
		if len(runs) == 0 {
			fmt.Fprintln(env.Stderr, "no runs yet")
			return nil
		}
		tw := tabwriter.NewWriter(env.Stderr, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "run\tpipeline\tstatus\tstarted\trecords\tin flight")
		for _, run := range runs {
			records, err := l.RunRecords(ctx, run.ID)
			if err != nil {
				return fail(ExitOther, "%v", err)
			}
			status, err := runStatus(ctx, l, run, len(records))
			if err != nil {
				return fail(ExitOther, "%v", err)
			}
			inFlight := "-"
			if run.Status == ledger.StatusPending {
				n, err := l.InFlight(ctx, run.ID)
				if err != nil {
					return fail(ExitOther, "%v", err)
				}
				inFlight = fmt.Sprint(n)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", run.ID, run.Pipeline, status, run.StartedAt, len(records), inFlight)
		}
		tw.Flush()
		return nil
	}

	target := positional[0]
	var run ledger.Run
	if target == "last" {
		run, err = l.LastRun(ctx)
	} else {
		run, err = l.GetRun(ctx, target)
	}
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return fail(ExitValidation, "unknown run %s", target)
		}
		return fail(ExitOther, "%v", err)
	}
	return printReceipt(ctx, env, l, run)
}

// printReceipt reconstructs a run's receipt from the ledger, so it can be read
// again long after the run finished.
func printReceipt(ctx context.Context, env Env, l *ledger.Ledger, run ledger.Run) error {
	recordsIn, err := l.RunRecords(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	status, err := runStatus(ctx, l, run, len(recordsIn))
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	interrupted := strings.HasPrefix(status, statusInterrupted)
	if interrupted && (run.Pid != 0 || run.Host != "") {
		status += " (was " + runner.ProcessLabel(run) + ")"
	}
	fmt.Fprintf(env.Stderr, "run %s\npipeline: %s\nstatus:   %s\nstarted:  %s\n",
		run.ID, run.Pipeline, status, run.StartedAt)
	if run.FinishedAt != "" {
		fmt.Fprintf(env.Stderr, "finished: %s\n", run.FinishedAt)
	}

	events, err := l.StepEventCounts(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	costs, err := l.CostsByStep(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	order, err := l.StepIDs(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}

	tw := tabwriter.NewWriter(env.Stderr, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "\nstep\tclaimed\tdone\tcached\tfailed\tcost")
	var total ledger.CostTotal
	for _, step := range order {
		counts := events[step]
		cost := costs[step]
		total.Add(cost)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", step,
			count(counts["claimed"]), count(counts["done"]),
			count(counts["skipped_cache"]), count(counts["failed"]), money(cost.Total()))
	}
	tw.Flush()
	// Withheld because the destination already has them (ADR-062): never
	// in the cached column.
	for _, step := range order {
		if n := events[step][ledger.AlreadyDelivered]; n > 0 {
			fmt.Fprintf(env.Stderr, "%s: %d already delivered\n", step, n)
		}
	}
	// The total carries its basis exactly as the live receipt did (ADR-046).
	fmt.Fprintf(env.Stderr, "total: %s\n", runner.FormatCost(total))

	// The states show where records stopped, which is the useful thing when a run
	// did not finish cleanly.
	records, err := l.RunRecords(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	states := map[string]int{}
	failed := 0
	for _, rr := range records {
		states[rr.State]++
		if rr.AnyFailed() {
			failed++
		}
	}
	fmt.Fprintf(env.Stderr, "records: %d", len(records))
	if len(states) > 0 {
		parts := make([]string, 0, len(states))
		for state, n := range states {
			parts = append(parts, fmt.Sprintf("%s=%d", state, n))
		}
		fmt.Fprintf(env.Stderr, " (%s)", strings.Join(parts, " "))
	}
	if failed > 0 {
		// A fail verdict is a filter stop or a withheld send (SPEC §8, ADR-031);
		// telling them apart needs step roles, which a bare run id does not carry.
		fmt.Fprintf(env.Stderr, ", %d with a fail verdict (filtered, or a send withheld)", failed)
	}
	fmt.Fprintln(env.Stderr)

	// The pipeline that produced this run, so it can be re-run or frozen.
	var config map[string]any
	if json.Unmarshal([]byte(run.ConfigJSON), &config) == nil {
		if steps, ok := config["steps"].([]any); ok {
			fmt.Fprintf(env.Stderr, "config:  %d steps recorded (`gtme freeze %s` rebuilds the pipeline)\n",
				len(steps), run.ID)
		}
	}
	// Deliveries this run held after a crash (ADR-060), and the release.
	held, err := l.UnconfirmedByRun(ctx, run.ID)
	if err != nil {
		return fail(ExitOther, "%v", err)
	}
	targets := make([]string, 0, len(held))
	for t := range held {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		fmt.Fprintf(env.Stderr, "held:     %d unconfirmed at %s — may have reached it before the run stopped; check the target, then: %s --resend-unconfirmed\n",
			held[t], t, resumeCommand(run))
	}
	if interrupted {
		fmt.Fprintf(env.Stderr, "resume:   %s\n", resumeCommand(run))
	}
	return nil
}

func count(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprint(n)
}

func money(v float64) string {
	if v == 0 {
		return "$0"
	}
	return fmt.Sprintf("$%.4f", v)
}

// runStatus is a run's status with three facts the bare word hides: a
// `running` run whose process is gone reads `interrupted` (ADR-061), a
// rehearsal is marked (SPEC §3, ADR-052 (7)), and a run that spent money and
// produced no records says so (SPEC §8, ADR-053) — the same words the live
// receipt used.
func runStatus(ctx context.Context, l *ledger.Ledger, run ledger.Run, records int) (string, error) {
	status, err := liveness(l, run)
	if err != nil {
		return "", err
	}
	if run.Dry {
		status += " (dry)"
	}
	if records == 0 {
		costs, err := l.CostsByStep(ctx, run.ID)
		if err != nil {
			return "", err
		}
		var total ledger.CostTotal
		for _, c := range costs {
			total.Add(c)
		}
		if total.Total() > 0 {
			status += " — 0 records, " + runner.FormatSpent(total)
		}
	}
	return status, nil
}

// Liveness words for a running run (SPEC §8, ADR-061).
const (
	statusInterrupted = "interrupted"
)

// liveness is a run's status as `gtme runs` shows it: a `running` run whose
// lock is free has no living process and reads `interrupted`; one recorded
// on another host cannot be probed from here and says where it runs. The
// stored status never changes — this is derived at read time.
func liveness(l *ledger.Ledger, run ledger.Run) (string, error) {
	if run.Status != ledger.StatusRunning {
		return run.Status, nil
	}
	if host, _ := os.Hostname(); run.Host != "" && run.Host != host {
		return fmt.Sprintf("running (on %s)", run.Host), nil
	}
	alive, err := l.RunAlive(run.ID)
	if err != nil {
		return "", err
	}
	if alive {
		return ledger.StatusRunning, nil
	}
	return statusInterrupted, nil
}

// resumeCommand is how an operator finishes a run. The ledger records the
// pipeline's name, not its file, so the file is named for the pipeline.
func resumeCommand(run ledger.Run) string {
	return fmt.Sprintf("gtme run %s.yaml --resume %s", run.Pipeline, run.ID)
}
