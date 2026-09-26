// Package textsteps registers the runner-owned template renderer
// (SPEC §10 item 10, ADR-057): text/compose, a compose-role step with no
// model and no one behind it. It exists as a manifest — so `gtme plan`
// resolves it, `gtme help --agent` lists it, and the planner applies the
// participant grammar (uses:, provides:, of:, template:) exactly as it does
// for ai/* and human/* — but no protocol session is ever opened: the runner
// renders the template per record itself (internal/runner/text.go).
package textsteps

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/gtme-run/gtme/internal/adapters"
)

// ComposeID is the adapter id.
const ComposeID = "text/compose"

//go:embed text-compose.json
var composeManifest []byte

func init() {
	adapters.Register(composeManifest, func() adapters.Adapter { return neverOpened{} })
}

// neverOpened is the adapter behind the manifest: opening a session is a
// programming error, not a runtime condition.
type neverOpened struct{}

func (neverOpened) Run(ctx context.Context, p adapters.Ports) error {
	return fmt.Errorf("textsteps: text/compose is runner-owned and never opens a session")
}
