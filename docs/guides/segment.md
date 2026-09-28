---
name: Segment
description: Save a slice of the ledger as a named segment, re-run it as the ledger grows, and seed a pipeline from it through a group snapshot
for: "Your ledger holds several runs, and you want a slice of it, such as everyone who scored well but was never contacted, saved by name and worked by a new pipeline."
learn:
  - "how to save a query as a segment and run it again by name"
  - "how gtme query stops a statement that would change the ledger"
  - "how a segment seeds a pipeline through a group snapshot"
  - "why a segment's rows change between two reads and a group's members don't"
order: 10
roles: [operator]
links:
  - to: /concepts/groups
    type: depends-on
    description: What a segment and a group are, and why they're kept apart
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/ledger
    type: relates-to
    description: The current_values view and deliveries table the segment reads
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: What each receipt column counts, including a delivery skipped by idempotency
  - to: /concepts/pipeline
    type: relates-to
    description: What second-look-send.yaml is made of, a source and a list of steps
  - to: /start/show-me
    type: relates-to
    description: Runs hello.yaml, which is where this guide's history starts
  - to: /concepts/participants
    type: relates-to
    description: A person's review can sit between the snapshot and the send
  - to: /reference/cli/query
    type: relates-to
    description: Every gtme query flag, including --save, --name, --list, and --format
  - to: /reference/cli/groups
    type: relates-to
    description: Every gtme groups verb, including add --from-segment
  - to: /reference/ledger-schema/deliveries
    type: relates-to
    description: Every delivery row, with the target and scope a slice can narrow on
  - to: /reference/ledger-schema/current_values
    type: relates-to
    description: The view a slice reads each person's latest values from
  - to: /start/my-stack
    type: relates-to
    description: Swapping the CSV deliver step for a real sender
  - to: /spec#segment
    type: decided-by
    description: The Segment story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: gtme query runs read-only, and --save stores a named segment
  - to: /decisions#adr-021
    type: decided-by
    description: Why a group keeps its members when the segment it came from changes
  - to: /decisions#adr-037
    type: decided-by
    description: A segment can fill a with value directly, for a list that should drift
---

# Segment

**Goal: a slice of your [ledger](/concepts/ledger) saved by name, and a [pipeline](/concepts/pipeline) that works it.** You'll also see a live slice and a frozen list drift apart. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Segment story](/spec#segment):

> a slice of accumulated knowledge is a SQL statement away, and the same slice can seed a new pipeline.

## Before you start

**You need `gtme` ([Install](/start/install)) and a ledger with a few [runs](/concepts/runs-and-receipts) in it.** If segment and group are new words, read [Groups and segments](/concepts/groups) first. A segment lives in the ledger it was saved in, so next week set the same `GTME_LEDGER`, or run from the same folder. On your own ledger, start at step 1. Otherwise, build the practice history first, and the outputs will match this page.

**This guide needs no API keys and sends nothing past your own disk.** The only priced step is `demo/enrich`, a pretend $0.01 per record that calls no vendor. Every delivery writes a local CSV.

## Build a practice history

Run `hello.yaml` from [See it run](/start/show-me):

```sh
mkdir -p segment && cd segment
export GTME_LEDGER=./ledger.db
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/hello.yaml
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
gtme run hello.yaml
```

The receipt ends with `total: $0.0300 (estimated) spent`.

`hello.yaml` scores three people and writes anyone at 70 or more to `out.csv`. Jane scores 100 and goes out. Bob and Carol score 69. They're the slice this guide saves: people who scored 60 or more and were never delivered to.

## Steps

1. Find the fields and the delivery targets your slice can use. On your own ledger, run both before you write the query:

    ```sh
    gtme query --format table \
        "SELECT DISTINCT field FROM current_values ORDER BY field"
    ```

    It prints:

    ```
    field
    company_domain
    demo.note
    demo.score
    email
    full_name
    title
    6 rows
    ```

    ```sh
    gtme query --format table \
        "SELECT target, scope, count(*) AS n FROM deliveries
         GROUP BY target, scope"
    ```

    It prints:

    ```
    target       scope    n
    csv/deliver  out.csv  1
    1 rows
    ```

    [`current_values`](/reference/ledger-schema/current_values) holds each person's latest value for every field. [`deliveries`](/reference/ledger-schema/deliveries) holds every delivery any pipeline made, to any target: a CSV write and a group handoff count too, and a [dry-run](/concepts/gate-ladder) writes none. On a real ledger, "never emailed" usually means never delivered to one campaign, so narrow by its `scope`.

