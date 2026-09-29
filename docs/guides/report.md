---
name: Report
description: Answer what a past run did and what it cost, and what a month of runs spent, from the ledger alone with gtme runs and one query each
for: "Your pipelines ran days ago, someone asks what one run did or what this month cost, and all you have is the ledger."
learn:
  - "how to find a past run and print its receipt again"
  - "how to read the ledger's receipt, and how it maps to the one the run printed"
  - "how to check a receipt against the step_events and costs rows it came from"
  - "how to total spend across runs, split into measured and estimated dollars"
order: 5
roles: [operator]
links:
  - to: /concepts/runs-and-receipts
    type: depends-on
    description: What each column of the live receipt counts, which this guide reconstructs from the ledger
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/ledger
    type: relates-to
    description: The runs layer holds step_events and costs, the two tables every receipt is summed from
  - to: /start/show-me
    type: relates-to
    description: Runs the same hello.yaml twice, which is where this guide's history starts
  - to: /concepts/facts
    type: relates-to
    description: Why the second run skips a score the ledger already holds inside its cache window
  - to: /concepts/participants
    type: relates-to
    description: The human step in openers.yaml, and how gtme answer records what a person did
  - to: /reference/cli/runs
    type: relates-to
    description: Every form of the command this guide reads receipts with
  - to: /reference/cli/query
    type: relates-to
    description: Read-only SQL against the ledger, used for the check and the monthly total
  - to: /reference/cli/answer
    type: relates-to
    description: The --cost and --measured flags that record a participant's cost
  - to: /reference/ledger-schema/costs
    type: relates-to
    description: One row per identity per step that spent, with amount_usd and basis
  - to: /reference/ledger-schema/step_events
    type: relates-to
    description: One row per record per step event, which the receipt's counts are
  - to: /reference/cli/freeze
    type: relates-to
    description: Prints the pipeline a run used, including the rates behind its cost avoided
  - to: /concepts/pipeline
    type: relates-to
    description: What the practice history's second file is made of
  - to: /spec#report
    type: decided-by
    description: The Report story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: gtme runs lists runs and prints one run's receipt, and totals carry their basis
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The runs, step_events, and costs tables the queries read
  - to: /decisions#adr-046
    type: decided-by
    description: Every cost row records whether its dollars were measured or estimated
---

# Report

**Goal: what any past run did and cost, read from the [ledger](/concepts/ledger) alone.** You'll also total a week or a month of runs. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Report story](/spec#report):

> what happened in a run, and what it cost, is always reconstructable after the fact.

## Before you start

**You need `gtme` ([Install](/start/install)) and a ledger with some [runs](/concepts/runs-and-receipts) in it.** On your own ledger, go straight to the steps. Otherwise, build the practice history in the next section first, and the outputs will match this page.

**This guide needs no API keys and sends nothing past your own disk.** `demo/enrich` charges a pretend $0.01 per record and calls no vendor. The $0.50 in the next section is a number you enter, and nobody is paid.

## Build a practice history

Run `hello.yaml` from [See it run](/start/show-me) twice, add two people to its CSV, and run it a third time:

```sh
mkdir -p report && cd report
export GTME_LEDGER=./ledger.db
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/hello.yaml
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
gtme run hello.yaml
gtme run hello.yaml
cat >> contacts.csv <<'EOF'
Dana Park,dana@contoso.com,Head of Growth,contoso.com
Eli Brooks,eli@northwind.io,VP Sales,northwind.io
EOF
gtme run hello.yaml
```

The three runs end with these total lines:

```
total: $0.0300 (estimated) spent
...
total: $0 spent, $0.0300 avoided via cache (3 records skipped)
...
total: $0.0200 (estimated) spent, $0.0300 avoided via cache (3 records skipped)
```

The second run found every score still fresh in the ledger and paid for none. `3 records skipped` is the three cached scores. Jane's delivery, already made, prints as `out: 1 already delivered` and isn't counted as a cache skip. The third run paid for the two new people only.

To put a real bill in the ledger too, save a second [pipeline](/concepts/pipeline) as `openers.yaml`. A copywriter writes an opener by hand for anyone scoring 90 or more, and bills $0.50 each:

```yaml
name: openers
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: score
    use: demo/enrich
    with:
      cost_per_record_usd: 0.01

  - id: top
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 90

  - id: opener
    use: human/compose
    uses: [full_name, title]
    provides: [opener]
    with:
      prompt: never
```

