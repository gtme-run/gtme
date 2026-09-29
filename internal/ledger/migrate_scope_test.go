package ledger

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrationBackfillsHTTPDeliverScope: migration 0015 (ADR-062) scopes
// http/deliver rows written before it on the url of their run's
// http/deliver step, leaves a run with two such steps unscoped, and
// backfills dispatched events by their own step.
func TestMigrationBackfillsHTTPDeliverScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db := l.DB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	one := `{"name":"one","steps":[{"id":"hook","use":"http/deliver","with":{"url":"https://a.example/hook"}}]}`
	two := `{"name":"two","steps":[{"id":"a","use":"http/deliver","with":{"url":"https://a.example/hook"}},` +
		`{"id":"b","use":"http/deliver","with":{"url":"https://b.example/hook"}}]}`
	exec(`INSERT INTO runs (id, pipeline, config_json, started_at, status) VALUES ('r1','one',?,'2026-01-01T00:00:00Z','done')`, one)
	exec(`INSERT INTO runs (id, pipeline, config_json, started_at, status) VALUES ('r2','two',?,'2026-01-01T00:00:00Z','done')`, two)
	exec(`INSERT INTO deliveries (id, identity_id, target, scope, idempotency, run_id, created_at) VALUES
	  ('d1','i1','http/deliver','','jane@acme.com','r1','2026-01-01T00:00:00Z'),
	  ('d2','i2','http/deliver','','bob@globex.io','r2','2026-01-01T00:00:00Z'),
	  ('d3','i3','csv/deliver','out.csv','carol@initech.dev','r1','2026-01-01T00:00:00Z')`)
	exec(`INSERT INTO step_events (id, run_id, step_id, identity_id, event, detail, created_at) VALUES
	  ('e1','r2','b','i4','dispatched','{"target":"http/deliver","scope":"","idempotency":"x@y.z"}','2026-01-01T00:00:00Z')`)
	exec(`DELETE FROM schema_migrations WHERE name = '0015_http_deliver_scope.sql'`)
	l.Close()

	l, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer l.Close()
	scope := func(id string) string {
		t.Helper()
		var s string
		if err := l.DB().QueryRow(`SELECT scope FROM deliveries WHERE id = ?`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := scope("d1"); got != "https://a.example/hook" {
		t.Errorf("one http/deliver step: scope = %q, want the step's url", got)
	}
	if got := scope("d2"); got != "" {
		t.Errorf("two http/deliver steps: scope = %q, want '' (ambiguous)", got)
	}
	if got := scope("d3"); got != "out.csv" {
		t.Errorf("csv/deliver row changed: scope = %q", got)
	}
	var ev string
	if err := l.DB().QueryRow(`SELECT json_extract(detail, '$.scope') FROM step_events WHERE id = 'e1'`).Scan(&ev); err != nil {
		t.Fatal(err)
	}
	if ev != "https://b.example/hook" {
		t.Errorf("dispatched event scope = %q, want its own step's url", ev)
	}
}
