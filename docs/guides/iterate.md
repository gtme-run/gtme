---
name: Iterate
description: Change a pipeline that has already run, check the edit with plan and a small run, read what it re-runs and reuses, then run it at full size
for: "Your pipeline has run for real, you've changed it (a tighter filter, a new template, a new step), and you want to know it works before it runs on the whole list."
learn:
  - "how to check an edit with plan and a 5-record run before the full one"
  - "which steps an edit runs again and which it reuses from the ledger"
  - "why people delivered under the old version don't get the new one, and what to do about it"
  - "what the small run costs next to the full run"
order: 8
roles: [operator, builder]
links:
  - to: /concepts/gate-ladder
    type: depends-on
    description: Plan checks the edit for free, and the small run is a dry-run, which spends and holds every delivery
  - to: /concepts/runs-and-receipts
    type: depends-on
    description: What the receipt's cached column counts, and the target, scope, and key that delivery idempotency matches on
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/pipeline
    type: relates-to
    description: What each key in the edited file does
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The enrich, filter, compose, and deliver roles, which differ in what an edit makes them run again
  - to: /concepts/facts
    type: relates-to
    description: The cache window that keeps an enrich step's results reusable across an edit
  - to: /concepts/ledger
    type: relates-to
    description: The step_events table the reuse queries read
  - to: /start/my-csv
    type: relates-to
    description: Runs a file with AI steps, where a changed prompt re-judges and pays again
  - to: /reference/cli/plan
    type: relates-to
    description: Every line plan prints after an edit
  - to: /reference/ledger-schema/step_events
    type: relates-to
    description: The events and details the reuse queries read, including the judgment signature
  - to: /reference/ledger-schema/deliveries
    type: relates-to
    description: The target, scope, and key each delivery row carries
  - to: /spec#iterate
    type: decided-by
    description: The Iterate story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: Plan checks the edited file against the ledger before any adapter runs
  - to: /spec#deliver-idempotency
    type: decided-by
    description: A deliver step skips a record already delivered to the same target, scope, and key, whatever the new values are
  - to: /decisions#adr-039
    type: decided-by
    description: A compose or AI step reuses a result only when its judgment signature and its inputs both match
  - to: /decisions#adr-045
    type: decided-by
    description: Only a target that updates in place can re-deliver changed values
  - to: /decisions#adr-047
    type: decided-by
    description: The runner enforces a source's limit and stops paging the vendor at the cap
---

# Iterate

**Goal: an edited pipeline, checked on 5 records, then run on the whole list.** You'll see what the edit re-runs, what it reuses, and what happens to people who got the old version. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Iterate story](/spec#iterate):

> a pipeline change is checked before it is trusted, cheaply.

## Before you start

**You need `gtme` ([Install](/start/install)) and a [pipeline](/concepts/pipeline) that has already run for real, with the [ledger](/concepts/ledger) it ran against.** On your own pipeline, go straight to the steps and make your own edit. Otherwise, build the practice history first, and the outputs will match this page.

**This guide needs no API keys and sends nothing past your own disk.** `demo/enrich` charges a pretend $0.01 per record and calls no vendor, and the edit spends a pretend $0.12 in all. The one deliver step writes CSV files in your folder. On your own pipeline, the small check spends 5 records' worth of each priced step and sends nothing.

## Build a practice history

**The practice file keeps the VPs and heads of a function in a lead list and writes each one an opening line.** Make a folder with its own ledger:

```sh
mkdir -p iterate && cd iterate
export GTME_LEDGER=./ledger.db
```

Save the list as `leads.csv`:

```sh
cat > leads.csv <<'EOF'
Full Name,Email,Title,Company Website
Jane Doe,jane.doe@acme.com,VP Marketing,acme.com
Bob Stone,bob@globex.io,Head of Growth,globex.io
Carol Reyes,carol@initech.dev,Marketing Operations Manager,initech.dev
Dana Park,dana@contoso.com,Head of Growth,contoso.com
Eli Moss,eli@umbrella.co,VP Sales,umbrella.co
Fay Chen,fay@wayne.io,VP Demand Gen,wayne.io
Gus Lee,gus@hooli.com,Director of Demand Gen,hooli.com
Hana Sato,hana@stark.io,Head of Marketing,stark.io
Ivan Cruz,ivan@soylent.co,VP Revenue,soylent.co
Jo Kim,jo@tyrell.ai,Head of Sales,tyrell.ai
Kai Wren,kai@cyberdyne.io,Account Executive,cyberdyne.io
Lu Ortiz,lu@oscorp.dev,VP Marketing,oscorp.dev
EOF
```

