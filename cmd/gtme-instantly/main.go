// Command gtme-instantly is instantly/add-to-campaign as a process adapter
// (SPEC §5, §6, §10 item 6; ADR-063). It is the same adapter the binary used
// to carry, run out of process: NDJSON on stdin and stdout, diagnostics on
// stderr, and gtme's exit codes for its error classes. The release workflow
// ships it as the `run` beside the adapter's manifest.json, and `gtme adapters
// add instantly/add-to-campaign` installs that pair from the registry.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/internal/adapters/instantly"
	"github.com/gtme-run/gtme/internal/httpx"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		// The release workflow writes manifest.json from this, so the shipped
		// manifest is always the one compiled into the adapter.
		os.Stdout.Write(instantly.Manifest())
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	err := (&instantly.Adapter{}).Run(ctx, adapters.Ports{In: os.Stdin, Out: os.Stdout, Log: os.Stderr, Env: env})
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	if code := httpx.ExitCodeFor(err); code != 0 {
		os.Exit(code)
	}
	os.Exit(1)
}
