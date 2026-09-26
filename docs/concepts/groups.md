---
name: Groups and segments
description: A segment is a saved query that recomputes every time, and a group is a named set of identities that pipelines add to, gate on, and source from
for: "You want one pipeline to decide who's worth working and another to work them, or you need a contact rule that holds across every campaign, or want a saved list that updates itself."
learn:
  - "how a segment differs from a group, and when to use each"
  - "how a two-stage campaign hands records off through a group with limit and once"
  - "what a suppression group blocks that delivery idempotency lets through"
  - "why a group keeps its first answer when a segment recomputes"
order: 9
links:
  - to: /concepts/ledger
    type: relates-to
    description: Groups are the ledger's third layer, and a segment is a query over its views
  - to: /concepts/pipeline
    type: relates-to
    description: The rest of the pipeline file, which leaves the group keys to this page
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: Delivery idempotency stops the same delivery twice, and suppression is the rule layered above it
  - to: /concepts/gate-ladder
    type: relates-to
    description: A dry-run adds no group members and records no touches, so a rehearsal leaves every group as it was
  - to: /guides/multi-stage
    type: relates-to
    description: Builds a qualify-then-send campaign on real vendors, with a review between the stages
  - to: /guides/segment
    type: relates-to
    description: Saves a slice of the ledger as a segment and uses it to seed a pipeline
  - to: /reference/cli/groups
    type: relates-to
    description: Every gtme groups verb and flag
  - to: /reference/cli/query
    type: relates-to
    description: Every gtme query flag, including --save, --name, and --list
  - to: /reference/pipeline-yaml
    type: relates-to
    description: The group keys with their types and where each is valid
  - to: /decisions#adr-021
    type: decided-by
    description: Groups as a named association between identities and a context, and why they aren't segments
  - to: /decisions#adr-032
    type: decided-by
    description: The handoff between stages is a delivery, and a group source takes limit
  - to: /decisions#adr-052
    type: decided-by
    description: once on a group source skips members the pipeline already finished
  - to: /decisions#adr-037
    type: decided-by
    description: A segment can fill a config value, and when to read a group instead
---

# Groups and segments

Here's one campaign split into two pipelines. The first decides who qualifies:

```yaml
name: qualify
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: marketers
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'title' AND value LIKE '%Marketing%'

group: qualified
```

The second works the qualified people, one per run:

```yaml
name: send
version: 1

source:
  group: qualified
  limit: 1
  once: true

steps:
  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      name: full_name
    idempotency: email
    record: contacted
```

Save both next to the `contacts.csv` from [See it run](/start/show-me), which has three people: Jane, Bob, and Carol. Everything on this page spends $0 and sends nothing, because the only target is a local CSV file. Run the first against a scratch ledger:

```sh
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run qualify.yaml
```

The output is similar to the following:

```
...
step       adapter     in  out  empty  cached  filtered  failed  cost  avoided
source     csv/source  0   3    -      0       -         -       $0    -
marketers  sql/filter  3   2    -      0       1         -       $0    -
group "qualified": 2 record(s) added
total: $0 spent
```

Then run the second:

```sh
gtme run send.yaml
```

The output is similar to the following:

```
run 01M3DM6VQQEK7NECNRSZN6DCNT (send)
source: sourced 1 members of group "qualified" (2 of 2 not yet worked; limit 1, oldest first)
...
step    adapter          in  out  empty  cached  filtered  failed  cost  avoided
source  group:qualified  0   1    -      0       -         -       $0    -
out     csv/deliver      1   1    -      0       -         -       $0    -
total: $0 spent
```

Run `gtme run send.yaml` again and it writes Carol. A third run sources nobody:

```
source: sourced 0 members of group "qualified" (0 of 2 not yet worked; limit 1, oldest first)
```

Here's what the three runs left behind:

```sh
gtme groups
```

```
group      type    members  added  removed  touched  created
contacted  person  0        0      0        2        2026-09-26
qualified  person  2        2      0        0        2026-09-26
```

## What you just saw

**A group is a named set of identities, stored in the [ledger](/concepts/ledger).** The last line, `group: qualified`, adds every record that finishes the run. Bob's title failed the `marketers` filter, so the [receipt](/concepts/runs-and-receipts) line under the table says 2 were added.

**`source: {group: qualified}` makes the group a pipeline's source.** `limit: 1` serves at most one member per run, oldest-added first.

`once: true` skips members this pipeline already finished, meaning they completed its last step or a filter stopped them. A record that failed a step stays eligible and gets retried. Without `once:`, every run would serve Jane.

That `source:` block is the piece you'd hand your agent: a bounded read that advances through a set another pipeline already decided.

**`gtme groups` lists every group.** `members` is current membership. `added` and `removed` count membership events, and `touched` counts deliveries recorded under the group.

The `out` step's `record: contacted` appends a `touched` event per successful delivery, and the first one created `contacted`. A touch isn't membership, which is why `contacted` has 2 touches and 0 members. Without `record:`, touches go to a group named after the pipeline.

**`gtme groups show` says where a group sits in the chain.**

