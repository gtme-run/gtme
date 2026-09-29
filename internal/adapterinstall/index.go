package adapterinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/gtme-run/gtme/spec"
)

// DefaultRegistry is the index URL baked into the binary (ADR-042);
// GTME_REGISTRY overrides it.
const DefaultRegistry = "https://raw.githubusercontent.com/gtme-run/gtme-bindings/main/index.json"

// RegistryURL is the index the verbs read.
func RegistryURL() string {
	if v := os.Getenv("GTME_REGISTRY"); v != "" {
		return v
	}
	return DefaultRegistry
}

// Entry is one index row (spec/schemas/registry-index.schema.json).
type Entry struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Vendor      string   `json:"vendor,omitempty"`
	Role        string   `json:"role"`
	EntityType  string   `json:"entity_type"`
	Needs       []string `json:"needs,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Credentials []string `json:"credentials,omitempty"`
	Source      struct {
		URL  string `json:"url"`
		Path string `json:"path"`
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
	} `json:"source"`
	SHA256 string `json:"sha256,omitempty"`
	Tier   string `json:"tier"`
	Since  string `json:"since,omitempty"`
	// Kind is "binding" (the default) or "process" (ADR-063).
	Kind string `json:"kind,omitempty"`
	// Release and Assets describe a process entry: the tag its archives were
	// published at, and one checksum-pinned archive per platform.
	Release string           `json:"release,omitempty"`
	Assets  map[string]Asset `json:"assets,omitempty"`
}

// Asset is one platform's archive of a process entry (ADR-063).
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// KindProcess marks a process entry (ADR-063).
const KindProcess = "process"

// IsProcess reports whether the entry is a prebuilt process adapter.
func (e *Entry) IsProcess() bool { return e.Kind == KindProcess }

// Index is the registry's published document.
type Index struct {
	Version     int     `json:"version"`
	GeneratedAt string  `json:"generated_at,omitempty"`
	Bindings    []Entry `json:"bindings"`
	// Skipped are the rows this binary could not read (#174): each failed
	// the row schema — an unknown role or kind, a missing member — and was
	// left out so the rest of the index still loads.
	Skipped []SkippedRow `json:"-"`
}

// SkippedRow names one index row LoadIndex left out, and why. URL and Path
// are the row's source when it had a readable one, so a content-hash check
// can tell that the row for its source was skipped.
type SkippedRow struct {
	ID        string
	URL, Path string
	Err       error
}

// indexSchema checks the document, rowSchema one row of it: a row the
// binary cannot read is skipped, never the whole index (#174), which is how
// a registry can list a new kind or role without breaking older clients.
var indexSchema, rowSchema = func() (*jsonschema.Schema, *jsonschema.Schema) {
	c := jsonschema.NewCompiler()
	if err := c.AddResource("registry-index.schema.json", strings.NewReader(string(spec.RegistryIndexSchema))); err != nil {
		panic(fmt.Sprintf("adapters: index schema resource: %v", err))
	}
	doc, err := c.Compile("registry-index.schema.json")
	if err != nil {
		panic(fmt.Sprintf("adapters: compiling registry-index.schema.json: %v", err))
	}
	row, err := c.Compile("registry-index.schema.json#/properties/bindings/items")
	if err != nil {
		panic(fmt.Sprintf("adapters: compiling registry-index.schema.json rows: %v", err))
	}
	return doc, row
}()

// LoadIndex fetches the registry index and validates it: the document
// whole, then each row on its own, skipping (into Skipped) a row that fails.
func LoadIndex() (*Index, error) {
	url := RegistryURL()
	resp, err := get(url, "application/json")
	if err != nil {
		return nil, fmt.Errorf("adapters: fetching registry index %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// GitHub answers an invalid bearer token with 404, not 401 — when the
		// token was actually sent, say so instead of impersonating a missing
		// index (#26).
		if resp.StatusCode == http.StatusNotFound && token() != "" && authorized(url) {
			return nil, fmt.Errorf("adapters: registry index %s: %s (a GITHUB_TOKEN is set, and GitHub answers an invalid token with 404, not 401 — try unsetting it)", url, resp.Status)
		}
		return nil, fmt.Errorf("adapters: registry index %s: %s", url, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("adapters: reading registry index: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("adapters: registry index %s is not JSON: %w", url, err)
	}
	rows, isList := doc["bindings"].([]any)
	if isList {
		// The rows are checked one by one below.
		doc["bindings"] = []any{}
	}
	if err := indexSchema.Validate(doc); err != nil {
		return nil, fmt.Errorf("adapters: registry index %s does not conform to registry-index.schema.json: %w", url, err)
	}
	// The document validated, so version is 1 and generated_at a string.
	ix := &Index{Version: 1}
	ix.GeneratedAt, _ = doc["generated_at"].(string)
	for i, r := range rows {
		body, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		var e Entry
		// A loose decode names the row in the warning even when it fails.
		lerr := json.Unmarshal(body, &e)
		if verr := rowSchema.Validate(r); verr != nil || lerr != nil {
			if verr == nil {
				verr = lerr
			}
			s := SkippedRow{ID: e.ID, URL: e.Source.URL, Path: e.Source.Path, Err: verr}
			if s.ID == "" {
				s.ID = fmt.Sprintf("row %d", i)
			}
			ix.Skipped = append(ix.Skipped, s)
			continue
		}
		ix.Bindings = append(ix.Bindings, e)
	}
	return ix, nil
}

// Reason is the row's first schema failure in one line: the member and what
// was wrong with it.
func (s SkippedRow) Reason() string {
	var ve *jsonschema.ValidationError
	if !errors.As(s.Err, &ve) {
		return s.Err.Error()
	}
	for len(ve.Causes) > 0 {
		ve = ve.Causes[0]
	}
	if ve.InstanceLocation == "" {
		return ve.Message
	}
	return strings.TrimPrefix(ve.InstanceLocation, "/") + ": " + ve.Message
}

// SkippedSource returns the skipped row that published the given repository
// path, if one did.
func (ix *Index) SkippedSource(url, path string) *SkippedRow {
	for i := range ix.Skipped {
		if ix.Skipped[i].URL == url && ix.Skipped[i].Path == path {
			return &ix.Skipped[i]
		}
	}
	return nil
}

// Search matches id, vendor, description and role, case-insensitively
// (SPEC §8).
func (ix *Index) Search(q string) []Entry {
	q = strings.ToLower(q)
	var out []Entry
	for _, e := range ix.Bindings {
		hay := strings.ToLower(e.ID + " " + e.Vendor + " " + e.Description + " " + e.Role)
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

// Find returns the entry with the given adapter id, if the index lists one.
func (ix *Index) Find(id string) *Entry {
	for i := range ix.Bindings {
		if ix.Bindings[i].ID == id {
			return &ix.Bindings[i]
		}
	}
	return nil
}

// FindSource returns the entry publishing the given repository path, if the
// index carries one — the hook for the content-hash refusal (SPEC §11 M19).
func (ix *Index) FindSource(url, path string) *Entry {
	for i := range ix.Bindings {
		if ix.Bindings[i].Source.URL == url && ix.Bindings[i].Source.Path == path {
			return &ix.Bindings[i]
		}
	}
	return nil
}
