package e2e

// The M19 acceptance (SPEC §11): the `gtme adapters` verbs against a local
// tarball server and a local index — search finds by vendor; add installs
// under the dashed id with `.source.json` (resolved commit + content hash),
// runs fixtures first and prints hosts and credentials; failing or missing
// fixtures do not install; a content-hash mismatch against the index
// refuses; the installed binding resolves through `gtme plan` and serves its
// fixtures under `--simulate`; `update` moves the pin only when asked;
// `adapters` lists source and pin; a bundle records the pin.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtme-run/gtme/internal/adapterinstall"
)

const petsBindingYAML = `id: pets/list
version: 1
role: source
entity_type: person

credentials: [PETS_API_KEY]
auth:
  type: bearer
  env: PETS_API_KEY

config_schema:
  type: object
  additionalProperties: false
  properties:
    base_url:
      type: string
      default: "https://api.pets.example"
    limit:
      type: integer

request:
  method: GET
  url: "{{config.base_url}}/v1/owners"

extract:
  records: results
  fields:
    email: email
    full_name: name
`

const petsFixturesJSON = `{
  "config": {"limit": 5},
  "responses": [
    {"match": "GET /v1/owners", "status": 200,
     "body": {"results": [
       {"email": "ann@example.test", "name": "Ann Aardvark"},
       {"email": "bob@example.test", "name": "Bob Boa"}
     ]}}
  ]
}`

// badFixturesJSON is well-formed but yields no records: a source whose
// fixtures produce nothing fails verify.
const badFixturesJSON = `{
  "config": {},
  "responses": [
    {"match": "GET /v1/owners", "status": 200, "body": {"nothing": true}}
  ]
}`

const fakeSHA = "1111111111111111111111111111111111111111"
const fakeSHA2 = "2222222222222222222222222222222222222222"

