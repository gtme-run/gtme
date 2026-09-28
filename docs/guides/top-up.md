---
name: Top up
description: Re-run a launched pipeline on an overlapping export, preview what it will spend, and prove from the ledger it paid less and delivered nobody twice
for: "You launched a pipeline, a fresh export or list just arrived with some of the same people, some new ones, and some changed rows, and you want to run it again without paying or sending twice."
learn:
  - "how to read what a top-up will spend before it spends"
  - "how to prove from the ledger that overlapping people were skipped and nobody was delivered twice"
  - "why a changed row doesn't make an enrich step pay again"
  - "the two cases that do pay or send again: a fact past its cache window, and a new delivery file or campaign"
order: 7
roles: [operator]
links:
  - to: /concepts/facts
    type: depends-on
    description: The cache window decides which facts are fresh enough to skip, which is what makes a top-up cheaper
  - to: /concepts/runs-and-receipts
    type: depends-on
    description: What the cached and avoided columns count, and why delivery idempotency holds per target and scope
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate is the rung that previews the top-up with a copy of the ledger, and the armed run is the top-up
  - to: /concepts/ledger
    type: relates-to
    description: The step_events and deliveries tables the proof queries read
  - to: /concepts/pipeline
    type: relates-to
    description: What the practice file declares, step by step
  - to: /concepts/groups
    type: relates-to
    description: A suppression group is the rule for not touching someone again across files or campaigns
  - to: /start/show-me
    type: relates-to
    description: Runs a keyless pipeline twice on the same CSV, the simplest version of this guide
  - to: /reference/cli/query
    type: relates-to
    description: Every flag of the command the proof queries use
  - to: /reference/cli/show
    type: relates-to
    description: Every flag of gtme show, including --provenance and --fields
  - to: /spec#top-up
    type: decided-by
    description: The Top-up story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: An enrich step skips a record when every field it provides is current inside the window, logged as skipped_cache
  - to: /spec#deliver-idempotency
    type: decided-by
    description: A deliver step skips a record already in deliveries for the same target, scope, and key
  - to: /decisions#adr-044
    type: decided-by
    description: Delivery dedupe is scoped to the campaign or file, so a new one is a fresh decision
  - to: /decisions#adr-039
    type: decided-by
    description: Simulate cache-skips exactly as an armed run would, and an AI step re-judges when its inputs change
---

# Top up