Save this as `leads.yaml`:

```yaml
name: leads
version: 1

source:
  use: csv/source
  with:
    path: leads.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: keep
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head of%')

  - id: opener
    use: text/compose
    uses: [full_name, company_domain]
    provides: [opener]
    with:
      template: "{{ record.full_name }}, a quick question about growth at {{ record.company_domain }}"

  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      full_name: full_name
      opener: leads.opener
    idempotency: email
```

Launch it [armed](/concepts/gate-ladder). This spends $0 and writes 9 rows to `out.csv`:

```sh
gtme run leads.yaml
```

The run ends with this [receipt](/concepts/runs-and-receipts):

```
step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
source  csv/source    0   12   -      0       -         -       $0    -
keep    sql/filter    12  9    -      0       3         -       $0    -
opener  text/compose  9   9    -      0       -         -       $0    -
out     csv/deliver   9   9    -      0       -         -       $0    -
total: $0 spent
```

Nine people are in `out.csv`, each with a line about growth. The `keep` query reads `current_values`, which holds each person's latest value for every field, and returns the people to keep.

## Steps

1. Edit the file. The new version adds a `score` step, keeps only leaders who score 70 or more, and gives `opener` a new question. Replace `leads.yaml` with:

    ```yaml
    name: leads
    version: 1

    source:
      use: csv/source
      with:
        path: leads.csv
        columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

    steps:
      - id: score
        use: demo/enrich
        with:
          cost_per_record_usd: 0.01

      - id: keep
        use: sql/filter
        with:
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head of%')
            INTERSECT
            SELECT identity_id FROM current_values
            WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 70

      - id: opener
        use: text/compose
        uses: [full_name, company_domain]
        provides: [opener]
        with:
          template: "{{ record.full_name }}, how is {{ record.company_domain }} planning Q4 pipeline?"

      - id: out
        use: csv/deliver
        with:
          path: out.csv
        variables:
          full_name: full_name
          opener: leads.opener
        idempotency: email
    ```

    `score` is a new [enrich](/concepts/steps-and-roles) step, one that adds fields to each record. `keep` has a second condition, and `opener` has a new template. Leave `version: 1` alone. It's the version of the file format, and plan refuses anything else:

    ```
    gtme: leads.yaml: pipeline: unsupported version 2 (v0 understands version 1)
    ```