// tarGz builds a repository tarball with one top-level directory.
func tarGz(t *testing.T, topdir string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: topdir + "/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// registryWorld is one local GitHub-shaped world: an API, a tarball host, an
// index, and a live vendor endpoint — everything the M19 acceptance needs.
type registryWorld struct {
	srv     *httptest.Server
	headSHA string
	repo    map[string]string // files in the repo at headSHA
	index   map[string]any
}

func newRegistryWorld(t *testing.T) *registryWorld {
	t.Helper()
	w := &registryWorld{headSHA: fakeSHA}
	w.repo = map[string]string{
		"pets-list/binding.yaml":              petsBindingYAML,
		"pets-list/fixtures/conformance.json": petsFixturesJSON,
		"bad-fix/binding.yaml":                strings.Replace(petsBindingYAML, "id: pets/list", "id: pets/bad", 1),
		"bad-fix/fixtures/conformance.json":   badFixturesJSON,
		"no-fix/binding.yaml":                 strings.Replace(petsBindingYAML, "id: pets/list", "id: pets/nofix", 1),
		"tampered/binding.yaml":               strings.Replace(petsBindingYAML, "id: pets/list", "id: pets/tampered", 1),
		"tampered/fixtures/conformance.json":  petsFixturesJSON,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/petco/bindings/commits/", func(rw http.ResponseWriter, req *http.Request) {
		// A full sha resolves to itself (a pinned ref); anything else to HEAD.
		if ref := strings.TrimPrefix(req.URL.Path, "/repos/petco/bindings/commits/"); len(ref) == 40 {
			json.NewEncoder(rw).Encode(map[string]string{"sha": ref})
			return
		}
		json.NewEncoder(rw).Encode(map[string]string{"sha": w.headSHA})
	})
	mux.HandleFunc("/petco/bindings/tar.gz/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(tarGz(t, "bindings-"+w.headSHA, w.repo))
	})
	mux.HandleFunc("/index.json", func(rw http.ResponseWriter, req *http.Request) {
		json.NewEncoder(rw).Encode(w.index)
	})
	// The vendor's "live" API, for the armed run the bundle test freezes.
	mux.HandleFunc("/v1/owners", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprint(rw, `{"results": [{"email": "ann@example.test", "name": "Ann Aardvark"}]}`)
	})
	w.srv = httptest.NewServer(mux)
	t.Cleanup(w.srv.Close)

	w.index = map[string]any{
		"version":  1,
		"bindings": []map[string]any{w.entry("pets/list", "pets-list", ""), w.entry("pets/tampered", "tampered", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")},
	}
	return w
}

// entry builds one index row; sha256 empty means "compute the honest hash".
func (w *registryWorld) entry(id, path, sha256 string) map[string]any {
	if sha256 == "" {
		sha256 = w.contentHash(path)
	}
	return map[string]any{
		"id": id, "description": "Owners of pets, from the pets API", "vendor": "pets",
		"role": "source", "entity_type": "person", "credentials": []string{"PETS_API_KEY"},
		"source": map[string]string{"url": "github.com/petco/bindings", "path": path, "ref": "main", "sha": w.headSHA},
		"sha256": sha256, "tier": "verified",
	}
}

// contentHash computes the rule the binary uses, over the repo's files.
func (w *registryWorld) contentHash(path string) string {
	dir, err := os.MkdirTemp("", "hash")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	for name, body := range w.repo {
		if !strings.HasPrefix(name, path+"/") {
			continue
		}
		rel := strings.TrimPrefix(name, path+"/")
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	h, err := adapterinstall.ContentHash(dir)
	if err != nil {
		panic(err)
	}
	return h
}

func (w *registryWorld) env() []string {
	return []string{
		"GTME_GITHUB_API=" + w.srv.URL,
		"GTME_GITHUB_CODELOAD=" + w.srv.URL,
		"GTME_REGISTRY=" + w.srv.URL + "/index.json",
	}
}

func TestAdaptersSearchFindsByVendor(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)
	res := h.runWithEnv(w.env(), "", "adapters", "search", "pets")
	if res.code != 0 {
		t.Fatalf("search exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "pets/list", "search output")
	contains(t, res.stderr, "gtme adapters add github.com/petco/bindings/pets-list@main", "search output names the install command")
}

func TestAdaptersAddInstallsVerifiedAndPinned(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/pets-list@main")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	// The reviewable surface printed before install: hosts and credentials.
	contains(t, res.stderr, "api.pets.example", "add output (host)")
	contains(t, res.stderr, "PETS_API_KEY", "add output (credential)")
	contains(t, res.stderr, "fixtures:    ok", "add output (fixtures ran)")

	dir := filepath.Join(h.home, ".gtme", "adapters", "pets-list")
	if _, err := os.Stat(filepath.Join(dir, "binding.yaml")); err != nil {
		t.Fatalf("binding.yaml not installed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, adapterinstall.SourceFile))
	if err != nil {
		t.Fatalf("no %s: %v", adapterinstall.SourceFile, err)
	}
	var src adapterinstall.Source
	if err := json.Unmarshal(raw, &src); err != nil {
		t.Fatal(err)
	}
	if src.Commit != fakeSHA {
		t.Errorf("commit = %q, want the resolved sha %q", src.Commit, fakeSHA)
	}
	wantHash, err := adapterinstall.ContentHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if src.SHA256 != wantHash {
		t.Errorf("sha256 = %q, want the content hash %q", src.SHA256, wantHash)
	}

	// The installed binding resolves through plan and simulates from fixtures.
	path := h.write("p.yaml", "name: pets\nsource:\n  use: pets/list\nsteps: []\n")
	plan := h.runWithEnv([]string{"PETS_API_KEY=x"}, "", "plan", path)
	if plan.code != 0 {
		t.Fatalf("plan exit = %d\n%s", plan.code, plan.stderr)
	}
	sim := h.runWithEnv([]string{"PETS_API_KEY=x"}, "", "run", path, "--simulate")
	if sim.code != 0 {
		t.Fatalf("simulate exit = %d\n%s", sim.code, sim.stderr)
	}

	// A second add refuses: update is the verb that moves a pin.
	again := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/pets-list@main")
	if again.code == 0 {
		t.Fatal("re-add should refuse; update moves the pin")
	}
	contains(t, again.stderr, "update", "re-add error")

	// The listing shows source and pin.
	list := h.runWithEnv(w.env(), "", "adapters")
	contains(t, list.stderr, "pets/list", "adapters list")
	contains(t, list.stderr, "github.com/petco/bindings/pets-list@main", "adapters list (source)")
	contains(t, list.stderr, fakeSHA[:12], "adapters list (pin)")
}

// TestAdaptersAddBareIDs is M33's acceptance (ADR-059): `adapters add`
// takes several references, a bare registry id resolves through the index
// to the entry's source pinned at the index's sha, each installs on its own,
// and one that fails does not undo the others but sets the exit code.
func TestAdaptersAddBareIDs(t *testing.T) {
	w := newRegistryWorld(t)
	var rows []map[string]any
	for id, dir := range map[string]string{"apollo/search": "apollo-search", "apollo/enrich": "apollo-enrich"} {
		for _, f := range []string{"binding.yaml", "fixtures/conformance.json"} {
			raw, err := os.ReadFile(filepath.Join(registryDir(), dir, filepath.FromSlash(f)))
			if err != nil {
				t.Fatal(err)
			}
			w.repo[dir+"/"+f] = string(raw)
		}
		e := w.entry(id, dir, "")
		e["source"].(map[string]string)["sha"] = fakeSHA2
		rows = append(rows, e)
	}
	w.index["bindings"] = rows
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "add", "apollo/search", "apollo/enrich")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	for _, dir := range []string{"apollo-search", "apollo-enrich"} {
		raw, err := os.ReadFile(filepath.Join(h.home, ".gtme", "adapters", dir, adapterinstall.SourceFile))
		if err != nil {
			t.Fatalf("%s: no %s: %v", dir, adapterinstall.SourceFile, err)
		}
		var src adapterinstall.Source
		if err := json.Unmarshal(raw, &src); err != nil {
			t.Fatal(err)
		}
		if src.Commit != fakeSHA2 || src.Ref != fakeSHA2 {
			t.Errorf("%s: pinned to commit %q ref %q, want the index sha %q", dir, src.Commit, src.Ref, fakeSHA2)
		}
	}

	// One unknown id among good ones: the good one installs, the exit code
	// reports the failure, and the error says where to look.
	h2 := newHarness(t)
	res = h2.runWithEnv(w.env(), "", "adapters", "add", "nope/missing", "apollo/search")
	if res.code == 0 {
		t.Fatalf("add with an unknown id exited 0\n%s", res.stderr)
	}
	contains(t, res.stderr, "nope/missing is not in the registry index", "add error")
	contains(t, res.stderr, "1 of 2 not installed", "add summary")
	if _, err := os.Stat(filepath.Join(h2.home, ".gtme", "adapters", "apollo-search", "binding.yaml")); err != nil {
		t.Errorf("apollo/search should still install beside the failure: %v", err)
	}
}

func TestAdaptersAddRefusesUnverifiable(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/bad-fix@main")
	if res.code == 0 {
		t.Fatal("failing fixtures must not install")
	}
	contains(t, res.stderr, "fixtures", "bad-fixtures error")
	if _, err := os.Stat(filepath.Join(h.home, ".gtme", "adapters", "pets-bad")); err == nil {
		t.Error("pets/bad was installed despite failing fixtures")
	}

	res = h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/no-fix@main")
	if res.code == 0 {
		t.Fatal("a binding without fixtures must not install")
	}
	contains(t, res.stderr, "fixtures are mandatory", "no-fixtures error")
	if _, err := os.Stat(filepath.Join(h.home, ".gtme", "adapters", "pets-nofix")); err == nil {
		t.Error("pets/nofix was installed despite shipping no fixtures")
	}
}

func TestAdaptersAddRefusesHashMismatch(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)
	res := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/tampered@main")
	if res.code == 0 {
		t.Fatal("a content-hash mismatch against the index must refuse")
	}
	contains(t, res.stderr, "hash mismatch", "mismatch error")
	if _, err := os.Stat(filepath.Join(h.home, ".gtme", "adapters", "pets-tampered")); err == nil {
		t.Error("pets/tampered was installed despite the index mismatch")
	}
}

func TestAdaptersUpdateMovesPinOnlyWhenAsked(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/pets-list@main")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}

	// The repository moves; nothing local changes until update is asked for.
	w.headSHA = fakeSHA2
	w.repo["pets-list/binding.yaml"] = strings.Replace(petsBindingYAML, "version: 1", "version: 2", 1)
	w.index["bindings"] = []map[string]any{w.entry("pets/list", "pets-list", "")}

	list := h.runWithEnv(w.env(), "", "adapters")
	contains(t, list.stderr, fakeSHA[:12], "pin before update")
	if strings.Contains(list.stderr, fakeSHA2[:12]) {
		t.Error("pin moved without update being asked")
	}

	up := h.runWithEnv(w.env(), "", "adapters", "update", "pets/list")
	if up.code != 0 {
		t.Fatalf("update exit = %d\n%s", up.code, up.stderr)
	}
	contains(t, up.stderr, fakeSHA2[:12], "update output")
	list = h.runWithEnv(w.env(), "", "adapters")
	contains(t, list.stderr, fakeSHA2[:12], "pin after update")
	raw, _ := os.ReadFile(filepath.Join(h.home, ".gtme", "adapters", "pets-list", "binding.yaml"))
	contains(t, string(raw), "version: 2", "updated binding content")
}

func TestBundleRecordsThePin(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "add", "github.com/petco/bindings/pets-list@main")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	// A real (sourcing-only) run against the local vendor endpoint, then a
	// bundle frozen from it: the bundle carries the binding AND its pin.
	path := h.write("p.yaml", "name: pets\nsource:\n  use: pets/list\n  with:\n    base_url: "+jsonString(w.srv.URL)+"\nsteps: []\n")
	run := h.runWithEnv([]string{"PETS_API_KEY=x"}, "", "run", path)
	if run.code != 0 {
		t.Fatalf("run exit = %d\n%s", run.code, run.stderr)
	}
	bundleDir := filepath.Join(h.work, "bundle")
	fr := h.runWithEnv(nil, "", "freeze", "last", "--bundle", bundleDir)
	if fr.code != 0 {
		t.Fatalf("freeze exit = %d\n%s", fr.code, fr.stderr)
	}
	pin := filepath.Join(bundleDir, "adapters", "pets-list", adapterinstall.SourceFile)
	raw, err := os.ReadFile(pin)
	if err != nil {
		t.Fatalf("bundle does not record the pin: %v", err)
	}
	contains(t, string(raw), fakeSHA, "bundled .source.json")
}

// TestAdaptersSearchWithStaleGitHubToken (#26): the registry index is public
// raw content — GITHUB_TOKEN must not be sent to it, because
// raw.githubusercontent.com answers an invalid bearer token with 404 and a
// stale token makes the whole registry look missing. And when a 404 does come
// back from a host the token WAS sent to, the error names the token as a
// suspect instead of impersonating a missing index.
func TestAdaptersSearchWithStaleGitHubToken(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)

	// A raw-content host with GitHub's observed behaviour: any Authorization
	// header — valid or not — turns the answer into 404. Deliberately NOT set
	// as GTME_GITHUB_API/CODELOAD; the token has no business here.
	rawSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			http.NotFound(rw, req)
			return
		}
		json.NewEncoder(rw).Encode(w.index)
	}))
	t.Cleanup(rawSrv.Close)

	env := []string{
		"GTME_GITHUB_API=" + w.srv.URL,
		"GTME_GITHUB_CODELOAD=" + w.srv.URL,
		"GTME_REGISTRY=" + rawSrv.URL + "/index.json",
		"GITHUB_TOKEN=ghp_stale_or_revoked",
	}
	res := h.runWithEnv(env, "", "adapters", "search", "pets")
	if res.code != 0 {
		t.Fatalf("search exit = %d with a stale GITHUB_TOKEN — the token leaked to the raw host\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "pets/list", "search output")

	// Same stale token, but the registry now lives on the API host, where the
	// token IS attached: the 404 must name the credential as a possible cause.
	authed := []string{
		"GTME_GITHUB_API=" + rawSrv.URL,
		"GTME_GITHUB_CODELOAD=" + rawSrv.URL,
		"GTME_REGISTRY=" + rawSrv.URL + "/index.json",
		"GITHUB_TOKEN=ghp_stale_or_revoked",
	}
	res = h.runWithEnv(authed, "", "adapters", "search", "pets")
	if res.code == 0 {
		t.Fatalf("search succeeded, want a 404 failure\n%s", res.stderr)
	}
	contains(t, res.stderr, "404", "stderr")
	contains(t, res.stderr, "GITHUB_TOKEN", "the 404 error suspects the token")
}

// TestVerifyAndPlanRefuseUnsupportedEntityType (#27): an entity_type with no
// SPEC §4 identity derivation is a static property of the manifest — verify
// must refuse to certify it and plan must refuse to run it, instead of the
// runner dropping 100% of the records after a source has been called and
// billed.
func TestVerifyAndPlanRefuseUnsupportedEntityType(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.home, ".gtme", "adapters", "example-thing-search")
	if err := os.MkdirAll(filepath.Join(dir, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("binding.yaml", `id: example/thing-search
version: 1
role: source
entity_type: widget
provides:
  type: object
  additionalProperties: false
  properties:
    example.name: { type: string }
config_schema:
  type: object
  additionalProperties: false
  properties:
    base_url: { type: string, default: "https://example.invalid" }
request:
  method: GET
  url: "{{config.base_url}}/things"
extract:
  records: things
  fields:
    example.name: name
`)
	writeFile("fixtures/conformance.json", `{ "config": {},
  "responses": [ { "match": "GET /things", "status": 200,
    "body": { "things": [ { "name": "one" }, { "name": "two" } ] } } ] }`)

	res := h.run("adapters", "verify", "example/thing-search")
	if res.code != 2 {
		t.Fatalf("verify exit = %d, want 2 for an underivable entity_type\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, `"widget"`, "verify refusal names the type")
	contains(t, res.stderr, "person", "verify refusal names the supported set")

	h.write("probe.yaml", `name: widget-probe
version: 1
source:
  use: example/thing-search
`)
	res = h.run("plan", "probe.yaml")
	if res.code != 2 {
		t.Fatalf("plan exit = %d, want 2 for an underivable entity_type\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, `"widget"`, "plan problem names the type")

	// The same gate holds when the entity type comes from config, not a
	// manifest: csv/source with an unsupported entity_type.
	h.write("things.csv", "name\none\n")
	h.write("csvprobe.yaml", `name: csv-widget-probe
version: 1
source:
  use: csv/source
  with:
    path: things.csv
    entity_type: widget
`)
	res = h.run("plan", "csvprobe.yaml")
	if res.code != 2 {
		t.Fatalf("csv plan exit = %d, want 2 for an underivable entity_type\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, `"widget"`, "csv plan problem names the type")
}

// TestAdaptersVerifyHandInstalledProcessAdapter (#168): a process adapter
// put on GTME_ADAPTER_PATH by hand, as manifest.json + run, is one that
// `gtme adapters` lists and `gtme adapters verify` checks, not "not
// installed".
func TestAdaptersVerifyHandInstalledProcessAdapter(t *testing.T) {
	h := newHarness(t)
	root := t.TempDir()
	dir := filepath.Join(root, "territory-owner")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id": "territory/owner", "version": 1, "role": "enrich", "entity_type": "person",
  "needs": {"type": "object", "required": ["company_domain"], "properties": {"company_domain": {"type": "string"}}},
  "provides": {"type": "object", "additionalProperties": false, "properties": {"territory.owner": {"type": "string"}}},
  "cost_estimate_usd": 0}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"GTME_ADAPTER_PATH=" + root}

	list := h.runWithEnv(env, "", "adapters")
	contains(t, list.stderr, "territory/owner", "adapters list")
	contains(t, list.stderr, "installed by hand", "adapters list")

	res := h.runWithEnv(env, "", "adapters", "verify", "territory/owner")
	if res.code != 0 {
		t.Fatalf("verify exit = %d, want 0 for a hand-installed process adapter\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "territory/owner v1 — enrich (person), process adapter", "verify output")
}

// TestAdaptersSkipAnUnreadableIndexRow (#174): a traverse row is listed, and
// a row this binary cannot read is skipped with a warning naming it, while
// search and a bare-id add still work for every other row.
func TestAdaptersSkipAnUnreadableIndexRow(t *testing.T) {
	w := newRegistryWorld(t)
	traverse := w.entry("pets/siblings", "pets-list", "")
	traverse["role"] = "traverse"
	future := w.entry("pets/future", "pets-list", "")
	future["kind"] = "wasm"
	w.index["bindings"] = []map[string]any{future, w.entry("pets/list", "pets-list", ""), traverse}
	h := newHarness(t)

	res := h.runWithEnv(w.env(), "", "adapters", "search", "pets")
	if res.code != 0 {
		t.Fatalf("search exit = %d, want 0 with one unreadable row\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "warning: registry index: skipped pets/future", "search warns about the row")
	contains(t, res.stderr, "pets/siblings", "search lists the traverse row")
	contains(t, res.stderr, "pets/list", "search lists the good row")

	res = h.runWithEnv(w.env(), "", "adapters", "add", "pets/list")
	if res.code != 0 {
		t.Fatalf("bare-id add exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "skipped pets/future", "add warns about the row")
	if _, err := os.Stat(filepath.Join(h.home, ".gtme", "adapters", "pets-list", "binding.yaml")); err != nil {
		t.Fatalf("pets/list not installed: %v", err)
	}
}

// TestAdaptersUpdateFollowsTheIndexForABareID (#175): a binding added by
// bare id is pinned at the index row's sha, and `update` moves it to the
// sha the index lists now, hash-checked, instead of re-resolving the old
// commit forever.
func TestAdaptersUpdateFollowsTheIndexForABareID(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)
	res := h.runWithEnv(w.env(), "", "adapters", "add", "pets/list")
	if res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	readSource := func() adapterinstall.Source {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(h.home, ".gtme", "adapters", "pets-list", adapterinstall.SourceFile))
		if err != nil {
			t.Fatal(err)
		}
		var src adapterinstall.Source
		if err := json.Unmarshal(raw, &src); err != nil {
			t.Fatal(err)
		}
		return src
	}
	if src := readSource(); src.Ref != fakeSHA || src.Commit != fakeSHA {
		t.Fatalf("bare-id add pinned ref %q commit %q, want the index sha %q", src.Ref, src.Commit, fakeSHA)
	}

	// The index has not moved: update leaves the pin where it is.
	res = h.runWithEnv(w.env(), "", "adapters", "update", "pets/list")
	if res.code != 0 {
		t.Fatalf("update exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, "pin unchanged", "update with an unchanged index")

	// The registry publishes a new commit: update follows the index's pin.
	w.headSHA = fakeSHA2
	w.repo["pets-list/binding.yaml"] = strings.Replace(petsBindingYAML, "version: 1", "version: 2", 1)
	w.index["bindings"] = []map[string]any{w.entry("pets/list", "pets-list", "")}
	res = h.runWithEnv(w.env(), "", "adapters", "update", "pets/list")
	if res.code != 0 {
		t.Fatalf("update exit = %d\n%s", res.code, res.stderr)
	}
	contains(t, res.stderr, fakeSHA[:12]+" → "+fakeSHA2[:12], "update moves the pin")
	if src := readSource(); src.Ref != fakeSHA2 || src.Commit != fakeSHA2 {
		t.Errorf("after update: ref %q commit %q, want the index's new sha %q", src.Ref, src.Commit, fakeSHA2)
	}
	raw, _ := os.ReadFile(filepath.Join(h.home, ".gtme", "adapters", "pets-list", "binding.yaml"))
	contains(t, string(raw), "version: 2", "updated binding content")

	// The new pin is still hash-checked against the index.
	w.headSHA = "3333333333333333333333333333333333333333"
	w.repo["pets-list/binding.yaml"] = strings.Replace(petsBindingYAML, "version: 1", "version: 3", 1)
	e := w.entry("pets/list", "pets-list", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	w.index["bindings"] = []map[string]any{e}
	res = h.runWithEnv(w.env(), "", "adapters", "update", "pets/list")
	if res.code == 0 {
		t.Fatalf("update past a content-hash mismatch exited 0\n%s", res.stderr)
	}
	contains(t, res.stderr, "hash mismatch", "update refuses a mismatch")
}

// TestAdaptersFailedAddLeavesNoTempDir (#184): every refusal after the fetch
// (failing fixtures, no fixtures, a content-hash mismatch, a failed update)
// removes the fetched directory instead of leaving it in TMPDIR.
func TestAdaptersFailedAddLeavesNoTempDir(t *testing.T) {
	w := newRegistryWorld(t)
	h := newHarness(t)
	tmp := t.TempDir()
	env := append(w.env(), "TMPDIR="+tmp)

	for _, ref := range []string{
		"github.com/petco/bindings/bad-fix@main",
		"github.com/petco/bindings/no-fix@main",
		"github.com/petco/bindings/tampered@main",
	} {
		if res := h.runWithEnv(env, "", "adapters", "add", ref); res.code == 0 {
			t.Fatalf("add %s should refuse\n%s", ref, res.stderr)
		}
	}
	// An update whose new commit fails verify cleans up too.
	if res := h.runWithEnv(env, "", "adapters", "add", "github.com/petco/bindings/pets-list@main"); res.code != 0 {
		t.Fatalf("add exit = %d\n%s", res.code, res.stderr)
	}
	w.headSHA = fakeSHA2
	w.repo["pets-list/fixtures/conformance.json"] = badFixturesJSON
	w.index["bindings"] = []map[string]any{}
	if res := h.runWithEnv(env, "", "adapters", "update", "pets/list"); res.code == 0 {
		t.Fatalf("update to a commit with failing fixtures should refuse\n%s", res.stderr)
	}

	left, err := filepath.Glob(filepath.Join(tmp, "gtme-adapter-fetch-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("refused add/update left %d fetched dir(s) in TMPDIR: %v", len(left), left)
	}
}
