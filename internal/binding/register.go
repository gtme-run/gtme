package binding

import (
	"io/fs"
	"os"

	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/spec"
)

// Loader is the adapters.BindingLoader implementation: binding.yaml (plus the
// fixtures beside it) → manifest + engine factory. Wired up by
// internal/adapters/all.
func Loader(dir string, raw []byte) (*adapters.Manifest, func() adapters.Adapter, bool, error) {
	b, err := Parse(raw)
	if err != nil {
		return nil, nil, false, err
	}
	m, err := b.Manifest()
	if err != nil {
		return nil, nil, false, err
	}
	fixtures, err := LoadFixtures(os.DirFS(dir))
	if err != nil {
		return nil, nil, false, err
	}
	newFunc := func() adapters.Adapter { return &Engine{B: b, Fixtures: fixtures} }
	return m, newFunc, fixtures != nil, nil
}

// LoadFS loads a binding and its fixtures from an fs.FS rooted at the
// binding's directory (embedded or on disk).
func LoadFS(dir fs.FS) (*Binding, *FixtureSet, error) {
	raw, err := fs.ReadFile(dir, "binding.yaml")
	if err != nil {
		return nil, nil, err
	}
	b, err := Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	fixtures, err := LoadFixtures(dir)
	if err != nil {
		return nil, nil, err
	}
	return b, fixtures, nil
}

// Shipped lists the names of every binding embedded under spec/bindings/:
// since M33 (ADR-059) one, the worked example `help --bindings` prints,
// registered as no adapter — every vendor adapter is a registry entry.
func Shipped() []string {
	entries, err := fs.ReadDir(spec.Bindings, "bindings")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// ShippedFS returns the fs.FS rooted at one shipped binding's directory.
func ShippedFS(name string) (fs.FS, error) {
	return fs.Sub(spec.Bindings, "bindings/"+name)
}
