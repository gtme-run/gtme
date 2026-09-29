package e2e

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// hangManifest and hangScript are an enrich adapter that answers records
// until it reaches the one named by hang_on, touches a marker file, and then
// hangs — a step the test can interrupt at a known point. Once the marker
// exists it answers everything, so the same file resumes cleanly.
const hangManifest = `{
  "id": "hang-enrich",
  "version": 1,
  "role": "enrich",
  "entity_type": "person",
  "needs": {"type":"object","properties":{"email":{"type":"string"}}},
  "provides": {"type":"object","additionalProperties":false,"properties":{"hang.seen":{"type":"string"}}},
  "config_schema": {"type":"object","additionalProperties":false,"properties":{"hang_on":{"type":"string"},"marker":{"type":"string"}}}
}`

const hangScript = `#!/usr/bin/env python3
import json, os, sys, time
PROVIDES = {"type":"object","additionalProperties":False,"properties":{"hang.seen":{"type":"string"}}}
config = {}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    msg = json.loads(line)
    if msg.get("type") == "OPEN":
        config = msg.get("config") or {}
        print(json.dumps({"type":"SCHEMA","provides":PROVIDES}), flush=True)
    elif msg.get("type") == "RECORD":
        key = msg["key"]
        if key.get("identity_key") == config.get("hang_on") and not os.path.exists(config["marker"]):
            open(config["marker"], "w").close()
            time.sleep(60)
        print(json.dumps({"type":"RECORD","key":key,"fields":{"hang.seen":"yes"}}), flush=True)
    elif msg.get("type") == "END":
        break
print(json.dumps({"type":"END"}), flush=True)
`

// startGtme runs gtme in the background and returns the command and its
// stderr buffer; the caller signals it and waits.
func (h *harness) startGtme(extraEnv []string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	h.t.Helper()
	cmd := exec.Command(gtmBin, args...)
	cmd.Dir = h.work
	cmd.Env = append(h.env(), extraEnv...)
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &errb
	if err := cmd.Start(); err != nil {
		h.t.Fatalf("starting gtme: %v", err)
	}
	return cmd, &errb
}

// waitForFile polls until path exists, failing the test after a deadline.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// TestInterruptFinishesTheRun is #135: Ctrl-C during an ordinary step ends
// the run failed in the ledger, as the receipt says, and every record the
// receipt counts as failed has its failed step event.
func TestInterruptFinishesTheRun(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.writeAdapter("hang-enrich", hangManifest, hangScript)
	marker := filepath.Join(h.work, "hung")
	h.write("hang.yaml", `name: interruptible
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: hang
    use: hang-enrich
    with:
      hang_on: bob@globex.io
      marker: `+marker+`
`)

	cmd, stderr := h.startGtme([]string{"GTME_CONCURRENCY=1"}, "run", "hang.yaml")
	waitForFile(t, marker)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("SIGINT: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("an interrupted run exited 0\nstderr:\n%s", stderr)
	}
	out := stderr.String()
	contains(t, out, "— failed", "stderr")
	if strings.Contains(out, "context canceled") {
		t.Errorf("finishing the run must not use the cancelled context\nstderr:\n%s", out)
	}
	if n := h.queryInt(`SELECT count(*) FROM runs WHERE status = 'running'`); n != 0 {
		t.Errorf("running runs = %d, want 0 — the receipt said failed", n)
	}
	if n := h.queryInt(`SELECT count(*) FROM runs WHERE status = 'failed'`); n != 1 {
		t.Errorf("failed runs = %d, want 1", n)
	}
	// jane answered before the hang; bob and carol were in the killed session.
	if n := h.queryInt(`SELECT count(*) FROM step_events
	  WHERE step_id = 'hang' AND event = 'failed' AND identity_id IS NOT NULL`); n != 2 {
		t.Errorf("per-record failed events = %d, want 2 (the receipt's failed records)\nstderr:\n%s", n, out)
	}
	if n := h.queryInt(`SELECT count(*) FROM step_events WHERE step_id = 'hang' AND event = 'done'`); n != 1 {
		t.Errorf("done events = %d, want 1", n)
	}

	// The interrupted run resumes: the answered record is not redone.
	h.write("hang.yaml", `name: interruptible
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: hang
    use: hang-enrich
`)
	res := h.mustRun("run", "hang.yaml", "--resume", "last")
	contains(t, res.stderr, "hang: 2 in, 2 out", "stderr")
	if n := h.queryInt(`SELECT count(*) FROM runs WHERE status = 'done'`); n != 1 {
		t.Errorf("done runs = %d, want 1 after the resume", n)
	}
}