**Goal: a top-up run that pays only for new people and delivers nobody twice.** It re-runs a launched pipeline over a new export, and the [ledger](/concepts/ledger) proves the result. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Top-up story](/spec#top-up):

> re-running a pipeline against overlapping data costs less and delivers nothing twice.

## Before you start

**You need `gtme` ([Install](/start/install)) and a pipeline you've already run [armed](/concepts/gate-ladder), for real with no flag, plus a new export for it.** Run the top-up against the same ledger as the launch. A different ledger has no history, so gtme would pay for and deliver to everyone again. On your own ledger, go straight to the steps. Otherwise, build the practice history first, and the outputs will match this page.

**This guide needs no API keys and sends nothing past your own disk.** `demo/enrich` charges a pretend $0.01 per record and calls no vendor. The one [deliver](/concepts/steps-and-roles) step writes to CSV files in your folder.

## Build a practice history

**The practice file is a webinar follow-up: score each attendee, keep anyone scoring 70 or more, and write them to `follow-up.csv`.** Make a folder with its own ledger:

```sh
mkdir -p top-up && cd top-up
export GTME_LEDGER=./ledger.db
```

Save this [pipeline](/concepts/pipeline) as `attendees.yaml`:

```yaml
name: attendees
version: 1

source:
  use: csv/source
  with:
    path: attendees.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: score
    use: demo/enrich
    cache: 30d
    with:
      cost_per_record_usd: 0.01

  - id: keep
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 70

  - id: follow_up
    use: csv/deliver
    with:
      path: follow-up.csv
    variables:
      full_name: full_name
      score: demo.score
    idempotency: email
```

`score` is an enrich step, one that adds fields to each record, and `cache: 30d` is its [cache window](/concepts/facts): a score written in the last 30 days counts as fresh. `idempotency: email` on `follow_up` keys each delivery on the email address.

Save the first export and launch:

```sh
cat > attendees.csv <<'EOF'
Full Name,Email,Title,Company Website
Jane Doe,jane.doe@acme.com,VP Marketing,acme.com
Bob Stone,bob@globex.io,Head of Growth,globex.io
Carol Reyes,carol@initech.dev,Marketing Operations Manager,initech.dev
Dana Park,dana@contoso.com,Head of Growth,contoso.com
EOF
gtme run attendees.yaml
```

The run ends with this [receipt](/concepts/runs-and-receipts):

```
step       adapter      in  out  empty  cached  filtered  failed  cost     avoided
source     csv/source   0   4    -      0       -         -       $0       -
score      demo/enrich  4   4    -      0       -         -       $0.0400  -
keep       sql/filter   4   2    -      0       2         -       $0       -
follow_up  csv/deliver  2   2    -      0       -         -       $0       -
total: $0.0400 (estimated) spent
```

Jane and Dana scored 70 or more, passed `keep`, and went to `follow-up.csv`. Now the new export arrives. Jane and Dana are back, Bob's title changed, Carol is gone, and Gus and Hana are new. Replace the file:

```sh
cat > attendees.csv <<'EOF'
Full Name,Email,Title,Company Website
Jane Doe,jane.doe@acme.com,VP Marketing,acme.com
Bob Stone,bob@globex.io,VP Growth,globex.io
Dana Park,dana@contoso.com,Head of Growth,contoso.com
Gus Lee,gus@hooli.com,Director of Demand Gen,hooli.com
Hana Sato,hana@stark.io,CMO,stark.io
EOF
```

## Steps

1. Preview the top-up. Simulate, the lowest rung of the gate ladder, spends and sends nothing, and keeps nothing:

    ```sh
    gtme run attendees.yaml --simulate
    ```

    The output is similar to the following:

    ```
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    ...
    run 01M3K60VSDD1YXKTZXJNFP22M6 — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
    step       adapter      in  out  empty  cached  filtered  failed  cost     avoided
    source     csv/source   0   5    -      0       -         -       $0       -
    score      demo/enrich  5   2    -      3       -         -       $0.0200  $0.0300
    keep       sql/filter   5   4    -      0       1         -       $0       -
    follow_up  csv/deliver  4   0    -      2       -         -       $0       $0.0000
    follow_up: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
      gus@hooli.com
        full_name: "Gus Lee"
        score: "91"
      hana@stark.io
        full_name: "Hana Sato"
        score: "81"
    total: $0.0200 (estimated) spent, $0.0300 avoided via cache (5 records skipped)
    ```

    **The `score` row says the top-up pays for two people: `cached 3`, `$0.0200`.** Jane, Bob, and Dana have fresh scores, so it pays for Gus and Hana only. The `follow_up` row shows `cached 2`, because Jane and Dana are already in `follow-up.csv`, and the resolved variables list the two people it will write. The total's `5 records skipped` is those three scores plus those two deliveries. The hint names `--dry-run` because simulate borrows dry-run's wording; the armed run is next only because this file is keyless.

    Simulate works on a copy of the ledger, so its cache skips are exactly the armed run's (the decision record, [ADR-039](/decisions#adr-039)). Its results are not: a vendor search returns recorded responses, and a model step returns canned text for $0. With either kind of step, or a live delivery target such as a campaign, run a dry-run next and read the same counts there before you arm.

1. Run the top-up armed, the run with no flag. This spends the pretend $0.02 and writes two rows to `follow-up.csv`:

    ```sh
    gtme run attendees.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3K60VTDBY3DB55F58J4DWD0 (attendees)
    ...
    step       adapter      in  out  empty  cached  filtered  failed  cost     avoided
    source     csv/source   0   5    -      0       -         -       $0       -
    score      demo/enrich  5   2    -      3       -         -       $0.0200  $0.0300
    keep       sql/filter   5   4    -      0       1         -       $0       -
    follow_up  csv/deliver  4   2    -      2       -         -       $0       $0.0000
    total: $0.0200 (estimated) spent, $0.0300 avoided via cache (5 records skipped)
    ```

    The receipt matches the preview. `avoided $0.0300` is the "costs less" half of the promise.

    A person whose last attempt failed, or came back with nothing written, has no fresh value, so a top-up tries them again and pays for them. One whose last attempt wrote some of the step's fields counts as done and is skipped.

1. Prove the skips from the ledger. This lists every record the newest run skipped, and why:

    ```sh
    gtme query "SELECT i.identity_key, e.step_id,
                  json_extract(e.detail, '$.reason') AS reason
                FROM step_events e
                JOIN identities i ON i.id = e.identity_id
                WHERE e.run_id = (SELECT max(id) FROM runs)
                  AND e.event = 'skipped_cache'
                ORDER BY e.id"
    ```

    It prints:

    ```
    {"identity_key":"jane.doe@acme.com","reason":"fresh_in_ledger","step_id":"score"}
    {"identity_key":"bob@globex.io","reason":"fresh_in_ledger","step_id":"score"}
    {"identity_key":"dana@contoso.com","reason":"fresh_in_ledger","step_id":"score"}
    {"identity_key":"jane.doe@acme.com","reason":"already_delivered","step_id":"follow_up"}
    {"identity_key":"dana@contoso.com","reason":"already_delivered","step_id":"follow_up"}
    5 rows
    ```

    Every overlapping person has a `skipped_cache` event on `score`, in the ledger's `step_events` table. `fresh_in_ledger` means every field the step provides already had a value inside its window, so the runner called no [adapter](/concepts/adapter-tiers) and wrote no cost ([SPEC §7](/spec#7-contract-validation--the-planner--decided)). `already_delivered` is the delivery check ([SPEC §8](/spec#deliver-idempotency)).

1. Check the `deliveries` table, where each delivery row names its target file or campaign as its `scope`, to see that nobody was delivered twice to this file:

    ```sh
    gtme query "SELECT idempotency, run_id FROM deliveries
                WHERE scope = 'follow-up.csv' ORDER BY created_at"
    ```

    The output is similar to the following. Rows written in the same moment can come back in either order:

    ```
    {"idempotency":"jane.doe@acme.com","run_id":"01M3K60RE7XV6CVE2J7FJD37B5"}
    {"idempotency":"dana@contoso.com","run_id":"01M3K60RE7XV6CVE2J7FJD37B5"}
    {"idempotency":"gus@hooli.com","run_id":"01M3K60VTDBY3DB55F58J4DWD0"}
    {"idempotency":"hana@stark.io","run_id":"01M3K60VTDBY3DB55F58J4DWD0"}
    4 rows
    ```

    There's one row per person. Jane's and Dana's rows still carry the launch's run id, and the top-up added rows for Gus and Hana only. That's the "delivers nothing twice" half.

1. Look at Bob, whose title changed:

    ```sh
    gtme show bob@globex.io --provenance --fields title,demo.score
    ```

    It prints:

    ```json
    {
      "entity_type": "person",
      "fields": {
        "demo.score": {
          "confidence": 1,
          "created_at": "2026-09-28T04:57:51.309Z",
          "run_id": "01M3K60RE7XV6CVE2J7FJD37B5",
          "source": "demo/enrich@1",
          "value": 69
        },
        "title": {
          "confidence": 1,
          "created_at": "2026-09-28T04:57:54.766Z",
          "run_id": "01M3K60VTDBY3DB55F58J4DWD0",
          "source": "csv/source@1",
          "value": "VP Growth"
        }
      },
      "identity_key": "bob@globex.io",
      "identity_key_tier": "email"
    }
    ```

    **A changed row doesn't make an enrich step pay again.** The title is the top-up's, and the score is still the launch's. The cache check looks only at the fields a step writes, so Bob keeps his old score, and stays filtered out, until the window runs out. gtme has no flag to re-score one person early.

    If a step's output depends on a field that changes often, give it a shorter `cache:`. A model step, `ai/*`, works the other way and judges again when the fields it reads change (ADR-039).

## When a top-up pays or sends again

**A fact past its cache window gets paid for again.** Ask the ledger when each score goes stale:

```sh
gtme query "SELECT i.identity_key,
              datetime(f.created_at, '+30 days') AS pays_again_after
            FROM current_fields f
            JOIN identities i ON i.id = f.identity_id
            WHERE f.field = 'demo.score' AND f.source LIKE 'demo/enrich%'
            ORDER BY f.created_at"
```

The output is similar to the following:

```
{"identity_key":"bob@globex.io","pays_again_after":"2026-10-28 04:57:51"}
{"identity_key":"jane.doe@acme.com","pays_again_after":"2026-10-28 04:57:51"}
{"identity_key":"carol@initech.dev","pays_again_after":"2026-10-28 04:57:51"}
{"identity_key":"dana@contoso.com","pays_again_after":"2026-10-28 04:57:51"}
{"identity_key":"gus@hooli.com","pays_again_after":"2026-10-28 04:57:54"}
{"identity_key":"hana@stark.io","pays_again_after":"2026-10-28 04:57:54"}
6 rows
```

Times are UTC. A top-up after those times calls `demo/enrich` again for each person in that export, writes a new score beside the old one, and the receipt shows the spend. Carol isn't in the new export, so nothing touches her. Change the `30` to the step's window, which `gtme plan` prints on each paid step's `cache:` line. Setting `cache: 0d` turns the cache off, and `gtme plan` warns that every run then pays for every record.

**A new delivery file or campaign is a new delivery.** Idempotency holds per target and scope: the file for `csv/deliver`, the campaign for Instantly. In `attendees.yaml`, point `follow_up` at a new file:

```yaml
    with:
      path: q4-invite.csv
```

Run it armed. This writes to a new file, `q4-invite.csv`, and spends nothing:

```sh
gtme run attendees.yaml
```

The receipt ends with:

```
...
score      demo/enrich  5   0    -      5       -         -       $0    $0.0500
keep       sql/filter   5   4    -      0       1         -       $0    -
follow_up  csv/deliver  4   4    -      0       -         -       $0    -
total: $0 spent, $0.0500 avoided via cache (5 records skipped)
```

Every score came from the cache, and all four kept people were delivered to the new file. Jane now has one row per file:

```sh
gtme query "SELECT scope, run_id FROM deliveries
            WHERE idempotency = 'jane.doe@acme.com' ORDER BY created_at"
```

```
{"run_id":"01M3K60RE7XV6CVE2J7FJD37B5","scope":"follow-up.csv"}
{"run_id":"01M3K618NR59147FXH3BCHCVZW","scope":"q4-invite.csv"}
2 rows
```

A new campaign is a new decision, per the decision record, [ADR-044](/decisions#adr-044). To keep someone from a second touch across campaigns, add a [suppression group](/concepts/groups) to the deliver step.

## What you have now

**A topped-up pipeline, and two queries that prove it paid less and delivered nobody twice.** `follow-up.csv` holds each kept person once:

```sh
cat follow-up.csv
```

The output is similar to the following:

```
identity_key,full_name,score
jane.doe@acme.com,Jane Doe,100
dana@contoso.com,Dana Park,75
gus@hooli.com,Gus Lee,91
hana@stark.io,Hana Sato,81
```

To have Claude Code run the next top-up this way, paste this line:

```text
Top up PIPELINE with the new export, using the same GTME_LEDGER as the launch: run it with --simulate and show me the receipt, then dry-run it if it has a vendor or model step or a live target, then after I say go, run it armed and prove with gtme query that the overlapping people were skipped_cache on every paid step and nobody already in deliveries for this target and scope got a second row.
```

Replace `PIPELINE` with the pipeline file.

## Next

- [The `step_events` table](/reference/ledger-schema/step_events) lists the columns and events the skip proof reads.
- [The `deliveries` table](/reference/ledger-schema/deliveries) lists the target, scope, and key every delivery row carries.
- [`gtme query`](/reference/cli/query) lists every flag of the command the proofs use.
