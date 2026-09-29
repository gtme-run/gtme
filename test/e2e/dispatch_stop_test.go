package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lockerManifest and lockerScript are an enrich adapter that, on its first
// record, adds a trigger to the ledger that aborts every write of its
// field, so gtme's next ledger write fails — a runner-side error, not an
// adapter crash. Every session appends a line to
// the sessions file, so the test can count how many were opened.
const lockerManifest = `{
  "id": "ledger-locker",
  "version": 1,
  "role": "enrich",
  "entity_type": "person",
  "needs": {"type":"object","properties":{"email":{"type":"string"}}},
  "provides": {"type":"object","additionalProperties":false,"properties":{"locker.seen":{"type":"string"}}},
  "config_schema": {"type":"object","additionalProperties":false,"properties":{"ledger":{"type":"string"},"sessions":{"type":"string"}}}
}`

const lockerScript = `#!/usr/bin/env python3
import json, os, sqlite3, sys
PROVIDES = {"type":"object","additionalProperties":False,"properties":{"locker.seen":{"type":"string"}}}
config = {}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    msg = json.loads(line)
    if msg.get("type") == "OPEN":
        config = msg.get("config") or {}
        with open(config["sessions"], "a") as f:
            f.write("open\n")
        print(json.dumps({"type":"SCHEMA","provides":PROVIDES}), flush=True)
    elif msg.get("type") == "RECORD":
        if not os.path.exists(config["sessions"] + ".armed"):
            open(config["sessions"] + ".armed", "w").close()
            db = sqlite3.connect(config["ledger"], timeout=30)
            db.execute("""CREATE TRIGGER induced_failure BEFORE INSERT ON field_values
                          WHEN NEW.field = 'locker.seen'
                          BEGIN SELECT RAISE(ABORT, 'induced ledger failure'); END""")
            db.commit()
            db.close()
        print(json.dumps({"type":"RECORD","key":msg["key"],"fields":{"locker.seen":"yes"}}), flush=True)
    elif msg.get("type") == "END":
        break
print(json.dumps({"type":"END"}), flush=True)
`

// TestRunnerErrorStopsDispatch is #82: a runner-side failure inside a step
// (here, a ledger write that cannot get the lock) stops the step from
// dispatching further chunks — they would be sent, and for a paid step
// billed, with nothing recordable. The chunk in progress finishes; the
// undispatched records stay at the previous state for a resume.
func TestRunnerErrorStopsDispatch(t *testing.T) {
	h := newHarness(t)
	var csv strings.Builder
	csv.WriteString("email,full_name\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&csv, "p%03d@example.com,Person %03d\n", i, i)
	}
	h.write("people.csv", csv.String())
	h.writeAdapter("ledger-locker", lockerManifest, lockerScript)
	sessions := filepath.Join(h.work, "sessions")
	h.write("lock.yaml", `name: locked
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: lock
    use: ledger-locker
    with:
      ledger: `+h.ledger+`
      sessions: `+sessions+`
`)

	res := h.runWithEnv([]string{"GTME_CONCURRENCY=1"}, "", "run", "lock.yaml")
	if res.code == 0 {
		t.Fatalf("expected the ledger error to fail the run\nstderr:\n%s", res.stderr)
	}
	data, err := os.ReadFile(sessions)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "open"); n != 1 {
		t.Errorf("sessions opened = %d, want 1 — a runner error must stop dispatch\nstderr:\n%s", n, res.stderr)
	}
	if n := h.queryInt(`SELECT count(*) FROM run_records WHERE state = 'sourced'`); n < 100 {
		t.Errorf("records left at the previous state = %d, want the undispatched chunks (>= 100)", n)
	}
}