1. Save the slice as a segment:

    ```sh
    gtme query --save near-misses "SELECT v.identity_id, i.identity_key,
        v.value AS score
        FROM current_values v JOIN identities i ON i.id = v.identity_id
        WHERE v.field = 'demo.score' AND CAST(v.value AS INTEGER) >= 60
        AND v.identity_id NOT IN (SELECT identity_id FROM deliveries)"
    ```

    It prints:

    ```
    saved segment "near-misses"
    {"identity_id":"01M3MCRRZ4CTX3TRW74F1A432M","identity_key":"bob@globex.io","score":69}
    {"identity_id":"01M3MCRRZ5E617KRH4S6NGTVJY","identity_key":"carol@initech.dev","score":69}
    2 rows
    ```

    The query reads every run in the ledger at once. `CAST` turns the stored score into a number so `>= 60` compares numbers. To exclude only one campaign's deliveries, end the query with this, where `SCOPE` is a value from the previous step:

    ```sql
    AND v.identity_id NOT IN (SELECT identity_id FROM deliveries WHERE scope = 'SCOPE')
    ```

    `--save` stores the SQL in the ledger under the name, then runs it. Keep the `identity_id` column. Without it, `gtme groups add --from-segment` stops with `the query must yield an identity_id column`.

    Saving under a name that exists replaces its SQL. `--save` stores the query before running it, so a query that errors is saved anyway. Fix it and save again under the same name.

1. Run it again by name:

    ```sh
    gtme query --name near-misses --format table
    ```

    It prints:

    ```
    identity_id                 identity_key       score
    01M3MCRRZ4CTX3TRW74F1A432M  bob@globex.io      69
    01M3MCRRZ5E617KRH4S6NGTVJY  carol@initech.dev  69
    2 rows
    ```

    `--format table` is for reading. The default prints one JSON object per line, for a script or an agent. `gtme query --list` prints every saved segment with its SQL.

1. Check that a query can't change the ledger. Try a delete that starts like a read:

    ```sh
    gtme query "WITH d AS (SELECT id FROM deliveries)
        DELETE FROM deliveries WHERE id IN (SELECT id FROM d)"
    ```

    It prints:

    ```
    gtme: query failed: attempt to write a readonly database (8)
    ```

    A plain `DELETE` stops earlier, at `"delete" is not a SELECT`. This one gets past that check and fails anyway, because `gtme query` opens the ledger read-only. Either way the exit code is `2` and the ledger is unchanged.

1. Snapshot the segment into a group:

    ```sh
    gtme groups add second-look --from-segment near-misses
    ```

    It prints:

    ```
    group second-look: 2 added, 0 unchanged
    ```

    `second-look` now holds Bob and Carol, whatever the segment returns later.

1. Seed a pipeline from the group. Save this as `second-look-send.yaml`:

    ```yaml
    name: second-look-send
    version: 1

    source:
      group: second-look

    steps:
      - id: out
        use: csv/deliver
        with:
          path: second-look.csv
        variables:
          name: full_name
          title: title
          score: demo.score
        idempotency: email
    ```

    `source: {group: second-look}` makes the group's members the pipeline's records. The `out` step writes them to `second-look.csv`, and `idempotency: email` means nobody is written to that file twice. For a real campaign, swap `csv/deliver` for your sender's [deliver](/concepts/steps-and-roles) step and dry-run it first, as in [Your stack](/start/my-stack). Run it:

    ```sh
    gtme run second-look-send.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3MCRS4X9V9ECMD4M5JSZQYV (second-look-send)
    source: sourced 2 members of group "second-look"
    ...
    step    adapter            in  out  empty  cached  filtered  failed  cost  avoided
    source  group:second-look  0   2    -      0       -         -       $0    -
    out     csv/deliver        2   2    -      0       -         -       $0    -
    total: $0 spent
    ```

