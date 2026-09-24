package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAPIEngineSendsWorkspaceHeader: an identity-linked Anthropic key needs
// anthropic-workspace-id on every request; the engine sends it when the
// optional credential is set and omits it otherwise.
func TestAPIEngineSendsWorkspaceHeader(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("anthropic-workspace-id"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	run := func(ws string) {
		getenv := func(k string) string {
			switch k {
			case "ANTHROPIC_API_KEY":
				return "test-key"
			case "GTME_ANTHROPIC_BASE_URL":
				return srv.URL
			case "ANTHROPIC_WORKSPACE_ID":
				return ws
			}
			return ""
		}
		e, err := newAPIEngine(getenv)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Complete(context.Background(), Request{Prompt: "hi", Model: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	run("wrkspc_123")
	run("")
	if len(got) != 2 || got[0] != "wrkspc_123" || got[1] != "" {
		t.Errorf("workspace headers seen = %q, want [wrkspc_123 \"\"]", got)
	}
}

// TestMaxTokensIsAStopReasonNotAnError: a reply cut off at max_tokens is a
// successful, billed call — Complete reports StopMaxTokens with the usage
// priced and no error, so an adapter can decide what to do with it.
func TestMaxTokensIsAStopReasonNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"[{\"identity_key\":"}],"stop_reason":"max_tokens","usage":{"input_tokens":1000,"output_tokens":50}}`))
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
	res, err := e.Complete(context.Background(), Request{Model: "claude-sonnet-4-6", Prompt: "x", MaxTokens: 50})
	if err != nil || res.StopReason != StopMaxTokens || res.InputTokens != 1000 || !res.Priced {
		t.Fatalf("res = %+v, err = %v; want StopMaxTokens, priced usage, no error", res, err)
	}
}
