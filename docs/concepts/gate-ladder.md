---
name: The gate ladder
description: gtme runs a pipeline four ways, from simulate through plan and dry-run to armed, and each rung spends, writes, or sends more than the last
for: "You've written a pipeline and want to know what each run command will spend and send before you point it at anything real."
learn:
  - "what simulate, plan, dry-run, and armed each touch"
  - "what preflight checks before anything sends"
  - "why a dry-run spends money and the armed run after it doesn't pay again"
  - "what each rung spends and sends on a pipeline from Apollo to Instantly"
order: 3
roles: [operator, builder]
links:
  - to: /concepts/pipeline
    type: depends-on
    description: Every rung runs the same pipeline file; nothing in the file changes between rungs
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The deliver role is the one step the ladder holds back until the run is armed
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: Every rung except plan ends in a receipt, and that page reads its columns
  - to: /concepts/ledger
    type: relates-to
    description: Simulate runs on a throwaway copy of the ledger; dry-run writes facts and no deliveries; armed writes everything
  - to: /guides/connect-your-stack
    type: relates-to
    description: Walks the ladder with your own keys, from secrets through plan and dry-run to armed
  - to: /reference/cli/run
    type: relates-to
    description: The flags that pick a rung, --simulate and --dry-run, and --resume after a blocked preflight
  - to: /reference/cli/plan
    type: relates-to
    description: Every line plan prints, and the --viz diagram
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: What plan validates and prints, with no network and no spend
  - to: /decisions#adr-019
    type: decided-by
    description: Dry-run holds deliver steps and prints their resolved variables as the approval artifact
  - to: /decisions#adr-028
    type: decided-by
    description: Simulate runs the whole pipeline offline from fixtures and persists nothing
  - to: /decisions#adr-040
    type: decided-by
    description: Preflight reads the delivery target at dry-run and at the start of an armed run
---

# The gate ladder

These examples run `hello.yaml` from [A pipeline is a YAML file](/concepts/pipeline). `score` rates three fictional people at a pretend $0.01 each and caches the score for 30 days, `demo/enrich`'s default, which is why the armed run shows `cached 3`. `keep` drops anyone under 70, and `out` writes the rest to `out.csv`. In the folder with `hello.yaml` and `contacts.csv` ([See it run](/start/show-me) has the download lines), point gtme at a fresh ledger:

```sh
export GTME_LEDGER=./ladder.db
```