1. Grow the ledger. Add two people to `contacts.csv` and run `hello.yaml` again:

    ```sh
    cat >> contacts.csv <<'EOF'
    Dana Park,dana@contoso.com,Head of Growth,contoso.com
    Ivan Petrov,ivan@cyberdyne.ai,Demand Gen Manager,cyberdyne.ai
    EOF
    gtme run hello.yaml
    ```

    The receipt ends:

    ```
    total: $0.0200 (estimated) spent, $0.0300 avoided via cache (4 records skipped)
    ```

    The run paid to score Dana and Ivan only. The 4 skipped are the three scores already in the ledger and Jane's delivery to `out.csv`. Dana cleared 70 and went to `out.csv`. Ivan didn't.

1. Read the segment and the group again:

    ```sh
    gtme query --name near-misses --format table
    ```

    It prints:

    ```
    identity_id                 identity_key       score
    01M3MCRS5SY594CGD99MM0PFY5  ivan@cyberdyne.ai  65
    1 rows
    ```

    ```sh
    gtme groups show second-look
    ```

    The output is similar to the following:

    ```
    group second-look (person) — 2 member(s)
      person:bob@globex.io
      person:carol@initech.dev
    written by:  (none)
    sourced by:  second-look-send
    recent events:
      2026-09-28 16:15  added    carol@initech.dev  {"evaluated_at":"2026-09-28T16:15:04.324Z","segment":"near-misses"}
      2026-09-28 16:15  added    bob@globex.io  {"evaluated_at":"2026-09-28T16:15:04.324Z","segment":"near-misses"}
    ```

    The segment ran its SQL against today's ledger. Bob and Carol left it because `second-look-send` delivered to them, Dana never entered it because `hello` delivered to her, and Ivan came in. The group still holds Bob and Carol, the two the segment returned when you snapshotted it. Each `added` event records the segment and when it was evaluated. `(person)` is the group's [type](/concepts/types-and-traverse).

1. Top up the group from the segment, and run the pipeline again:

    ```sh
    gtme groups add second-look --from-segment near-misses
    gtme run second-look-send.yaml
    ```

    The output is similar to the following:

    ```
    group second-look: 1 added, 0 unchanged
    ...
    step    adapter            in  out  empty  cached  filtered  failed  cost  avoided
    source  group:second-look  0   3    -      0       -         -       $0    -
    out     csv/deliver        3   1    -      2       -         -       $0    $0.0000
    total: $0 spent, $0.0000 avoided via cache (2 records skipped)
    ```

    The source served all three members. At `out`, Bob and Carol were already delivered, so they count as cached, and only Ivan was written. For a group you top up every week, `once: true` on the source serves only members this pipeline hasn't finished (the decision record, [ADR-052](/decisions#adr-052)).

## What you have now

**A segment, `near-misses`, that recomputes on every read, and a group, `second-look`, that changes only when you add to it.** `second-look.csv` holds everyone the pipeline worked:

```
identity_key,name,score,title
bob@globex.io,Bob Stone,69,Head of Growth
carol@initech.dev,Carol Reyes,69,Marketing Operations Manager
ivan@cyberdyne.ai,Ivan Petrov,65,Demand Gen Manager
```

Check both with `gtme query --list` and `gtme groups show second-look`.

**We recommend the snapshot when someone should see the list before it's worked.** The list can't move between your check and the run (the decision record, [ADR-021](/decisions#adr-021)). If the slice is wrong, fix the SQL and save it again under the same name, and check the output before you snapshot. If the group already holds someone it shouldn't, `gtme groups remove second-look KEY --note TEXT` takes them out.

The alternative reads the segment live. A vendor source's `with:` value can be `{segment: NAME}`, which plan resolves and prints before anything runs ([ADR-037](/decisions#adr-037)). Use it for a list that should drift, such as this week's domains to search. It needs a vendor key, so this guide doesn't run it.

When you want to work the slice again, paste this line into Claude Code:

```text
Using the same GTME_LEDGER as before, run gtme query --name near-misses and show me the rows. After I approve them, and after you remove anyone I reject with gtme groups remove second-look KEY --note, run gtme groups add second-look --from-segment near-misses, then gtme run second-look-send.yaml, and show me the receipt.
```

## Next

- [`gtme query`](/reference/cli/query) lists every flag, including `--format csv` for handing a slice to a spreadsheet.
- [`gtme groups`](/reference/cli/groups) covers `remove` with `--note`, for taking someone out of a group after you've looked at it.
- [Participants](/concepts/participants) shows how a person can approve each record between the snapshot and the send.
