package cli

// Registry process entries (SPEC §8 `gtme adapters`, ADR-063): a prebuilt
// process adapter installed from a checksum-pinned archive as manifest.json +
// run on the §6 discovery path. Verified tier only.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gtme-run/gtme/internal/adapterinstall"
	"github.com/gtme-run/gtme/internal/adapters"
)

// adaptersAddProcess installs a process entry for this platform.
func adaptersAddProcess(env Env, e *adapterinstall.Entry) error {
	tmp, asset, err := fetchProcess(env, e)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	root, err := installDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(root, strings.ReplaceAll(e.ID, "/", "-"))
	if _, err := os.Stat(dest); err == nil {
		return fail(ExitValidation, "adapters: %s is already installed at %s — `gtme adapters update %s` moves the pin", e.ID, dest, e.ID)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := swapInto(tmp, dest, processSource(e, asset)); err != nil {
		return err
	}
	fmt.Fprintf(env.Stderr, "installed %s at %s — pinned to release %s (%s)\n", e.ID, dest, e.Release, shortCommit(asset.SHA256))
	return nil
}

// adaptersUpdateProcess moves an installed process adapter to the release
// the index lists now. A process entry has no ref of its own to ask for.
func adaptersUpdateProcess(env Env, id, dir, newRef string) error {
	if newRef != "" {
		return fail(ExitValidation, "adapters: %s is a process entry; it updates to the release the registry index lists, not to a ref", id)
	}
	src, err := adapterinstall.ReadSource(dir)
	if err != nil {
		return err
	}
	if src == nil {
		return fail(ExitValidation, "adapters: %s was installed by hand (no %s) — nothing to update from", id, adapterinstall.SourceFile)
	}
	var ix *adapterinstall.Index
	e, err := findEntry(id, &ix)
	if err != nil {
		return err
	}
	if !e.IsProcess() {
		return fail(ExitValidation, "adapters: the registry now lists %s as a binding — remove %s and `gtme adapters add %s`", id, dir, id)
	}
	if a, ok := e.Assets[adapterinstall.Platform()]; ok && a.SHA256 == src.SHA256 {
		fmt.Fprintf(env.Stderr, "%s is already at release %s — pin unchanged\n", id, e.Release)
		return nil
	}
	tmp, asset, err := fetchProcess(env, e)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := swapInto(tmp, dir, processSource(e, asset)); err != nil {
		return err
	}
	fmt.Fprintf(env.Stderr, "updated %s — release %s → %s\n", id, src.Release, e.Release)
	return nil
}

// fetchProcess is the front half of add and update: the verified tier, this
// platform's asset, the checksum, and the manifest. Nothing installs
// unverified: for a process entry that is the pinned checksum, a manifest
// that validates and names the entry's id, and the tests its CI ran.
func fetchProcess(env Env, e *adapterinstall.Entry) (string, adapterinstall.Asset, error) {
	if e.Tier != "verified" {
		return "", adapterinstall.Asset{}, fail(ExitValidation,
			"adapters: %s is a %s process entry — only verified process entries install (ADR-063)", e.ID, e.Tier)
	}
	platform := adapterinstall.Platform()
	asset, ok := e.Assets[platform]
	if !ok {
		return "", adapterinstall.Asset{}, fail(ExitValidation,
			"adapters: %s has no build for %s — the registry publishes %s", e.ID, platform, strings.Join(e.Platforms(), ", "))
	}
	tmp, err := adapterinstall.FetchProcess(asset)
	if err != nil {
		if errors.Is(err, adapterinstall.ErrChecksum) || errors.Is(err, adapterinstall.ErrArchive) {
			return "", asset, fail(ExitValidation, "%v", err)
		}
		return "", asset, fail(ExitNetwork, "%v", err)
	}
	fmt.Fprintf(env.Stderr, "fetched %s release %s for %s\n", e.ID, e.Release, platform)
	fmt.Fprintf(env.Stderr, "  built from:  %s/%s@%s (%s)\n", e.Source.URL, e.Source.Path, refOrHead(e.Source.Ref), shortCommit(e.Source.SHA))
	if _, err := verifyProcessDir(env, tmp, e.ID); err != nil {
		os.RemoveAll(tmp)
		return "", asset, err
	}
	return tmp, asset, nil
}

// verifyProcessDir is `gtme adapters verify` for a process adapter: the
// manifest validates and names id, run is an executable, the adapter–type
// contract holds, and the reviewable surface is printed.
func verifyProcessDir(env Env, dir, id string) (*adapters.Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fail(ExitValidation, "adapters: %s: %v", id, err)
	}
	m, err := adapters.ParseManifest(raw)
	if err != nil {
		return nil, fail(ExitValidation, "adapters: %s: %v", id, err)
	}
	if m.ID != id {
		return nil, fail(ExitValidation, "adapters: the archive for %s carries a manifest for %q — refusing to install", id, m.ID)
	}
	if strings.HasPrefix(m.ID, "demo/") {
		return nil, fail(ExitValidation,
			"adapters: %s: the demo/ prefix is reserved for the binary's own synthetic adapters — refusing to install", m.ID)
	}
	info, err := os.Stat(filepath.Join(dir, "run"))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fail(ExitValidation, "adapters: %s: run is missing or not executable", id)
	}
	if err := checkTypeContract(m, dir); err != nil {
		return nil, fail(ExitValidation, "adapters: %s: %v", m.ID, err)
	}
	fmt.Fprintf(env.Stderr, "%s v%d — %s (%s), process adapter\n", m.ID, m.Version, m.Role, m.EntityType)
	creds := "none"
	if len(m.Credentials) > 0 {
		creds = strings.Join(m.Credentials, ", ")
	}
	if len(m.CredentialsOptional) > 0 {
		creds += " (optional: " + strings.Join(m.CredentialsOptional, ", ") + ")"
	}
	fmt.Fprintf(env.Stderr, "  demands:     %s\n", creds)
	fmt.Fprintf(env.Stderr, "  needs:       %s\n", schemaSummary(m.Needs))
	fmt.Fprintf(env.Stderr, "  provides:    %s\n", schemaSummary(m.Provides))
	var caps []string
	if m.Preflights {
		caps = append(caps, "preflights")
	}
	if m.Attests {
		caps = append(caps, "attests")
	}
	if m.IdempotencyScope != "" {
		caps = append(caps, "scope: "+m.IdempotencyScope)
	}
	if len(caps) > 0 {
		fmt.Fprintf(env.Stderr, "  declares:    %s\n", strings.Join(caps, ", "))
	}
	return m, nil
}