```
group qualified (person) — 2 member(s)
  person:carol@initech.dev
  person:jane.doe@acme.com
written by:  qualify
sourced by:  send
recent events:
  2026-09-26 01:10  added    carol@initech.dev  {"pipeline":"qualify"}
  2026-09-26 01:10  added    jane.doe@acme.com  {"pipeline":"qualify"}
```

The first lines are the members by [identity key](/concepts/identity-keys). `written by` and `sourced by` are the pipelines that added to the group and read from it.

Groups also gate steps. `require: [qualified]` and `exclude: [contacted]` work on any step after the source. An `ai/filter` step with `exclude:` set to the group it writes judges each person once.

## Put a review in the middle

**A `group/deliver` step lets you approve the handoff before anyone joins the group.** Replace the last line of `qualify.yaml`, `group: qualified`, with this step and save the file as `qualify-review.yaml`:

```yaml
  - id: handoff
    use: group/deliver
    with: { group: qualified }
    variables:
      name: full_name
      title: title
```

```sh
gtme run qualify-review.yaml --dry-run
```

The output is similar to the following:

```
...
handoff: 2 record(s) would be handed off to group "qualified" (held back — dry run)
handoff: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
  jane.doe@acme.com
    name: "Jane Doe"
    title: "VP Marketing"
  carol@initech.dev
    name: "Carol Reyes"
    title: "Marketing Operations Manager"
total: $0 spent
```

The armed run, without the flag, prints:

```
handoff: 2 record(s) handed off to group "qualified"
```

A [dry-run](/concepts/gate-ladder) holds the handoff and prints who would be added, and the armed run is the approval. A `group/deliver` and a network delivery never share a pipeline, or approving one would approve both.

To turn someone away after arming, remove them from the group with a reason:

```sh
gtme groups remove qualified carol@initech.dev --note "not a fit"
```

To keep them out before the handoff, narrow with a `sql/filter` step ahead of it.

On the other side, `limit: 50` with `once: true` works the group 50 a run. gtme has no scheduler, so cron or your agent runs `send.yaml` daily, as [Run on cron and events](/guides/cron-and-events) shows.

## Suppression groups

**A suppression group blocks a second contact on any target.** Delivery idempotency only stops a repeat to the same file or campaign. `contacted` already exists, because `send.yaml` recorded to it. Here's the step of a `followup.yaml` that sources `qualified` with no limit and writes `followup.csv`:

```yaml
  - id: nudge
    use: csv/deliver
    with:
      path: followup.csv
    variables:
      name: full_name
    idempotency: email
    suppress: { group: contacted, within: 30d }
```

```sh
gtme run followup.yaml
```

The output is similar to the following:

```
...
nudge: 2 record(s) suppressed:
  jane.doe@acme.com: touched in "contacted" 6s ago
  carol@initech.dev: touched in "contacted" 6s ago
total: $0 spent
```

Without the `suppress:` line, the same run writes both rows. A suppressed record gets a fail verdict at that step, the kind a filter writes and the ledger keeps. The record still moves on to later steps, because suppression withholds only the send. The receipt lists each one with the age of the touch that blocked it. `within:` is required, so every suppression rule has a window.

## Segments

**A segment is a saved read-only query, and it runs fresh every time.**

```sh
gtme query --save marketers "SELECT v.identity_id, i.identity_key \
    FROM current_values v JOIN identities i ON i.id = v.identity_id \
    WHERE v.field = 'title' AND v.value LIKE '%Marketing%'"
```

```
saved segment "marketers"
{"identity_id":"01M3DKR62VCD0KGYYKVJD8BRNT","identity_key":"jane.doe@acme.com"}
{"identity_id":"01M3DKR62W69R3SEB4T0ZJ1CTR","identity_key":"carol@initech.dev"}
2 rows
```

`gtme query --name marketers` runs it again. Import a new marketer tomorrow and the segment includes them, while `qualified` stays what the qualify run decided until a pipeline or a `gtme groups` edit changes it. `gtme groups add marketers-today --from-segment marketers` copies a segment's current members into a group.

That's it. That's how groups and segments work.

## Why it's this way

**An AI judge can answer differently on each run, and a group keeps its first answer.** The decision record, [ADR-021](/decisions#adr-021), made groups separate from segments so that re-judging someone takes a deliberate edit. `gtme plan` checks that every group a file reads from or gates on exists, and it can't check a SQL query the same way.

**The handoff is a write.** Nothing runs `send` when `qualify` finishes, so each runs on its own timetable ([ADR-032](/decisions#adr-032)).

**`once:` never removes members** ([ADR-052](/decisions#adr-052)). The group stays the reviewed set, and removing `once:` replays it. The cost is that a record that fails every time takes a `limit:` slot on every run until you fix it. Serve order is oldest-added only.

## Where it shows up

- [Multi-stage campaigns with groups](/guides/multi-stage) builds this split on real vendors, with a review in the middle.
- [Segment](/guides/segment) saves a segment and seeds a pipeline from it. [ADR-037](/decisions#adr-037) covers `{segment: marketers}` as a config value, a `with:` setting that reads its value from a saved query.
- [`gtme groups`](/reference/cli/groups), [`gtme query`](/reference/cli/query), and [pipeline.yaml keys](/reference/pipeline-yaml) are the lookup pages.
