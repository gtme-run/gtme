package ledger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestRunLockIsExclusiveAndProbed(t *testing.T) {
	ctx := context.Background()
	l, err := Open(ctx, filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if alive, err := l.RunAlive("01RUN"); err != nil || alive {
		t.Fatalf("a run never locked: alive=%v err=%v, want false", alive, err)
	}
	lock, err := l.LockRun("01RUN")
	if err != nil {
		t.Fatal(err)
	}
	if alive, err := l.RunAlive("01RUN"); err != nil || !alive {
		t.Fatalf("a held lock: alive=%v err=%v, want true", alive, err)
	}
	if _, err := l.LockRun("01RUN"); !errors.Is(err, ErrRunLocked) {
		t.Fatalf("a second lock: err=%v, want ErrRunLocked", err)
	}
	lock.Release()
	if alive, err := l.RunAlive("01RUN"); err != nil || alive {
		t.Fatalf("after release: alive=%v err=%v, want false", alive, err)
	}
	again, err := l.LockRun("01RUN")
	if err != nil {
		t.Fatalf("relocking after release: %v", err)
	}
	again.Release()
}
