// Package all imports every built-in adapter for its registration side effect.
// Anything that resolves adapters by id (the CLI) imports this package; the
// adapter packages themselves stay free of import cycles. It also wires the
// binding tier (SPEC §10a) into the adapter registry: the loader that lets
// installed binding.yaml directories resolve like any other adapter. No
// binding is built in since M33 (ADR-059): vendors are registry entries.
package all

import (
	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/internal/binding"

	_ "github.com/gtme-run/gtme/internal/adapters/aisteps"
	_ "github.com/gtme-run/gtme/internal/adapters/csvdeliver"
	_ "github.com/gtme-run/gtme/internal/adapters/csvsource"
	_ "github.com/gtme-run/gtme/internal/adapters/demoenrich"
	_ "github.com/gtme-run/gtme/internal/adapters/participants"
	_ "github.com/gtme-run/gtme/internal/adapters/textsteps"
)

func init() {
	adapters.BindingLoader = binding.Loader
}
