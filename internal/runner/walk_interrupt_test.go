package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/gtme-run/gtme/internal/adapters/all"
	"github.com/gtme-run/gtme/internal/ledger"
	"github.com/gtme-run/gtme/internal/pipeline"
	"github.com/gtme-run/gtme/internal/planner"
)

// TestInterruptedWalkLeavesRejectedRecordsSettled is #147: a person answers
// record 1 yes and record 2 no, then the walk is cut short at record 3.
// Only record 3 stays pending; the rejected record's verdict stands and is
// not re-pended behind it.
func TestInterruptedWalkLeavesRejectedRecordsSettled(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	csv := filepath.Join(dir, "people.csv")
	if err := os.WriteFile(csv, []byte("email,full_name,title\na@x.com,A,VP\nb@x.com,B,CMO\nc@x.com,C,CRO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yaml := filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(yaml, []byte(`name: gate
source:
  use: csv/source
  with:
    path: `+csv+`
steps:
  - id: vet
    use: human/filter
    uses: [title]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(ctx, filepath.Join(dir, "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p, err := pipeline.Load(yaml)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Build(ctx, p, l)
	if err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	res, err := Execute(ctx, Options{Ledger: l, Plan: plan, Stderr: &stderr, Concurrency: 1,
		Interactive: true, Stdin: strings.NewReader("y\nowns budget\nn\nno budget\n")})
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	var vet StepStat
	for _, s := range res.Steps {
		if s.ID == "vet" {
			vet = s
		}
	}
	if vet.Out != 1 || vet.Filtered != 1 || vet.InFlight != 1 {
		t.Errorf("vet: out=%d filtered=%d in flight=%d, want 1/1/1 (3 records, each once)\n%s",
			vet.Out, vet.Filtered, vet.InFlight, stderr.String())
	}
	n, err := l.InFlight(ctx, res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("records pending in the ledger = %d, want 1 (only the unanswered one)", n)
	}
}