`prompt: never` means gtme waits for an answer instead of asking in the terminal. Run it, record the copywriter's answer and bill with `gtme answer` ([Participants](/concepts/participants)), and run it again:

```sh
gtme run openers.yaml
gtme answer openers opener jane.doe@acme.com \
    --set opener="Congrats on the Q3 launch." \
    --as copywriter --cost 0.50 --measured
gtme run openers.yaml
```

The first run stops at `opener` with Jane waiting, since she's the only one `top` keeps. The second run collects her answer and finishes the same run, so `openers` counts as one run:

```
resuming run 01M3K4RSXBGSQFV2HW8AQYX6QA (openers)
...
opener  human/compose  1   1    -      0       -         -       $0.5000  -
total: $0.5000 spent
```

`--measured` marks the $0.50 as a real bill, so the total prints with no `(estimated)` (the decision record on cost basis, [ADR-046](/decisions#adr-046)).

## Steps

1. List the runs:

    ```sh
    gtme runs
    ```

    It prints output similar to the following:

    ```
    run                         pipeline  status  started                   records  in flight
    01M3K4RSXBGSQFV2HW8AQYX6QA  openers   done    2026-09-28T04:36:02.091Z  5        -
    01M3K4RSW88DCY1YHGHE9QJ1F1  hello     done    2026-09-28T04:36:02.056Z  5        -
    01M3K4RSVACWB04NTNW6PP9CDP  hello     done    2026-09-28T04:36:02.026Z  3        -
    01M3K4RSTADKBKQVXJ76WG7575  hello     done    2026-09-28T04:36:01.994Z  3        -
    ```

    Runs are newest first, and `started` is in UTC. `records` counts the people each run touched. `in flight` counts records still waiting on a person or a vendor, and such a run's status is `pending`. A [dry-run](/concepts/gate-ladder) shows as `done (dry)`, and what it spent is real. A run whose process died shows as `interrupted`.

1. Print one run's receipt from the ledger. This is the third `hello` run:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the `run` column of the row you want, here `01M3K4RSW88DCY1YHGHE9QJ1F1`, or use `last` for the newest. It prints:

    ```
    run 01M3K4RSW88DCY1YHGHE9QJ1F1
    pipeline: hello
    status:   done
    started:  2026-09-28T04:36:02.056Z
    finished: 2026-09-28T04:36:02.064Z

    step    claimed  done  cached  failed  cost
    source  -        1     -       -       $0
    score   2        2     3       -       $0.0200
    keep    -        5     -       -       $0
    out     1        1     -       -       $0
    out: 1 already delivered
    total: $0.0200 (estimated)
    records: 5 (score=3 out=2), 3 with a fail verdict (filtered, or a send withheld)
    config:  3 steps recorded (`gtme freeze 01M3K4RSW88DCY1YHGHE9QJ1F1` rebuilds the pipeline)
    ```

    **This receipt counts ledger events, so its columns differ from the table the run printed.** `claimed` counts records a step sent to its [adapter](/concepts/adapter-tiers), `done` counts records it finished, and `cached` counts records it reused from the ledger. This table shows where to read each answer:

    | Question | Where |
    |---|---|
    | How many people did it source? | The `records:` line's first number, 5 |
    | Where did each person stop? | The counts after it: 2 reached `out`, 3 stopped after `score` |
    | How many did a [filter](/concepts/steps-and-roles) drop? | `with a fail verdict`, 3 |
    | How many did a step pay for, and how many did it reuse? | `claimed` and `cached`: `score` paid for 2 and reused 3 |
    | What did it cost? | The `cost` column and the `total:` line |

    Here that's 5 sourced, 3 filtered, 2 new scores paid for, and Dana delivered, with Jane skipped as already delivered. For a resumed run, this receipt totals every part of it, so it's the one to quote.

    **What the cache saved isn't in the ledger.** The run's own receipt showed `$0.0300 avoided`, which is `cached` times the step's rate. [`gtme freeze RUN_ID`](/reference/cli/freeze) prints the pipeline the run used, rate included: 3 × `cost_per_record_usd: 0.01` is that $0.0300.

1. To prove a receipt, rebuild its step table from `step_events` and `costs`, the ledger tables it's added up from:

    ```sh
    gtme query "SELECT step_id,
                  sum(event = 'claimed') AS claimed,
                  sum(event = 'done') AS done,
                  sum(event = 'skipped_cache') AS cached,
                  sum(event = 'failed') AS failed,
                  (SELECT coalesce(sum(amount_usd), 0) FROM costs c
                   WHERE c.run_id = e.run_id AND c.step_id = e.step_id) AS usd
                FROM step_events e
                WHERE run_id = 'RUN_ID'
                GROUP BY step_id
                ORDER BY min(id)"
    ```

    Replace `RUN_ID` with the same id. It prints:

    ```
    {"cached":0,"claimed":0,"done":1,"failed":0,"step_id":"source","usd":0}
    {"cached":3,"claimed":2,"done":2,"failed":0,"step_id":"score","usd":0.02}
    {"cached":0,"claimed":0,"done":5,"failed":0,"step_id":"keep","usd":0}
    {"cached":1,"claimed":1,"done":1,"failed":0,"step_id":"out","usd":0}
    4 rows
    ```

    Every number matches the receipt, row for row, except `cached 1` on `out`, because `gtme runs` adds up ledger rows and nothing else. `skipped_cache` rows include the deliver step's `already_delivered` skip, which the receipt and `gtme runs` count as already delivered, not cached. The $0.02 on `score` is two `costs` rows, one per new person, both with `estimated` in `basis`.

1. List what each run of the last week cost:

    ```sh
    gtme query "SELECT r.id AS run, r.pipeline,
                  substr(r.started_at, 1, 16) AS started,
                  coalesce(sum(c.amount_usd) FILTER (WHERE c.basis = 'measured'), 0) AS measured,
                  coalesce(sum(c.amount_usd) FILTER (WHERE c.basis = 'estimated'), 0) AS estimated
                FROM runs r
                LEFT JOIN costs c ON c.run_id = r.id
                WHERE r.started_at >= date('now', '-7 days')
                GROUP BY r.id
                ORDER BY r.started_at"
    ```

    It prints:

    ```
    {"estimated":0.03,"measured":0,"pipeline":"hello","run":"01M3K4RSTADKBKQVXJ76WG7575","started":"2026-09-28T04:36"}
    {"estimated":0,"measured":0,"pipeline":"hello","run":"01M3K4RSVACWB04NTNW6PP9CDP","started":"2026-09-28T04:36"}
    {"estimated":0.02,"measured":0,"pipeline":"hello","run":"01M3K4RSW88DCY1YHGHE9QJ1F1","started":"2026-09-28T04:36"}
    {"estimated":0,"measured":0.5,"pipeline":"openers","run":"01M3K4RSXBGSQFV2HW8AQYX6QA","started":"2026-09-28T04:36"}
    4 rows
    ```

    `-7 days` counts back from now, in UTC. To find one run among many, add `AND r.pipeline = 'hello'` to the `WHERE` line.

1. Total the month by pipeline and by basis:

    ```sh
    gtme query "SELECT r.pipeline,
                  count(DISTINCT r.id) AS runs,
                  coalesce(sum(c.amount_usd) FILTER (WHERE c.basis = 'measured'), 0) AS measured,
                  coalesce(sum(c.amount_usd) FILTER (WHERE c.basis = 'estimated'), 0) AS estimated
                FROM runs r
                LEFT JOIN costs c ON c.run_id = r.id
                WHERE r.started_at >= date('now', 'start of month')
                GROUP BY r.pipeline"
    ```

    It prints:

    ```
    {"estimated":0.05,"measured":0,"pipeline":"hello","runs":3}
    {"estimated":0,"measured":0.5,"pipeline":"openers","runs":1}
    2 rows
    ```

    The total includes what dry-runs spent and what pending runs have spent so far. The month starts at midnight UTC.

    **Report the two columns separately.** A measured dollar is on a bill already. An estimated one is a count times a rate someone typed, so check it against the vendor's invoice before you quote it.

## What you have now

**Any run in the ledger, and any week or month of them, answered from the ledger alone.** To have Claude Code report this way, paste this line:

```text
Report on my gtme runs: list them, print the receipt of RUN_ID with gtme runs and read sourced, filtered, and delivered counts from its records line, check its counts and cost against step_events and costs with gtme query, then list this week's cost per run and this month's total by pipeline, with measured and estimated in separate columns.
```

Replace `RUN_ID` with the run you're asked about, or `last`.

## Next

- [`gtme runs`](/reference/cli/runs) lists every form of the command, including `last`.
- [The `costs` table](/reference/ledger-schema/costs) lists every column a cost row carries.
- [`gtme query`](/reference/cli/query) lists every flag of the command the checks use.
