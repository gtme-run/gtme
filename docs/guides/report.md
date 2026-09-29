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
resuming run 01M3QAG9K5PNTN183GS6Y5E65N (openers)
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
    01M3QAG9K5PNTN183GS6Y5E65N  openers   done    2026-09-29T19:33:12.421Z  5        -
    01M3QAG9HSVM8SHXTK9XFB2DHQ  hello     done    2026-09-29T19:33:12.377Z  5        -
    01M3QAG9GX892QZ22J8HVSJDGF  hello     done    2026-09-29T19:33:12.349Z  3        -
    01M3QAG9FFDHY95DHAKSSX0PB1  hello     done    2026-09-29T19:33:12.303Z  3        -
    ```

    Runs are newest first, and `started` is in UTC. `records` counts the people each run touched. `in flight` counts records still waiting on a person or a vendor, and such a run's status is `pending`. A [dry-run](/concepts/gate-ladder) shows as `done (dry)`, and what it spent is real. A run whose process died shows as `interrupted`, and its `in flight` counts the sends that got no response.

1. Print one run's receipt from the ledger. This is the third `hello` run:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the `run` column of the row you want, here `01M3QAG9HSVM8SHXTK9XFB2DHQ`, or use `last` for the newest. It prints:

    ```
    run 01M3QAG9HSVM8SHXTK9XFB2DHQ
    pipeline: hello
    status:   done
    started:  2026-09-29T19:33:12.377Z
    finished: 2026-09-29T19:33:12.410Z

    step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source   0   5    -      0       -         -       $0       -
    score   demo/enrich  5   2    -      3       -         -       $0.0200  $0.0300
    keep    sql/filter   5   2    -      0       3         -       $0       -
    out     csv/deliver  2   1    -      0       -         -       $0       -
    out: 1 already delivered
    total: $0.0200 (estimated) spent, $0.0300 avoided via cache (3 records skipped)
    records: 5 (out=2 score=3), 3 with a fail verdict (filtered, or a send withheld)
    config:  3 steps recorded (`gtme freeze 01M3QAG9HSVM8SHXTK9XFB2DHQ` rebuilds the pipeline)
    ```

    **This is the table the run printed when it finished, rebuilt from the ledger, with the run-level lines after it.** This table shows where to read each answer:

    | Question | Where |
    |---|---|
    | How many people did it source? | The `records:` line's first number, 5 |
    | How many did a [filter](/concepts/steps-and-roles) drop? | `filtered` on the `keep` row, 3 |
    | How many did a step pay for, and how many did it reuse? | `out` and `cached`: `score` paid for 2 and reused 3 |
    | How many did it deliver? | `out` on the `out` row, 1, and the line under the table: 1 already delivered |
    | What did it cost, and what did the cache save? | The `cost` and `avoided` columns and the `total:` line |

    Here that's 5 sourced, 3 filtered, 2 new scores paid for, and Dana delivered, with Jane skipped as already delivered. For a resumed run, the table counts each record once, by its latest outcome, across every session, so it's the one to quote. A run recorded by an older `gtme` prints `?` where its ledger never stored a number, such as `avoided`.

1. To prove a receipt, rebuild its step table from `step_events` and `costs`, the ledger tables it's read from. Each per-record event names its column in `detail.outcome`:

    ```sh
    gtme query "WITH latest AS (
                  SELECT id, step_id, json_extract(detail, '$.outcome') AS outcome,
                         json_extract(detail, '$.avoided_usd') AS avoided,
                         row_number() OVER (PARTITION BY step_id, identity_id
                                            ORDER BY id DESC) AS n
                  FROM step_events
                  WHERE run_id = 'RUN_ID' AND json_extract(detail, '$.outcome') IS NOT NULL)
                SELECT step_id, count(*) AS records,
                  sum(outcome = 'out') AS out, sum(outcome = 'cached') AS cached,
                  sum(outcome = 'filtered') AS filtered, sum(outcome = 'failed') AS failed,
                  sum(outcome = 'already_delivered') AS already_delivered,
                  (SELECT coalesce(sum(amount_usd), 0) FROM costs c
                   WHERE c.run_id = 'RUN_ID' AND c.step_id = l.step_id) AS usd,
                  coalesce(sum(avoided), 0) AS avoided
                FROM latest l
                WHERE n = 1
                GROUP BY step_id
                ORDER BY min(id)"
    ```

    Replace `RUN_ID` with the same id, in both places. It prints:

    ```
    {"already_delivered":0,"avoided":0.03,"cached":3,"failed":0,"filtered":0,"out":2,"records":5,"step_id":"score","usd":0.02}
    {"already_delivered":0,"avoided":0,"cached":0,"failed":0,"filtered":3,"out":2,"records":5,"step_id":"keep","usd":0}
    {"already_delivered":1,"avoided":0,"cached":0,"failed":0,"filtered":0,"out":1,"records":2,"step_id":"out","usd":0}
    3 rows
    ```

    Every number matches the receipt, row for row. `records` is the `in` column. The query keeps each record's latest event at each step, which is how the receipt counts a resumed run. The source row isn't here, because the source's event names no record. The $0.02 on `score` is two `costs` rows, one per new person, both with `estimated` in `basis`.

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
    {"estimated":0.03,"measured":0,"pipeline":"hello","run":"01M3QAG9FFDHY95DHAKSSX0PB1","started":"2026-09-29T19:33"}
    {"estimated":0,"measured":0,"pipeline":"hello","run":"01M3QAG9GX892QZ22J8HVSJDGF","started":"2026-09-29T19:33"}
    {"estimated":0.02,"measured":0,"pipeline":"hello","run":"01M3QAG9HSVM8SHXTK9XFB2DHQ","started":"2026-09-29T19:33"}
    {"estimated":0,"measured":0.5,"pipeline":"openers","run":"01M3QAG9K5PNTN183GS6Y5E65N","started":"2026-09-29T19:33"}
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
Report on my gtme runs: list them, print the receipt of RUN_ID with gtme runs and read sourced from its records line and filtered, cached, and delivered from its table, check its counts and cost against step_events and costs with gtme query, then list this week's cost per run and this month's total by pipeline, with measured and estimated in separate columns.
```

Replace `RUN_ID` with the run you're asked about, or `last`.

## Next

- [`gtme runs`](/reference/cli/runs) lists every form of the command, including `last`.
- [The `costs` table](/reference/ledger-schema/costs) lists every column a cost row carries.
- [`gtme query`](/reference/cli/query) lists every flag of the command the checks use.
