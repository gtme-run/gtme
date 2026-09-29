package e2e

// Registry process entries (SPEC §8, ADR-063): instantly/add-to-campaign is a
// prebuilt process adapter the registry lists per platform. The suite serves
// an index and an archive built from cmd/gtme-instantly — never the network.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// processArchive packs the suite's built instantly adapter the way the
// release workflow does: manifest.json and run at the archive root.
func processArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, mode := range map[string]int64{"manifest.json": 0o644, "run": 0o755} {
		body, err := os.ReadFile(filepath.Join(processDir, "instantly-add-to-campaign", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write(body)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// processRegistry serves index.json listing instantly as a process entry for
// the given platforms, and the archive. sum overrides the published checksum.
func processRegistry(t *testing.T, platforms []string, sum string) *httptest.Server {
	t.Helper()
	archive := processArchive(t)
	if sum == "" {
		h := sha256.Sum256(archive)
		sum = hex.EncodeToString(h[:])
	}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	assets := map[string]any{}
	for _, p := range platforms {
		assets[p] = map[string]any{"url": srv.URL + "/gtme-instantly.tar.gz", "sha256": sum}
	}
	index, _ := json.Marshal(map[string]any{
		"version": 1,
		"bindings": []map[string]any{{
			"id": "instantly/add-to-campaign", "kind": "process", "description": "Add a person to an Instantly campaign",
			"vendor": "Instantly", "role": "deliver", "entity_type": "person", "credentials": []string{"INSTANTLY_API_KEY"},
			"source":  map[string]any{"url": "github.com/gtme-run/gtme", "path": "cmd/gtme-instantly", "ref": "v9.9.9", "sha": strings.Repeat("a", 40)},
			"tier":    "verified",
			"release": "v9.9.9",
			"assets":  assets,
		}},
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) { w.Write(index) })
	mux.HandleFunc("/gtme-instantly.tar.gz", func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	return srv
}

func TestAdaptersAddInstallsAProcessEntry(t *testing.T) {
	here := runtime.GOOS + "/" + runtime.GOARCH
	srv := processRegistry(t, []string{here}, "")
	h := newHarness(t)
	// Only what `add` installs: the suite's own copy is off the path.
	env := []string{"GTME_REGISTRY=" + srv.URL + "/index.json", "GTME_ADAPTER_PATH=" + t.TempDir()}

	res := h.runWithEnv(env, "", "adapters", "add", "instantly/add-to-campaign")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "instantly/add-to-campaign v2 — deliver (person), process adapter", "add prints the surface")
	contains(t, res.stderr, "declares:    preflights, attests, scope: campaign", "add prints the capabilities")
	contains(t, res.stderr, "pinned to release v9.9.9", "add names the release")
	dest := filepath.Join(h.home, ".gtme", "adapters", "instantly-add-to-campaign")
	info, err := os.Stat(filepath.Join(dest, "run"))
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("run not installed executable: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dest, ".source.json"))
	contains(t, string(raw), `"kind": "process"`, ".source.json")

	res = h.runWithEnv(env, "", "adapters", "verify", "instantly/add-to-campaign")
	if res.code != 0 {
		t.Fatalf("verify exit = %d\n%s", res.code, res.stderr)
	}
	res = h.runWithEnv(env, "", "adapters", "search", "instantly")
	contains(t, res.stderr, "gtme adapters add instantly/add-to-campaign", "search names the install command")
	res = h.runWithEnv(env, "", "adapters")
	contains(t, res.stderr, "release v9.9.9, built from github.com/gtme-run/gtme/cmd/gtme-instantly", "adapters lists the release")
	res = h.runWithEnv(env, "", "adapters", "update", "instantly/add-to-campaign")
	if res.code != 0 {
		t.Fatalf("update exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "already at release v9.9.9", "update with an unchanged index")

	// The installed adapter delivers, dedupes on the id, and names the campaign.
	fake := &fakeInstantly{name: "Q3 VP Marketing"}
	api := httptest.NewServer(fake)
	defer api.Close()
	h.write("contacts.csv", campaignZeroCSV)
	y := strings.Replace(outFloorYAML, "%s\n", "instantly/add-to-campaign\n", 1)
	h.write("p.yaml", strings.Replace(y, "%s", `      campaign: "`+fakeCampaignID+`"
      base_url: "`+api.URL+`"`, 1))
	run := append(env, "INSTANTLY_API_KEY=test-key")
	if res := h.runWithEnv(run, "", "run", "p.yaml"); res.code != 0 {
		t.Fatalf("run exit = %d\n%s", res.code, res.stderr)
	}
	res = h.runWithEnv(run, "", "run", "p.yaml")
	if res.code != 0 {
		t.Fatalf("second run exit = %d\n%s", res.code, res.stderr)
	}
	if fake.leads != 2 {
		t.Errorf("leads = %d, want 2", fake.leads)
	}
	contains(t, res.stderr, `campaign "Q3 VP Marketing" (`+fakeCampaignID+`)`, "preflight from the installed adapter")
}

func TestAdaptersAddRefusesAProcessChecksumMismatch(t *testing.T) {
	here := runtime.GOOS + "/" + runtime.GOARCH
	srv := processRegistry(t, []string{here}, strings.Repeat("0", 64))
	h := newHarness(t)
	res := h.runWithEnv([]string{"GTME_REGISTRY=" + srv.URL + "/index.json"}, "", "adapters", "add", "instantly/add-to-campaign")
	if res.code == 0 {
		t.Fatalf("add installed an archive whose checksum does not match\n%s", res.stderr)
	}
	contains(t, res.stderr, "checksum mismatch", "add refuses")
	if _, err := os.Stat(filepath.Join(h.home, ".gtme", "adapters", "instantly-add-to-campaign")); err == nil {
		t.Error("something was installed")
	}
}

func TestAdaptersAddRefusesAMissingPlatform(t *testing.T) {
	srv := processRegistry(t, []string{"linux/arm64"}, "")
	if runtime.GOOS+"/"+runtime.GOARCH == "linux/arm64" {
		srv = processRegistry(t, []string{"darwin/amd64"}, "")
	}
	h := newHarness(t)
	res := h.runWithEnv([]string{"GTME_REGISTRY=" + srv.URL + "/index.json"}, "", "adapters", "add", "instantly/add-to-campaign")
	if res.code == 0 {
		t.Fatalf("add installed with no build for this platform\n%s", res.stderr)
	}
	contains(t, res.stderr, "has no build for "+runtime.GOOS+"/"+runtime.GOARCH, "add names the platform")
}
