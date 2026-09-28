package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

// The secrets file lives in the gtme home, ~/.gtme/secrets (SPEC §6), or
// $GTME_HOME/secrets — never beside a GTME_LEDGER set elsewhere, which the
// docs point at a working folder for scratch runs (#119).
func TestSecretsLiveInTheHomeNotBesideTheLedger(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GTME_HOME", "")
	t.Setenv("GTME_LEDGER", filepath.Join(work, "ledger.db"))
	t.Setenv("GTME_TEST_KEY", "")

	if err := Set("GTME_TEST_KEY", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "secrets")); err == nil {
		t.Fatal("secret set wrote a secrets file beside GTME_LEDGER")
	}
	if _, err := os.Stat(filepath.Join(home, ".gtme", "secrets")); err != nil {
		t.Fatalf("no ~/.gtme/secrets: %v", err)
	}
	if v, ok := Lookup("GTME_TEST_KEY"); !ok || v != "v1" {
		t.Errorf("Lookup = %q %v", v, ok)
	}

	gh := t.TempDir()
	t.Setenv("GTME_HOME", gh)
	if p, err := Path(); err != nil || p != filepath.Join(gh, "secrets") {
		t.Errorf("with GTME_HOME, Path = %q %v, want %s", p, err, filepath.Join(gh, "secrets"))
	}
}