// TestKilledRunReadsInterrupted is ADR-061: a live run holds its lock, so
// `gtme runs` shows it running and --resume refuses it; after kill -9 the
// lock is free, `gtme runs` shows interrupted without writing the ledger,
// a plain run says so and starts fresh, and the resume finishes it.
func TestKilledRunReadsInterrupted(t *testing.T) {
	h := newHarness(t)
	h.write("people.csv", peopleCSV)
	h.writeAdapter("hang-enrich", hangManifest, hangScript)
	marker := filepath.Join(h.work, "hung")
	h.write("hang.yaml", `name: interruptible
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: hang
    use: hang-enrich
    with:
      hang_on: bob@globex.io
      marker: `+marker+`
`)

	cmd, stderr := h.startGtme([]string{"GTME_CONCURRENCY=1"}, "run", "hang.yaml")
	defer cmd.Process.Kill()
	waitForFile(t, marker)
	runID := h.queryStrings(`SELECT id FROM runs`)[0]

	listing := h.mustRun("runs")
	contains(t, listing.stderr, runID, "gtme runs")
	if !strings.Contains(listing.stderr, "running") || strings.Contains(listing.stderr, "interrupted") {
		t.Errorf("a live run must list as running\n%s", listing.stderr)
	}
	res := h.run("run", "hang.yaml", "--resume", runID)
	if res.code != 2 {
		t.Errorf("resuming a live run: exit = %d, want 2", res.code)
	}
	contains(t, res.stderr, "run "+runID+" is still running (pid", "stderr")

	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("kill -9: %v", err)
	}
	cmd.Wait()
	_ = stderr

	before := ledgerContent(t, h)
	listing = h.mustRun("runs")
	contains(t, listing.stderr, "interrupted", "gtme runs")
	receipt := h.mustRun("runs", runID)
	contains(t, receipt.stderr, "status:   interrupted (was pid ", "receipt")
	contains(t, receipt.stderr, "resume:   gtme run interruptible.yaml --resume "+runID, "receipt")
	if after := ledgerContent(t, h); after != before {
		t.Errorf("gtme runs wrote to the ledger")
	}
	if n := h.queryInt(`SELECT count(*) FROM runs WHERE status = 'running'`); n != 1 {
		t.Errorf("stored running runs = %d, want 1 — interrupted is derived, never stored", n)
	}

	// A plain run of the pipeline says the last one was interrupted and
	// starts fresh; it never resumes by itself.
	fresh := h.mustRun("run", "hang.yaml")
	contains(t, fresh.stderr, "run "+runID+" of \"interruptible\" was interrupted; this starts a new run. To finish that one instead: gtme run hang.yaml --resume "+runID, "stderr")
	if n := h.queryInt(`SELECT count(*) FROM runs`); n != 2 {
		t.Errorf("runs = %d, want 2", n)
	}

	res = h.mustRun("run", "hang.yaml", "--resume", runID)
	contains(t, res.stderr, "resuming run "+runID, "stderr")
	if strings.Contains(res.stderr, "the pipeline changed") {
		t.Errorf("an unchanged pipeline must not report a config change\nstderr:\n%s", res.stderr)
	}
	if got := h.queryStrings(`SELECT status FROM runs WHERE id = ?`, runID)[0]; got != "done" {
		t.Errorf("status after resume = %s, want done", got)
	}
}

// ledgerContent fingerprints every row of every table, to prove a read verb
// wrote nothing. (The files' bytes are not the test: opening a ledger that a
// killed process left mid-WAL lets SQLite checkpoint, which moves pages
// without changing a row.)
func ledgerContent(t *testing.T, h *harness) string {
	t.Helper()
	l := h.open()
	defer l.Close()
	tables := h.queryStrings(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	sum := sha256.New()
	for _, table := range tables {
		rows, err := l.DB().Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(sum, "%s%v\n", table, vals)
		}
		rows.Close()
	}
	return fmt.Sprintf("%x", sum.Sum(nil))
}
