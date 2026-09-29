package e2e

import (
	"bytes"
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
// hangs — a step the test can interrupt at a known point.
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
import json, sys, time
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
        if key.get("identity_key") == config.get("hang_on"):
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
