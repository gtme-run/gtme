package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TestAPIEngineBatchPath drives the api engine's Submit/Collect against a
// stubbed Message Batches endpoint (ADR-038): one request per record keyed by
// custom_id, still-processing reported as not ready, results read from the
// JSONL stream in any order, half-price accounting, errored requests carried
// as errors.
func TestAPIEngineBatchPath(t *testing.T) {
	var submitted struct {
		Requests []struct {
			CustomID string `json:"custom_id"`
			Params   struct {
				Model  string `json:"model"`
				System []struct {
					Text string `json:"text"`
				} `json:"system"`
				Messages []struct {
					Content []struct {
						Text         string          `json:"text"`
						CacheControl json.RawMessage `json:"cache_control"`
					} `json:"content"`
				} `json:"messages"`
			} `json:"params"`
		} `json:"requests"`
	}
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/messages/batches":
			if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
				t.Errorf("submit body: %v", err)
			}
			w.Write([]byte(`{"id":"msgbatch_1","type":"message_batch","processing_status":"in_progress","request_counts":{"processing":2,"succeeded":0,"errored":0,"canceled":0,"expired":0},"created_at":"2026-08-29T00:00:00Z","expires_at":"2026-08-30T00:00:00Z","results_url":null}`))
		case r.Method == "GET" && r.URL.Path == "/v1/messages/batches/msgbatch_1":
			polls++
			status := "in_progress"
			if polls > 1 {
				status = "ended"
			}
			w.Write([]byte(`{"id":"msgbatch_1","type":"message_batch","processing_status":"` + status + `","request_counts":{"processing":0,"succeeded":1,"errored":1,"canceled":0,"expired":0},"created_at":"2026-08-29T00:00:00Z","expires_at":"2026-08-30T00:00:00Z","results_url":"` + "http://" + r.Host + `/v1/messages/batches/msgbatch_1/results"}`))
		case r.Method == "GET" && r.URL.Path == "/v1/messages/batches/msgbatch_1/results":
			w.Header().Set("Content-Type", "application/x-jsonl")
			// Out of request order, on purpose.
			w.Write([]byte(`{"custom_id":"b_x_com","result":{"type":"errored","error":{"type":"error","error":{"type":"invalid_request_error","message":"too long"}}}}` + "\n" +
				`{"custom_id":"a_x_com","result":{"type":"succeeded","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"[{\"identity_key\":\"a_x_com\",\"pass\":true}]"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1000,"output_tokens":100}}}}` + "\n"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	getenv := func(k string) string {
		switch k {
		case "ANTHROPIC_API_KEY":
			return "test-key"
		case "GTME_ANTHROPIC_BASE_URL":
			return srv.URL
		}
		return ""
	}
	e, err := newAPIEngine(getenv)
	if err != nil {
		t.Fatal(err)
	}
	be, ok := e.(BatchEngine)
	if !ok || !Deferrable(e) {
		t.Fatal("the api engine must be a deferrable BatchEngine")
	}

	token, err := be.Submit(context.Background(), []BatchRequest{
		{CustomID: "a_x_com", Request: Request{System: "sys", Shared: "Judge.", Payload: "Records (1):\n{}", Prompt: "Judge.\n\nRecords (1):\n{}", Model: "claude-sonnet-4-6"}},
		{CustomID: "b_x_com", Request: Request{System: "sys", Shared: "Judge.", Payload: "Records (1):\n{}", Prompt: "Judge.\n\nRecords (1):\n{}", Model: "claude-sonnet-4-6"}},
	})
	if err != nil || token != "msgbatch_1" {
		t.Fatalf("Submit = %q, %v", token, err)
	}
	if len(submitted.Requests) != 2 || submitted.Requests[0].CustomID != "a_x_com" || submitted.Requests[1].Params.Model != "claude-sonnet-4-6" {
		t.Errorf("submitted = %+v", submitted.Requests)
	}
	// The shared half carries the cache breakpoint in a batch too.
	blocks := submitted.Requests[0].Params.Messages[0].Content
	if len(blocks) != 2 || blocks[0].Text != "Judge." || len(blocks[0].CacheControl) == 0 || len(blocks[1].CacheControl) != 0 {
		t.Errorf("content blocks = %+v", blocks)
	}
	if submitted.Requests[0].Params.System[0].Text != "sys" {
		t.Errorf("system = %+v", submitted.Requests[0].Params.System)
	}

	// Still processing.
	results, ready, err := be.Collect(context.Background(), token)
	if err != nil || ready || results != nil {
		t.Fatalf("first Collect = %v, ready %v, %v — want not ready", results, ready, err)
	}
	// Ended: results keyed by custom_id, half price, the errored one carried.
	results, ready, err = be.Collect(context.Background(), token)
	if err != nil || !ready {
		t.Fatalf("second Collect: ready %v, %v", ready, err)
	}
	a := results["a_x_com"]
	if a.Err != nil || !strings.Contains(a.Response.Text, `"pass":true`) || a.Response.InputTokens != 1000 || a.Response.Model != "claude-sonnet-4-6" {
		t.Errorf("a = %+v", a)
	}
	full, priced := Price("claude-sonnet-4-6", 1000, 100, 0, 0)
	if !priced || !a.Response.Priced || a.Response.CostUSD != full/2 {
		t.Errorf("batch cost = %v, want half of %v", a.Response.CostUSD, full)
	}
	if b := results["b_x_com"]; b.Err == nil || !strings.Contains(b.Err.Error(), "too long") {
		t.Errorf("b = %+v, want the provider's error", b)
	}
}

// TestCustomIDIsDerivedAndChecked is #75: Anthropic constrains custom_id to
// ^[a-zA-Z0-9_-]{1,64}$ and an identity key never fits it. BatchID does, and
// Submit refuses a raw key before it is sent — which is what the stubbed
// batch test above was missing.
func TestCustomIDIsDerivedAndChecked(t *testing.T) {
	valid := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	for _, key := range []string{"company:reconstructinc.com", "person:name@example.com"} {
		if id := BatchID(key); !valid.MatchString(id) || id != BatchID(key) {
			t.Errorf("BatchID(%q) = %q, want a stable id matching the pattern", key, id)
		}
	}
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	getenv := func(k string) string {
		switch k {
		case "ANTHROPIC_API_KEY":
			return "test-key"
		case "GTME_ANTHROPIC_BASE_URL":
			return srv.URL
		}
		return ""
	}
	e, err := newAPIEngine(getenv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.(BatchEngine).Submit(context.Background(), []BatchRequest{{CustomID: "person:a@x.com", Request: Request{Model: "claude-sonnet-4-6"}}})
	if err == nil || !strings.Contains(err.Error(), "custom_id") || called {
		t.Fatalf("err = %v, called = %v; want a custom_id error before any request", err, called)
	}
}
