package adapterinstall

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type member struct {
	name string
	body string
	typ  byte
}

func archive(t *testing.T, members ...member) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{Name: m.name, Mode: 0o755, Size: int64(len(m.body)), Typeflag: typ, Linkname: "/etc/passwd"}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(m.body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// TestUnpackProcess: a process archive holds exactly manifest.json and an
// executable run (ADR-063); anything else is ErrArchive.
func TestUnpackProcess(t *testing.T) {
	good := archive(t, member{name: "manifest.json", body: "{}"}, member{name: "./run", body: "#!/bin/sh\n"})
	dir, err := unpackProcess(good, "good")
	if err != nil {
		t.Fatalf("good archive: %v", err)
	}
	defer os.RemoveAll(dir)
	if info, err := os.Stat(filepath.Join(dir, "run")); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("run not executable: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "manifest.json")); info.Mode()&0o111 != 0 {
		t.Error("manifest.json is executable")
	}

	for name, raw := range map[string][]byte{
		"extra member":  archive(t, member{name: "manifest.json", body: "{}"}, member{name: "run", body: "x"}, member{name: "../evil", body: "x"}),
		"symlink run":   archive(t, member{name: "manifest.json", body: "{}"}, member{name: "run", typ: tar.TypeSymlink}),
		"duplicate run": archive(t, member{name: "manifest.json", body: "{}"}, member{name: "run", body: "a"}, member{name: "run", body: "b"}),
		"missing run":   archive(t, member{name: "manifest.json", body: "{}"}),
		"not a tarball": []byte("plain text"),
	} {
		dir, err := unpackProcess(raw, name)
		if dir != "" {
			os.RemoveAll(dir)
		}
		if !errors.Is(err, ErrArchive) {
			t.Errorf("%s: err = %v, want ErrArchive", name, err)
		}
	}
}
