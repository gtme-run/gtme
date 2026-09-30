package adapters

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// An external adapter's fatal error is on its stderr; the exit status alone
// says only which class it was. The session's error carries the last line,
// so the failed step event in the ledger says why, not just "exit status 3"
// (#202).
func TestExecExitErrorCarriesTheLastStderrLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script adapter")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "run")
	script := "#!/bin/sh\n" +
		"echo 'starting up' >&2\n" +
		"echo 'vendor: plan limit reached (HTTP 403): Remaining uploads: 0' >&2\n" +
		"exit 3\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	s, err := launchExec(context.Background(), dir, exe, Ports{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	s.CloseSend()
	for {
		if _, err := s.Next(); err != nil {
			break
		}
	}
	err = s.Wait()
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want an ExitError", err)
	}
	if ee.ExitCode() != 3 {
		t.Errorf("exit code = %d, want 3", ee.ExitCode())
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit status 3") {
		t.Errorf("error %q lost the exit status", msg)
	}
	if !strings.Contains(msg, "Remaining uploads: 0") {
		t.Errorf("error %q does not carry the adapter's last stderr line", msg)
	}
	if strings.Contains(msg, "starting up") {
		t.Errorf("error %q carries more than the last stderr line", msg)
	}
	// The operator still sees everything the adapter said, as before.
	if !strings.Contains(log.String(), "starting up") {
		t.Errorf("stderr log lost a line: %q", log.String())
	}
}