1. Plan it:

    ```sh
    gtme plan leads.yaml
    ```

    Plan prints the new step with its price:

    ```
    pipeline leads (version 1)
    ...
    2. score [enrich] — demo/enrich@1
         entity:    person
         reads:     email, full_name
         requires:  any of email | full_name
         provides:  demo.note, demo.score
         cache:     30d
         est/record: $0.0100
    ...
    plan ok — nothing has been spent
    ```

    If plan names a step and a field instead, the edit reads something no earlier step provides. Fix that step before anything else. Plan checks the file against the ledger before any [adapter](/concepts/adapter-tiers) runs ([SPEC §7](/spec#7-contract-validation--the-planner--decided)). It can't tell you whether the new query keeps the right people or whether the opener reads well. A small run can.

1. Cap the source at 5 records. Under the source's `with:`, add:

    ```yaml
        limit: 5
    ```

    The runner takes the first 5 records the source returns, so every later step sees at most 5. On a vendor source, it also stops paging the vendor there (the decision record, [ADR-047](/decisions#adr-047)). If none of the 5 pass your filter, raise the limit until a few do, because a check on 0 records checks nothing.

1. Dry-run it. A dry-run runs every step and holds every delivery, so you read the result before anyone gets it. This spends a pretend $0.05 and sends nothing:

    ```sh
    gtme run leads.yaml --dry-run
    ```

    The output is similar to the following:

    ```
    dry run: deliver steps will resolve and receipt their variables, but nothing sends
    ...
    run 01M3K7TQM5KDT3ZZCCTQCFYM6P — done (dry run — nothing sent)
    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   5    -      0       -         -       $0       -
    score   demo/enrich   5   5    -      0       -         -       $0.0500  -
    keep    sql/filter    5   2    -      0       3         -       $0       -
    opener  text/compose  2   2    -      0       -         -       $0       -
    out     csv/deliver   2   0    -      2       -         -       $0       $0.0000
    total: $0.0500 (estimated) spent, $0.0000 avoided via cache (2 records skipped)
    ```

    The whole chain ran on 5 records for $0.0500. Two people passed `keep`, and `out` shows `cached 2`: both are already in `out.csv`, so the deliver step skips them and there's nothing to review. This run can't show you the new line, and an armed run wouldn't send it to them.

1. Read what the edit ran again. This counts the newest run's events per step, by reason:

    ```sh
    gtme query "SELECT step_id, event,
                  json_extract(detail, '$.reason') AS reason, count(*) AS n
                FROM step_events
                WHERE run_id = (SELECT max(id) FROM runs)
                GROUP BY 1, 2, 3 ORDER BY min(id)"
    ```

    It prints the following. `claimed` means the step started work on a record:

    ```
    {"event":"done","n":1,"reason":null,"step_id":"source"}
    {"event":"claimed","n":5,"reason":null,"step_id":"score"}
    {"event":"done","n":5,"reason":null,"step_id":"score"}
    {"event":"done","n":2,"reason":"selected by predicate","step_id":"keep"}
    {"event":"done","n":3,"reason":"not selected by predicate","step_id":"keep"}
    {"event":"claimed","n":2,"reason":null,"step_id":"opener"}
    {"event":"done","n":2,"reason":null,"step_id":"opener"}
    {"event":"skipped_cache","n":2,"reason":"already_delivered","step_id":"out"}
    8 rows
    ```

    Each kind of step treats an edit differently:

    | Step | What it did | Why |
    |---|---|---|
    | `score` | Called the adapter for all 5 | It's new, so nothing was cached. Next time it reuses these scores for 30 days. |
    | `keep` | Judged all 5 | A `sql/filter` step recomputes on every run, so a changed query takes effect at once, for $0. |
    | `opener` | Composed 2 again | Both had a line from the launch, but the template changed. |
    | `out` | Skipped 2 | Both are already in `out.csv`. |

1. See why `opener` ran again. Compare Jane's two compositions:

    ```sh
    gtme query "SELECT e.run_id,
                  json_extract(e.detail, '$.signature') AS signature,
                  json_extract(e.detail, '$.input') AS input
                FROM step_events e JOIN identities i ON i.id = e.identity_id
                WHERE e.step_id = 'opener' AND e.event = 'done'
                  AND i.identity_key = 'jane.doe@acme.com'
                ORDER BY e.id"
    ```

    It prints:

    ```
    {"input":"66e6f8c7179f","run_id":"01M3K7TQHQ17DGE86AZTM95TW1","signature":"535b16ff65bd"}
    {"input":"66e6f8c7179f","run_id":"01M3K7TQM5KDT3ZZCCTQCFYM6P","signature":"e0f2bf60d044"}
    2 rows
    ```

    `input` is a hash of the fields the step reads, and it's the same in both runs. `signature` is a hash of the step's adapter, template, and declared fields, and the new template changed it. A `text/compose` or `ai/*` step reuses a result only when both match ([ADR-039](/decisions#adr-039)).

    Here that re-run was free. On an `ai/filter` or `ai/compose` step, a changed prompt or model re-judges, and pays for, every record that reaches it. A small run caps that at the records it lets through. An enrich step's cache looks only at its own fields and [cache window](/concepts/facts), so edits elsewhere in the file leave it cached.

1. Decide whether people who got the old line should get the new one. Idempotency, the rule that stops a repeat delivery, matches on where a record was delivered (here `out.csv`) and its `idempotency:` field, `email`. The opener isn't part of that match, so anyone already in `out.csv` never gets the new one ([SPEC §8](/spec#deliver-idempotency)).

    We'd leave it: people who got the old line are skipped, and everyone new gets the new one. To send the new version to everyone, including a second message to past recipients, point `out` at a new file, or a new campaign on a live target. For this page, set:

    ```yaml
          path: out-q4.csv
    ```

1. Dry-run the small run again. This spends $0 and sends nothing:

    ```sh
    gtme run leads.yaml --dry-run
    ```

    The output is similar to the following:

    ```
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   5    -      0       -         -       $0    -
    score   demo/enrich   5   0    -      5       -         -       $0    $0.0500
    keep    sql/filter    5   2    -      0       3         -       $0    -
    opener  text/compose  2   0    -      2       -         -       $0    ?
    out     csv/deliver   2   0    -      0       -         -       $0    -
    out: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        full_name: "Jane Doe"
        opener: "Jane Doe, how is acme.com planning Q4 pipeline?"
      dana@contoso.com
        full_name: "Dana Park"
        opener: "Dana Park, how is contoso.com planning Q4 pipeline?"
    total: $0 spent, $0.0500+? avoided via cache (7 records skipped)
    ```

    `score` and `opener` came from the ledger, and both people now resolve with the new line. `?` means a template has no price to count as saved. A target that updates a record in place, such as `attio/assert`, can take `redeliver: on_change` and re-deliver changed values instead ([ADR-045](/decisions#adr-045)). Plan refuses it on a target that appends or sends, such as `csv/deliver`.

1. Delete the `limit: 5` line and dry-run the whole list. This spends a pretend $0.07 and sends nothing:

    ```sh
    gtme run leads.yaml --dry-run
    ```

    The output is similar to the following:

    ```
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   12   -      0       -         -       $0       -
    score   demo/enrich   12  7    -      5       -         -       $0.0700  $0.0500
    keep    sql/filter    12  4    -      0       8         -       $0       -
    opener  text/compose  4   2    -      2       -         -       $0       ?
    out     csv/deliver   4   0    -      0       -         -       $0       -
    out: resolved variables for 4 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        full_name: "Jane Doe"
        opener: "Jane Doe, how is acme.com planning Q4 pipeline?"
    ...
    total: $0.0700 (estimated) spent, $0.0500+? avoided via cache (7 records skipped)
    ```

    `score` paid for the 7 people the small run hadn't seen and reused the 5 it had. Read all 4 resolved lines, because they're exactly what the armed run sends.

1. Run it armed. This spends $0 and writes 4 rows to `out-q4.csv`:

    ```sh
    gtme run leads.yaml
    ```

    The output is similar to the following:

    ```
    ...
    run 01M3K7TQSBH8X9R01EEYNB6ESC — done
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   12   -      0       -         -       $0    -
    score   demo/enrich   12  0    -      12      -         -       $0    $0.1200
    keep    sql/filter    12  4    -      0       8         -       $0    -
    opener  text/compose  4   0    -      4       -         -       $0    ?
    out     csv/deliver   4   4    -      0       -         -       $0    -
    total: $0 spent, $0.1200+? avoided via cache (16 records skipped)
    ```

    Everything the armed run needed came from the two dry-runs, so it spent nothing and only delivered.

## What you have now

**The edited pipeline, run on the whole list, and a record of what checking it cost.** `out-q4.csv` holds the 4 people who passed the new filter, each with the new line:

```sh
cat out-q4.csv
```

The output is similar to the following. The rows can come back in a different order:

```
identity_key,full_name,opener
jo@tyrell.ai,Jo Kim,"Jo Kim, how is tyrell.ai planning Q4 pipeline?"
jane.doe@acme.com,Jane Doe,"Jane Doe, how is acme.com planning Q4 pipeline?"
dana@contoso.com,Dana Park,"Dana Park, how is contoso.com planning Q4 pipeline?"
hana@stark.io,Hana Sato,"Hana Sato, how is stark.io planning Q4 pipeline?"
```

The four runs spent this, from their receipts:

| Run | Records | Spent |
|---|---|---|
| Small dry-run | 5 | $0.0500 |
| Small dry-run, new file | 5 | $0 |
| Full dry-run | 12 | $0.0700 |
| Full armed run | 12 | $0 |

That's $0.12, the same as one full run with no check, because each run reused every score the one before it paid for. With an AI step, checking costs extra only when the small run changes your mind: each new prompt re-judges the records the small run already saw.

To have Claude Code check your next edit this way, paste this line:

```text
I edited PIPELINE. Run gtme plan on it, then add limit: 5 under source.with and dry-run it. Show me the receipt, the resolved variables, and the run's step_events counted by step and reason. If a deliver step skipped anyone as already_delivered, tell me who and ask whether to deliver to a new target. Then remove the limit, dry-run it at full size, show me the receipt and resolved variables, and run it armed only after I say go.
```

Replace `PIPELINE` with the pipeline file.

## Next

- [`gtme plan`](/reference/cli/plan) lists every line plan prints after an edit.
- [The `step_events` table](/reference/ledger-schema/step_events) lists the events and details the reuse queries read.
- [The `deliveries` table](/reference/ledger-schema/deliveries) lists the target, scope, and key every delivery row carries.
