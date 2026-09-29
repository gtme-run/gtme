package ledger

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// Issue #153: processes that open a ledger file that does not exist yet
// race the migrations. Every concurrent first open must succeed, and the
// ledger must end with each migration recorded exactly once.

// TestConcurrentFirstOpenGoroutines opens one fresh path from many
// goroutines, each with its own handle (its own SQLite connection).
func TestConcurrentFirstOpenGoroutines(t *testing.T) {
	for round := 0; round < 5; round++ {
		path := filepath.Join(t.TempDir(), "ledger.db")
		const n = 8
		var wg sync.WaitGroup
		errs := make(chan error, n)
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				l, err := Open(context.Background(), path)
				if err != nil {
					errs <- err
					return
				}
				l.Close()
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: a concurrent first open failed: %v", round, err)
		}
		assertMigratedOnce(t, path)
	}
}

// TestConcurrentFirstOpenProcesses does the same from separate processes,
// the shape the issue reported (two `gtme run` on a new ledger).
func TestConcurrentFirstOpenProcesses(t *testing.T) {
	if p := os.Getenv("GTME_TEST_OPEN_LEDGER"); p != "" {
		l, err := Open(context.Background(), p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		l.Close()
		os.Exit(0)
	}
	for round := 0; round < 3; round++ {
		path := filepath.Join(t.TempDir(), "ledger.db")
		const n = 6
		cmds := make([]*exec.Cmd, n)
		outs := make([]*safeBuffer, n)
		for i := range cmds {
			cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentFirstOpenProcesses$")
			cmd.Env = append(os.Environ(), "GTME_TEST_OPEN_LEDGER="+path)
			outs[i] = &safeBuffer{}
			cmd.Stdout, cmd.Stderr = outs[i], outs[i]
			cmds[i] = cmd
		}
		for _, cmd := range cmds {
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
		}
		for i, cmd := range cmds {
			if err := cmd.Wait(); err != nil {
				t.Fatalf("round %d: process %d failed a first open: %v\n%s", round, i, err, outs[i].String())
			}
		}
		assertMigratedOnce(t, path)
	}
}

func assertMigratedOnce(t *testing.T, path string) {
	t.Helper()
	l, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if err := l.db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != len(names) {
		t.Fatalf("schema_migrations holds %d rows, want %d", got, len(names))
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
