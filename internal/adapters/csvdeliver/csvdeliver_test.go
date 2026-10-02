package csvdeliver

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gtme-run/gtme/internal/protocol"
)

// Issue #164: a list or object variable lands in the CSV as JSON, the form
// dry-run and simulate print, not Go's %v form; strings are written as-is.
func TestAppendRowWritesNonScalarsAsJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	cfg, err := parseConfig(map[string]any{
		"path": path,
		"variables": map[string]any{
			"tools":   "tech_stack",
			"meta":    "extra",
			"name":    "full_name",
			"count":   "headcount",
			"big":     "revenue",
			"active":  "is_active",
			"missing": "not_there",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureHeader(cfg); err != nil {
		t.Fatal(err)
	}
	// Fields as they arrive off the wire: decoded JSON.
	fields := map[string]any{
		"tech_stack": []any{"HubSpot (Marketing automation)", "Salesforce (CRM)", "R&D <ops>"},
		"extra":      map[string]any{"b": 2.0, "a": "x"},
		"full_name":  "Ada Lovelace, \"Countess\"",
		"headcount":  42.0,
		"revenue":    2500000.0,
		"is_active":  true,
	}
	if err := appendRow(cfg, protocol.Key{IdentityKey: "email:ada@example.com"}, fields); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want header + 1", len(rows))
	}
	got := map[string]string{}
	for i, col := range rows[0] {
		got[col] = rows[1][i]
	}
	want := map[string]string{
		"identity_key": "email:ada@example.com",
		"tools":        `["HubSpot (Marketing automation)","Salesforce (CRM)","R&D <ops>"]`,
		"meta":         `{"a":"x","b":2}`,
		"name":         `Ada Lovelace, "Countess"`,
		"count":        "42",
		"big":          "2500000",
		"active":       "true",
		"missing":      "",
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("column %s = %q, want %q", col, got[col], w)
		}
	}
}

// Issue #185: appends to the file are serialized under an exclusive lock on
// it, so concurrent sessions (and processes) cannot interleave on a
// filesystem where O_APPEND alone does not keep a write whole. While another
// holder has the file locked, appendRow waits and writes nothing.
func TestAppendRowWaitsForTheFileLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	cfg, err := parseConfig(map[string]any{"path": path, "variables": map[string]any{"name": "full_name"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureHeader(cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	holder, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- appendRow(cfg, protocol.Key{IdentityKey: "email:ada@example.com"}, map[string]any{"full_name": "Ada"})
	}()
	select {
	case err := <-done:
		t.Fatalf("appendRow returned (%v) while another holder had the file locked", err)
	case <-time.After(200 * time.Millisecond):
	}
	if now, _ := os.ReadFile(path); string(now) != string(before) {
		t.Fatalf("appendRow wrote while the file was locked: %q", now)
	}

	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("appendRow did not finish after the lock was released")
	}
	if now, _ := os.ReadFile(path); string(now) != string(before)+"email:ada@example.com,Ada\n" {
		t.Fatalf("file after append = %q", now)
	}
}

// Issue #185: many sessions opening a new file and appending long, quoted
// rows at once leave the header first and one well-formed row each.
func TestConcurrentAppendsKeepEveryRowWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	cfg, err := parseConfig(map[string]any{"path": path, "variables": map[string]any{"note": "note"}})
	if err != nil {
		t.Fatal(err)
	}
	const workers, perWorker = 8, 50
	note := strings.Repeat(`a "quoted", long field `, 400)
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			if err := ensureHeader(cfg); err != nil {
				errs <- err
				return
			}
			for i := 0; i < perWorker; i++ {
				key := protocol.Key{IdentityKey: fmt.Sprintf("email:w%d-%d@example.com", w, i)}
				if err := appendRow(cfg, key, map[string]any{"note": note}); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != workers*perWorker+1 {
		t.Fatalf("got %d rows, want %d", len(rows), workers*perWorker+1)
	}
	if rows[0][0] != "identity_key" {
		t.Fatalf("first row is %q, want the header", rows[0][0])
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if row[1] != note {
			t.Fatalf("row %s has a damaged note", row[0])
		}
		seen[row[0]] = true
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("got %d distinct keys, want %d", len(seen), workers*perWorker)
	}
}
