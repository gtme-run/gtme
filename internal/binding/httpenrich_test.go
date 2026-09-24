package binding

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/gtme-run/gtme/internal/adapters"
	"github.com/gtme-run/gtme/internal/protocol"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestHTTPEnrichInterruptStillEnds: a cancelled context is the operator's
// interrupt, never a per-record "nothing stored".
func TestHTTPEnrichInterruptStillEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &HTTPEnrich{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})}}
	inR, inW := io.Pipe()
	go func() {
		w := protocol.NewWriter(inW)
		w.Write(protocol.Message{Type: protocol.TypeOpen, StepID: "fetch", RunID: "run1", Config: map[string]any{
			"url": "http://{{record.company_domain}}/", "markdown": true, "field": "web.homepage", "freshness_days": float64(7),
		}})
		w.Write(protocol.Record(protocol.Key{EntityType: "company", IdentityKey: "live.example"}, map[string]any{"company_domain": "live.example"}, nil))
		w.Write(protocol.End())
		inW.Close()
	}()
	err := a.Run(ctx, adapters.Ports{In: inR, Out: io.Discard, Log: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled to propagate", err)
	}
}

// TestHTTPEnrichRejectsAMalformedScheme: a transport failure per record is
// "nothing stored", but a URL that can never be fetched is a config error,
// refused at OPEN rather than warned once per record.
func TestHTTPEnrichRejectsAMalformedScheme(t *testing.T) {
	base := map[string]any{"markdown": true, "field": "web.homepage", "freshness_days": float64(7)}
	for url, wantErr := range map[string]bool{
		"htp://{{record.company_domain}}/":     true,
		"ftp://{{record.company_domain}}/":     true,
		"http://{{record.company_domain}}/":    false,
		"HTTPS://api.example/{{record.email}}": false,
		"{{record.website}}":                   false, // the record carries the scheme
	} {
		cfg := map[string]any{"url": url}
		for k, v := range base {
			cfg[k] = v
		}
		_, err := parseHTTPEnrichConfig(cfg)
		if (err != nil) != wantErr {
			t.Errorf("url %q: err = %v, want error %v", url, err, wantErr)
		}
	}
}