| Rung | Command | Spends | Ledger | Sends |
|---|---|---|---|---|
| Simulate | `gtme run hello.yaml --simulate` | No | A throwaway copy | No |
| Plan | `gtme plan hello.yaml` | No | Reads groups and SQL; writes no rows (it creates an empty ledger file if there isn't one) | No |
| Dry-run | `gtme run hello.yaml --dry-run` | Yes, on every step except deliver | Facts and costs, no deliveries | No. Preflight reads the target |
| Armed | `gtme run hello.yaml` | Yes | Everything | Yes, after preflight |

## Simulate

**Simulate runs the whole pipeline offline and keeps nothing.**

```sh
gtme run hello.yaml --simulate
```

The output is similar to the following:

```
simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
run 01M3DJMCH37P7QHXT7NNVCC0E3 (hello)
...
run 01M3DJMCH37P7QHXT7NNVCC0E3 — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
source  csv/source   0   3    -      0       -         -       $0       -
score   demo/enrich  3   3    -      0       -         -       $0.0300  -
keep    sql/filter   3   1    -      0       2         -       $0       -
out     csv/deliver  1   0    -      0       -         -       $0       -
out: resolved variables for 1 record(s) — review, then run again without --dry-run to arm:
  jane.doe@acme.com
    note: "synthetic — demo/enrich called no vendor"
    score: "100"
total: $0.0300 (estimated) spent
```

Every vendor [adapter](/concepts/adapter-tiers) answers from its fixtures (recorded sample responses). An AI step replays a recorded fixture response when one exists and otherwise returns synthetic text marked as such. `demo/enrich` isn't an AI step, and its `synthetic` note is its own.

`(estimated)` on the total means a rate was multiplied out, with nothing paid (the decision record, [ADR-046](/decisions#adr-046)). The ledger copy is discarded afterward, so the run leaves no history:

```sh
gtme runs
```

The output is the following:

```
no runs yet
```

Simulate borrows dry-run's wording for held deliveries, so `1 held (dry run)` and the hint to run again without `--dry-run` appear even though you didn't pass the flag. Don't arm after simulate; plan is next.

## Plan

**Plan checks the file and prices it, with no network and no spend.**

```sh
gtme plan hello.yaml
```

The output is the following:

```
pipeline hello (version 1)
...
2. score [enrich] — demo/enrich@1
...
     est/record: $0.0100

3. keep [filter] — sql/filter
...
     est/record: ?
...
send surface: 1 deliver step(s)
  out → csv/deliver (touch scope: hello)
...
plan ok — nothing has been spent
```

Plan checks that every field a step needs is provided upstream and that every credential resolves. It prints each step's cost per record: `?` where no estimate exists, as on `keep`, and `unset` where a step needs a rate you haven't set. Plan reads the ledger to check the [groups the file names](/concepts/groups) and the SQL in `sql/*` steps, and writes no rows. When something's wrong, plan names the step and the fix and exits with status 3.

## Dry-run

**A dry-run is a real run with every deliver step held: it spends, and it sends nothing.**

```sh
gtme run hello.yaml --dry-run
```

The output is similar to the following:

```
dry run: deliver steps will resolve and receipt their variables, but nothing sends
run 01M3DJMCKEX7Y2DKB61K2QZE08 (hello)
...
run 01M3DJMCKEX7Y2DKB61K2QZE08 — done (dry run — nothing sent)
step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
source  csv/source   0   3    -      0       -         -       $0       -
score   demo/enrich  3   3    -      0       -         -       $0.0300  -
keep    sql/filter   3   1    -      0       2         -       $0       -
out     csv/deliver  1   0    -      0       -         -       $0       -
out: resolved variables for 1 record(s) — review, then run again without --dry-run to arm:
  jane.doe@acme.com
    note: "synthetic — demo/enrich called no vendor"
    score: "100"
total: $0.0300 (estimated) spent
```

Every step before delivery runs for real: `score` spends, and its results land in the [ledger](/concepts/ledger) as facts. At `out`, the runner prints each record's resolved variables for you to review, and writes no delivery row. `gtme runs` lists the run as `done (dry)`:

```sh
gtme runs
```

The output is similar to the following:

```
run                         pipeline  status      started                   records  in flight
01M3DJMCKEX7Y2DKB61K2QZE08  cache     done (dry)  2026-09-26T00:42:50.862Z  3        -
```

**Preflight reads the delivery target before anything sends.** Plan never touches the network, so it can't know whether a campaign is paused, and preflight reads the target so it can. Each deliver adapter that supports preflight checks the live target, read-only, against what the step will send. `instantly/add-to-campaign` checks that the campaign is active and that its copy uses every variable the step sends. A dry-run prints the checks, and a blocked one fails its step before any record sends. `csv/deliver` doesn't preflight, so this run printed none.

## Armed

**Armed is the same command with no flag. It spends and sends.**

```sh
gtme run hello.yaml
```

The output is similar to the following:

```
run 01M3DJMCMTTCDTS8CP3SGYG6VN (hello)
...
out [info]: csv/deliver: wrote 1 row(s) to out.csv
...

run 01M3DJMCMTTCDTS8CP3SGYG6VN — done
step    adapter      in  out  empty  cached  filtered  failed  cost  avoided
source  csv/source   0   3    -      0       -         -       $0    -
score   demo/enrich  3   0    -      3       -         -       $0    $0.0300
keep    sql/filter   3   1    -      0       2         -       $0    -
out     csv/deliver  1   1    -      0       -         -       $0    -
total: $0 spent, $0.0300 avoided via cache (3 records skipped)
```

Read the `score` row: `cached 3`, `avoided $0.0300`. The dry-run paid for those scores, so the armed run didn't pay again.

`out` delivered Jane because the dry-run wrote no delivery row for [idempotency](/spec#deliver-idempotency) to find. Idempotency skips a key already delivered to the same target.

Nothing asks you to confirm. The dry-run's printed variables are the approval, and running the same command without the flag is the decision. There's no per-step arm: an armed run sends from every deliver step.

## On a real stack

**On `examples/apollo-to-instantly.yaml`, dry-run is the first rung that costs real money.** Plan's `est/record` for the Apollo reveal and the Harvest profile read, times the source's `limit:`, bounds the vendor spend, and the two AI steps get no estimate.

A dry-run runs every step before `send` for real. It spends Apollo credits on the reveal and model tokens on the AI filter and compose, so its receipt is the first real number for model spend. Nothing reaches Instantly, though preflight reads the campaign. Armed adds the survivors to the campaign preflight confirmed is active, and Instantly emails them on its own schedule.

That's it. That's the whole ladder.

## Why it's this way

**Delivery is the one step you can't take back, so it's the one that's gated.** Everything upstream can run again: a fact written twice is a second row, and a paid answer is remembered. [ADR-019](/decisions#adr-019) made the dry-run's printed variables the thing a person approves. [ADR-028](/decisions#adr-028) added simulate so an agent can check a pipeline offline before anyone reviews it.

**What it costs.** A dry-run spends, and seeing real vendor data always costs something. We accept that because the armed run reuses what the dry-run bought. Simulate is only as good as each adapter's fixtures: an adapter with no fixtures passes records through untouched, and the receipt counts it as a gap.

## Where it shows up

- [Connect your stack](/guides/connect-your-stack) walks the ladder with your own keys.
- [Steps and roles](/concepts/steps-and-roles) covers the deliver role, the one the ladder holds back.
- [`gtme plan`](/reference/cli/plan) and [`gtme run`](/reference/cli/run) list every flag.
