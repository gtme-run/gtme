// docsgen writes the generated half of docs/ — the reference collection and
// the glossary — from `gtme help --agent`, docs/_adapters.json,
// spec/fields/*.json, spec/ledger.sql, and each concept page's `defines:`.
// `make docs-reference` builds the binary, captures the agent document from
// a clean home, and runs this. With -check it writes nothing and exits 1
// when the committed pages differ from what it would write.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gtme-run/gtme/internal/docsgen"
)

func main() {
	agent := flag.String("agent", "", "path to `gtme help --agent` output captured from a clean home (required)")
	root := flag.String("root", "", "repo root (default: the nearest parent with go.mod)")
	check := flag.Bool("check", false, "write nothing; exit 1 if docs/ differs from what would be written")
	flag.Parse()
	if *agent == "" {
		fmt.Fprintln(os.Stderr, "docsgen: -agent FILE is required; see `make docs-reference`")
		os.Exit(2)
	}
	if *root == "" {
		r, err := findRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "docsgen:", err)
			os.Exit(2)
		}
		*root = r
	}
	in, err := docsgen.Load(*root, *agent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docsgen:", err)
		os.Exit(1)
	}
	out, err := docsgen.Generate(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docsgen:", err)
		os.Exit(1)
	}
	docs := filepath.Join(*root, "docs")
	if *check {
		drift, err := docsgen.Drift(docs, out)
		if err != nil {
			fmt.Fprintln(os.Stderr, "docsgen:", err)
			os.Exit(1)
		}
		for _, d := range drift {
			fmt.Fprintln(os.Stderr, "docsgen:", d)
		}
		if len(drift) > 0 {
			fmt.Fprintln(os.Stderr, "docsgen: docs/ is out of date; run `make docs-reference` and commit the result")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "docsgen: %d generated pages in sync\n", len(out.Files))
		return
	}
	changed, err := docsgen.Write(docs, out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docsgen:", err)
		os.Exit(1)
	}
	for _, c := range changed {
		fmt.Fprintln(os.Stderr, "docsgen:", c)
	}
	fmt.Fprintf(os.Stderr, "docsgen: %d generated pages, %d changed\n", len(out.Files), len(changed))
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s; pass -root", dir)
		}
		dir = parent
	}
}
