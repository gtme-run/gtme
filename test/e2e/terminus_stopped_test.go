package e2e

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// #203: a run that ends failed still adds the records that completed its
// final step to the terminus group (SPEC §8: every record that completes the
// run's final step is added), so the group and once: agree. Records the stop
// left unsent did not complete and do not join; the resume adds them.
func TestStoppedRunAddsItsCompletersToTheGroup(t *testing.T) {
	fake := &fillingInstantly{capacity: 5, emails: map[string]int{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	h := newHarness(t)
	var csv strings.Builder
	csv.WriteString("email,full_name\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&csv, "p%02d@example.com,Person %02d\n", i, i)
	}
	h.write("people.csv", csv.String())
	h.write("send.yaml", `name: send
source:
  use: csv/source
  with:
    path: people.csv
steps:
  - id: send
    use: instantly/add-to-campaign
    with:
      campaign: "`+fakeCampaignID+`"
      base_url: "`+srv.URL+`"
    variables:
      first_name: full_name
    idempotency: email
group: enrolled
`)
	env := []string{"INSTANTLY_API_KEY=test-key", "GTME_CONCURRENCY=1"}
	res := h.runWithEnv(env, "", "run", "send.yaml")
	if res.code == 0 {
		t.Fatalf("exit = 0, want the stop's non-zero exit\nstderr:\n%s", res.stderr)
	}
	if st := h.queryStrings(`SELECT status FROM runs`); len(st) != 1 || st[0] != "failed" {
		t.Fatalf("run status = %v, want failed", st)
	}
	members := `SELECT count(*) FROM group_members m JOIN groups g ON g.id = m.group_id WHERE g.name = 'enrolled'`
	if n := h.queryInt(members); n != 5 {
		t.Errorf("group members after the stopped run = %d, want the 5 that were sent\nstderr:\n%s", n, res.stderr)
	}
	contains(t, res.stderr, `group "enrolled": 5 record(s) added`, "the receipt says what joined")

	fake.mu.Lock()
	fake.capacity = 100
	fake.mu.Unlock()
	again := h.runWithEnv(env, "", "run", "send.yaml", "--resume", runIDOf(t, h))
	if again.code != 0 {
		t.Fatalf("resume exit = %d\nstderr:\n%s", again.code, again.stderr)
	}
	if n := h.queryInt(members); n != 10 {
		t.Errorf("group members after the resume = %d, want 10", n)
	}
	if n := h.queryInt(`SELECT count(*) FROM group_events WHERE event = 'added'`); n != 10 {
		t.Errorf("added events = %d, want 10: nobody is added twice", n)
	}
}
