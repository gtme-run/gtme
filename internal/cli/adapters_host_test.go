package cli

import (
	"encoding/json"
	"testing"

	"github.com/gtme-run/gtme/internal/binding"
)

// bindingHost fills a config default into the URL in either spelling of the
// reference, so verify prints the host a stranger's binding will call (#165).
func TestBindingHostAcceptsSpacedConfigReference(t *testing.T) {
	schema := json.RawMessage(`{"properties":{"base_url":{"default":"https://api.example.com"}}}`)
	for _, url := range []string{
		"{{config.base_url}}/v1/companies",
		"{{ config.base_url }}/v1/companies",
	} {
		b := &binding.Binding{ConfigSchema: schema}
		b.Request.URL = url
		if got := bindingHost(b); got != "api.example.com" {
			t.Errorf("bindingHost(%q) = %q, want api.example.com", url, got)
		}
	}
}