// swapInto installs src at dest with its .source.json, never leaving dest
// half-written: the tree is staged beside dest, an existing dest is moved
// aside, the stage is renamed into place, and a failed rename puts the old
// one back. A stage or old copy left by a crashed earlier attempt is cleared
// first.
func swapInto(src, dest string, s adapterinstall.Source) error {
	staging, old := dest+".staging", dest+".old"
	os.RemoveAll(staging)
	os.RemoveAll(old)
	if err := installTree(src, staging); err != nil {
		os.RemoveAll(staging)
		return err
	}
	if err := adapterinstall.WriteSource(staging, s); err != nil {
		os.RemoveAll(staging)
		return err
	}
	_, statErr := os.Stat(dest)
	existed := statErr == nil
	if existed {
		if err := os.Rename(dest, old); err != nil {
			os.RemoveAll(staging)
			return err
		}
	}
	if err := os.Rename(staging, dest); err != nil {
		if existed {
			os.Rename(old, dest)
		}
		os.RemoveAll(staging)
		return err
	}
	os.RemoveAll(old)
	return nil
}

func processSource(e *adapterinstall.Entry, a adapterinstall.Asset) adapterinstall.Source {
	return adapterinstall.Source{
		URL: e.Source.URL, Path: e.Source.Path, Ref: e.Source.Ref, Commit: e.Source.SHA,
		SHA256: a.SHA256, InstalledAt: time.Now().UTC().Format(time.RFC3339),
		Kind: adapterinstall.KindProcess, Asset: a.URL, Release: e.Release,
	}
}
