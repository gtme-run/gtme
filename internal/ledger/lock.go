package ledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// The run lock (SPEC §8, ADR-061): a process executing a run holds an
// exclusive advisory lock on locks/<run_id>.lock beside the ledger file until
// it exits. The kernel drops the lock however the process dies, so a
// `running` run whose lock is free has no living process — that is how
// `gtme runs` tells a crash from a run in progress, with no clock and no
// daemon. The pid and host on the run are for display only.

// ErrRunLocked means another living process holds the run's lock.
var ErrRunLocked = errors.New("ledger: run is locked by a living process")

// RunLock is a held run lock. Release it when the run finishes; the process
// exiting releases it too.
type RunLock struct {
	f    *os.File
	path string
}

// Release drops the lock and removes its file, so locks/ holds files only
// for runs whose process died. The file goes before the lock does: a
// process that opened it in between finds, once it holds the lock, that the
// path no longer names what it locked, and starts over (LockRun).
func (k *RunLock) Release() {
	if k == nil || k.f == nil {
		return
	}
	os.Remove(k.path)
	syscall.Flock(int(k.f.Fd()), syscall.LOCK_UN)
	k.f.Close()
	k.f = nil
}

func (l *Ledger) lockPath(runID string) string {
	return filepath.Join(filepath.Dir(l.path), "locks", runID+".lock")
}

// LockRun takes the run's lock without waiting for it. ErrRunLocked means a
// living process holds it. A probe by `gtme runs` holds the lock for an
// instant, so a refusal is retried briefly before it is believed.
func (l *Ledger) LockRun(runID string) (*RunLock, error) {
	path := l.lockPath(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ledger: creating the run lock directory: %w", err)
	}
	for attempt := 0; ; attempt++ {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, fmt.Errorf("ledger: opening the run lock: %w", err)
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if samePath(f, path) {
				return &RunLock{f: f, path: path}, nil
			}
			// The holder released and removed the file while this process
			// waited on it: lock the file at the path now, not the old one.
			f.Close()
			continue
		}
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("ledger: taking the run lock: %w", err)
		}
		if attempt >= 4 {
			return nil, ErrRunLocked
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// samePath reports whether the open file is still the one at path.
func samePath(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	now, err := os.Stat(path)
	if err != nil {
		return false
	}
	return os.SameFile(held, now)
}

// RunAlive reports whether a living process holds the run's lock. It writes
// nothing: a missing lock file means no process ever locked the run on this
// machine (a run from before ADR-061, or one that died before locking), and
// the probe takes a shared lock and drops it at once.
func (l *Ledger) RunAlive(runID string) (bool, error) {
	f, err := os.Open(l.lockPath(runID))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ledger: opening the run lock: %w", err)
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("ledger: probing the run lock: %w", err)
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, nil
}
