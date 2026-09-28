---
name: Guard
description: Catch a pipeline that would fail or overspend before it starts, with plan and simulate, and set the spend ceiling before the first run that spends
for: "Someone handed you a pipeline file, and you want every mistake in it caught, and its most expensive run priced, before anything spends."
learn:
  - "what plan catches, and what it guarantees when it does"
  - "how to prove a failed plan wrote nothing to the ledger"
  - "what plan can't see in a sql/filter step, and the check that can"
  - "how to bound a run's vendor spend before it starts"
order: 4
roles: [operator]
links:
  - to: /concepts/gate-ladder
    type: depends-on
    description: Plan and simulate are the two rungs that spend nothing, and this guide runs only those
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/pipeline
    type: relates-to
    description: The file plan reads, step by step
  - to: /concepts/groups
    type: relates-to
    description: A group the file names must exist in your ledger before plan passes
  - to: /concepts/ledger
    type: relates-to
    description: The costs and runs tables the zero-rows check reads
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A bundle folder is checked with simulate, because plan doesn't accept a folder yet
  - to: /guides/connect-your-stack
    type: relates-to
    description: Stores the vendor keys plan needs before it prints a vendor file's prices
  - to: /guides/launch
    type: relates-to
    description: The run that follows a clean guard, with the dry-run as the go/no-go
  - to: /reference/cli/plan
    type: relates-to
    description: Every line plan prints, and its exit codes
  - to: /reference/cli/run
    type: relates-to
    description: The --simulate flag this guide uses and --dry-run, the next rung and the first that spends
  - to: /reference/cli/groups
    type: relates-to
    description: Creates and fills a group such as the suppression group plan asked for
  - to: /spec#guard
    type: decided-by
    description: The Guard story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: What plan validates, with no network calls and no spend
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: A declared credential that doesn't resolve is a plan error
  - to: /decisions#adr-047
    type: decided-by
    description: A source's limit is enforced by the runner and stops the vendor's pagination at the cap
---

# Guard

