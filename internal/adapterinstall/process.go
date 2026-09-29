package adapterinstall

// Process entries (SPEC §8, ADR-063): a prebuilt process adapter, one
// checksum-pinned .tar.gz per platform holding manifest.json and an
// executable run at its root. This file downloads, checks and unpacks one;
// the policy (verified only, what is printed, where it installs) lives in
// internal/cli.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ErrChecksum is a process archive whose hash is not the one the index
// published.
var ErrChecksum = errors.New("checksum mismatch")

// Platform is this machine's asset key, as §13 writes the targets.
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// Platforms lists the entry's asset keys, sorted.
func (e *Entry) Platforms() []string {
	out := make([]string, 0, len(e.Assets))
	for k := range e.Assets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// maxArchiveBytes bounds a process adapter archive: an executable and its
// manifest, not a distribution.
const maxArchiveBytes = 128 << 20

// processFiles are the only members a process archive may carry.
var processFiles = map[string]os.FileMode{"manifest.json": 0o644, "run": 0o755}

// FetchProcess downloads the asset, refuses a checksum mismatch, and unpacks
// manifest.json and run into a fresh temp dir. The caller removes it.
func FetchProcess(a Asset) (string, error) {
	resp, err := get(a.URL, "")
	if err != nil {
		return "", fmt.Errorf("adapters: fetching %s: %w", a.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("adapters: fetching %s: %s", a.URL, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("adapters: fetching %s: %w", a.URL, err)
	}
	if len(raw) > maxArchiveBytes {
		return "", fmt.Errorf("adapters: %s is larger than %d bytes — not a process adapter archive", a.URL, maxArchiveBytes)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
		return "", fmt.Errorf("adapters: %w for %s — the index lists %s, the download hashes to %s; the thing reviewed is not the thing fetched, refusing to install",
			ErrChecksum, a.URL, short(a.SHA256), short(got))
	}
	return unpackProcess(raw, a.URL)
}

func unpackProcess(raw []byte, name string) (string, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(raw)))
	if err != nil {
		return "", fmt.Errorf("adapters: %s: not a gzip tarball: %w", name, err)
	}
	dir, err := os.MkdirTemp("", "gtme-adapter-process-*")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	seen := map[string]bool{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("adapters: reading %s: %w", name, err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		member := strings.TrimPrefix(hdr.Name, "./")
		mode, allowed := processFiles[member]
		if !allowed || hdr.Typeflag != tar.TypeReg {
			return "", fmt.Errorf("adapters: %s: unexpected member %q — a process archive holds manifest.json and run only", name, hdr.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxArchiveBytes))
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, member), body, mode); err != nil {
			return "", err
		}
		seen[member] = true
	}
	for member := range processFiles {
		if !seen[member] {
			return "", fmt.Errorf("adapters: %s: no %s in the archive", name, member)
		}
	}
	ok = true
	return dir, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