**Goal: the file you were handed, passing plan, and a spend ceiling for its first real run.** You spend nothing to get there. SPEC.md, the file that defines what gtme does, states the promise this guide checks in its [Guard story](/spec#guard):

> a pipeline that would fail or overspend is caught before it starts, not partway through.

## Before you start

**You need `gtme` and a [pipeline](/concepts/pipeline) file you didn't write.** [Install](/start/install) covers the first. For the second, this guide uses a colleague's file with three problems in it, built on the practice CSV from [See it run](/start/show-me).

**Steps 1 through 7 need no keys, spend nothing, and send nothing.** They run only plan and simulate, the two rungs of the [gate ladder](/concepts/gate-ladder) that spend nothing. Step 8 needs a vendor file's keys stored, as in [Connect your stack](/guides/connect-your-stack), before plan prints its prices. Plan checks that a key resolves and never calls the vendor with it.

**If you were handed a [bundle](/concepts/campaign-is-a-folder) folder, see the last section.**

## Steps

1. Make a folder with its own [ledger](/concepts/ledger), and fetch the CSV:

    ```sh
    mkdir -p guard && cd guard
    export GTME_LEDGER=./ledger.db
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
    ```

    Save your colleague's file as `q4-openers.yaml`:

    ```yaml
    name: q4-openers
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

      - id: keep
        use: sql/filter
        exclude: [q3-sent]
        with:
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'demo.score' AND CAST(valeu AS INTEGER) >= 70

      - id: opener
        use: text/compose
        uses: [first_name, company_domain]
        provides: [opener]
        with:
          template: "Hi {{ record.first_name }}, a quick question about {{ record.company_domain }}"

      - id: out
        use: csv/deliver
        with:
          path: out.csv
        variables:
          opener: q4-openers.opener
          score: demo.score
        idempotency: email
    ```

    `exclude: [q3-sent]` skips anyone in the `q3-sent` group, and `idempotency: email` means no one is delivered to `out.csv` twice.

1. Plan it, and print the exit code:

    ```sh
    gtme plan q4-openers.yaml; echo $?
    ```

    It prints:

    ```
    gtme: 2 plan problems:
      - step "keep": sql/filter: the query does not plan against the ledger: SQL logic error: no such column: valeu (1)
      - step "opener": needs first_name, which no earlier step provides (available: company_domain, demo.note, demo.score, email, full_name, title); installed adapters provide it: first_name ← apollo/enrich|apollo/search
    2
    ```

    Each problem names its step. `keep` misspells a column, and plan finds it by checking the query against the ledger without running it. `opener` reads `first_name`, and nothing upstream provides that field, so the message lists what's available. Exit code `2` means plan refused the file. A file whose only problems are missing keys exits `3`, and a file with both kinds exits `2`.

1. Check that the failed plan wrote nothing:

    ```sh
    gtme query "SELECT (SELECT count(*) FROM runs) AS runs,
                (SELECT count(*) FROM costs) AS cost_rows"
    ```

    It prints:

    ```
    {"cost_rows":0,"runs":0}
    1 rows
    ```

    No run started, so there's no cost row and no [receipt](/concepts/runs-and-receipts). On a ledger with history, run the same query before and after a failed plan, and the counts don't move. Plan makes no network call ([SPEC §7](/spec#7-contract-validation--the-planner--decided)).

1. Fix both problems, then plan again. In `keep`, change `valeu` to `value`. In `opener`, read the name the CSV has:

    ```yaml
        uses: [full_name, company_domain]
    ```

    ```yaml
          template: "{{ record.full_name }}, a quick question about {{ record.company_domain }}"
    ```

    ```sh
    gtme plan q4-openers.yaml; echo $?
    ```

    It prints:

    ```
    gtme: group "q3-sent" does not exist — create it with `gtme groups add q3-sent <identity-key>...` or snapshot a segment with `gtme groups add q3-sent --from-segment <name>`
    2
    ```

    Plan checks [groups](/concepts/groups) after every field and query passes. `keep` skips anyone in `q3-sent`, the people last quarter's campaign emailed, and your ledger has no such group. Plan checks that a group exists, never who is in it, so what you put in it decides who gets emailed again.

1. Decide what `q3-sent` holds before you create it. If last quarter's campaign ran from this ledger, the group may already exist under another name; `gtme groups` lists them. If a colleague ran it, get their list as a CSV and load it. `gtme groups add` takes only people your ledger already knows, so a new list goes in through a two-step file that sends nowhere:

    ```yaml
    name: load-q3-sent
    version: 1

    source:
      use: csv/source
      with:
        path: q3-sent.csv

    steps:
      - id: remember
        use: group/deliver
        with:
          group: q3-sent
    ```

    `gtme run load-q3-sent.yaml` costs $0, calls no network, and ends with:

    ```
    remember: 2 record(s) handed off to group "q3-sent"
    total: $0 spent
    ```

    Then check who's in it with `gtme groups show q3-sent`. On this practice ledger nobody has been emailed, so an empty group is accurate:

    ```sh
    gtme groups add q3-sent --type person
    ```

    It prints:

    ```
    group q3-sent: 0 added, 0 unchanged
    ```

1. Plan again:

    ```sh
    gtme plan q4-openers.yaml
    ```

    It prints:

    ```
    pipeline q4-openers (version 1)
    ...
    3. keep [filter] — sql/filter
         exclude:   members of q3-sent
         entity:    person
         reads:     (none)
         provides:  (none)
         est/record: ?
    ...
    send surface: 1 deliver step(s)
      out → csv/deliver (touch scope: q4-openers)
    ...
    plan ok — nothing has been spent
    ```

    `keep` now shows `exclude: members of q3-sent`, and the last line is `plan ok`. A SQL step has no vendor price, so its `est/record` is `?`.

1. Simulate it, and read each `sql/filter` row:

    ```sh
    gtme run q4-openers.yaml --simulate
    ```

    It prints output similar to the following:

    ```
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   3    -      0       -         -       $0       -
    score   demo/enrich   3   3    -      0       -         -       $0.0300  -
    keep    sql/filter    3   1    -      0       2         -       $0       -
    opener  text/compose  1   1    -      0       -         -       $0       -
    out     csv/deliver   1   0    -      0       -         -       $0       -
    ...
    ```

    `keep` took 3, filtered 2, and passed 1, which matches the data: only Jane scores 70 or more. If a `sql/filter` row shows `out 0`, check each quoted field name in its query against the `provides:` lines plan printed; plan can't see inside the quotes. On a vendor file, simulate serves recorded responses, so these counts show the file's logic, not your real data.

1. Price a vendor file. Fetch the Apollo-to-Instantly example and plan it:

    ```sh
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/apollo-to-instantly.yaml
    gtme plan apollo-to-instantly.yaml
    ```

    Plan prices the file only after every key resolves. With them stored, it prints each step's price per record:

    ```
    ...
    1. source [source] — apollo/search@2
         est/record: $0.0000

    2. icp-filter [filter] — ai/filter@1
    ...
         est/record: ?

    3. reveal [enrich] — apollo/enrich@1
         when:      icp-filter.passed
    ...
         est/record: $0.0100

    4. linkedin [enrich] — harvest/profile@1
         when:      icp-filter.passed
    ...
         est/record: $0.0120

    5. personalize [compose] — ai/compose@1
    ...
         est/record: ?

    6. send [deliver] — instantly/add-to-campaign@1
    ...
         est/record: $0.0000
    ...
    plan ok — nothing has been spent
    ```

1. Set the ceiling with the source's `limit:`, the only spend cap gtme has. This file sets it to 500:

    ```yaml
        limit: 500
    ```

    `source` and `send` cost $0.0000, so multiply the limit by the other priced steps: 500 × ($0.0100 + $0.0120) is $11.00 of vendor credits at most. `reveal` and `linkedin` run only on records the [filter](/concepts/steps-and-roles) passes, so the real figure is usually lower. The two AI steps, `icp-filter` and `personalize`, print `est/record: ?` because model tokens are metered, and plan can't price them. `limit:` caps how many records reach them too.

    For a first run, lower `limit:` to 25, which caps vendor credits at $0.55. The runner stops the source at the cap and stops paging the vendor there (the decision record, [ADR-047](/decisions#adr-047)). The dry-run at 25 then gives you the first real number for model spend.

## What you have now

**`q4-openers.yaml` passes plan, and `q3-sent` exists in your ledger.** Your ledger has no runs and no cost rows. For the vendor file, you have a ceiling: `limit:` times the `est/record` lines, plus a model cost the first dry-run measures.

Here's what plan caught on this file and on one-line edits to it, each from a real run:

| Mistake | What plan prints | Exit |
|---|---|---|
| A field nothing upstream provides | `step "opener": needs first_name, which no earlier step provides` | 2 |
| A bad table or column in a SQL step | `step "keep": sql/filter: the query does not plan against the ledger` | 2 |
| A group your ledger doesn't have | `group "q3-sent" does not exist` | 2 |
| A misspelled [adapter](/concepts/adapter-tiers) | `step "score": adapters: unknown adapter "demo/enrcih"` | 2 |
| A `when:` naming no earlier step | `pipeline: out: when references unknown or later step "fit"` | 2 |
| A key that doesn't resolve | `step "send": missing credential INSTANTLY_API_KEY` | 3 |

**Plan passes these, so check them by hand:**

| Plan passes | Check |
|---|---|
| Who is in a group the file names | `gtme groups show NAME`, and ask who ran the last campaign |
| A misspelled field inside a SQL query's quotes | Simulate, and look for `out 0` on a `sql/filter` row |
| A `sql/filter` whose `uses:` names a field nothing upstream provides | Compare its `reads:` line with the `provides:` lines of the steps before it |
| `uses:` at a SQL step's top level instead of inside `with:` | Its `reads:` line shows `(none)`; move `uses:` into `with:` |
| A bundle folder | Plan doesn't take one; use the next section |

To have Claude Code guard the next file this way, paste this line:

```text
Guard PIPELINE, spending nothing. Plan it, or simulate it if it's a bundle folder. Fix field and query problems, but ask me before creating or changing any group, and show me who is in each group the file excludes. Simulate it and flag any sql/filter with out 0. Then give me limit times the sum of the priced est/record lines as the spend ceiling.
```

Replace `PIPELINE` with the file or bundle folder you were handed.

## If you were handed a bundle folder

**Simulate the folder from inside it, because plan takes only a file today.** `gtme run . --simulate` checks the bundle's hashes and runs every plan check except the key check. Then it runs the bundle offline and keeps nothing. In the second stage of the shipped `qualify-group-send` bundle, on a fresh ledger, it stops here:

```
bundle send (frozen from run 01M2EPQW22W25MDB9RNP8E9DTC) — hashes verified
simulate: ignoring missing credentials (step "send": missing credential INSTANTLY_API_KEY (set it in the environment or run `gtme secret set INSTANTLY_API_KEY`))
gtme: group "qualified" does not exist — create it with `gtme groups add qualified <identity-key>...` or snapshot a segment with `gtme groups add qualified --from-segment <name>`
```

The fix there is to run the first stage, which fills `qualified`.

## Next

- [`gtme run`](/reference/cli/run) lists `--dry-run`, the next rung and the first that spends.
- [`gtme groups`](/reference/cli/groups) shows, adds, and snapshots groups such as `q3-sent`.
- [`gtme plan`](/reference/cli/plan) lists every line plan prints.
