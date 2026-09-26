---
name: Runs and receipts
description: A run is an id plus the ledger rows it wrote, and its receipt is the per-step count of records and dollars that those rows back up
for: "You've run a pipeline, the step table at the end has columns you can't read yet, and you want to know what each one counts and what a run left behind."
learn:
  - "what each receipt column counts, and what estimated means on the total line"
  - "what a run writes to the ledger, and what gtme runs reads back"
  - "why a delivery never happens twice for the same key, target, and scope"
  - "where --resume picks up a run that stopped"
order: 4
roles: [operator]
defines:
  - term: "run"
    definition: "One execution of a pipeline, identified by a ULID, plus every ledger row written under that id."
  - term: "receipt"
    definition: "The table a run ends with, one row per step, counting records in, out, empty, cached, filtered, and failed, with the dollars each step spent and avoided."
  - term: "delivery"
    definition: "The row a deliver step writes when a record is sent or handed off, carrying its target, scope, idempotency key, and status."
  - term: "idempotency"
    definition: "The rule that a delivery with the same key, target, and scope as an existing row is not made again."
  - term: "resume"
    definition: "gtme run --resume, which continues a stopped run under the same id from each record's last completed step."
  - term: "ULID"
    definition: "An id that sorts by the time it was made; every run and most ledger rows have one."
links:
  - to: /concepts/ledger
    type: relates-to
    description: A run's rows live in the ledger's runs layer, and that page walks through the second run of this same file
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate and dry-run print the same receipt with records held back, and the ladder says when a run spends or sends
  - to: /concepts/steps-and-roles
    type: relates-to
    description: A step's role decides which receipt columns it can fill, such as empty for field writers and filtered for filters
  - to: /concepts/groups
    type: relates-to
    description: A suppression group is the rule for "never twice across every target", which delivery idempotency deliberately doesn't cover
  - to: /guides/recover
    type: relates-to
    description: The task version of resume, for a run that stopped partway
  - to: /guides/report
    type: relates-to
    description: Reading receipts and cost rows across runs to report on a campaign
  - to: /reference/cli/runs
    type: relates-to
    description: Every flag and output of gtme runs
  - to: /reference/cli/show
    type: relates-to
    description: Every flag and output of gtme show, including --run
  - to: /decisions#adr-053
    type: decided-by
    description: A receipt may not assert more than the run can substantiate, which is where empty and the reconciliation rule come from
  - to: /decisions#adr-046
    type: decided-by
    description: Every cost row records a measured or estimated basis, and totals print it
  - to: /decisions#adr-044
    type: decided-by
    description: Delivery dedupe is keyed by target, scope, and key, so a different campaign is a fresh decision
  - to: /decisions#adr-036
    type: decided-by
    description: A delivery is accepted until the provider attests it was sent
---

# Runs and receipts

This is `hello.yaml` from [See it run](/start/show-me), run [armed](/concepts/gate-ladder), and it spends $0 (`demo/enrich` charges a pretend $0.01 per record) and sends nothing. From the folder that has `hello.yaml` and `contacts.csv`, run it against a fresh ledger:

```sh
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run hello.yaml
```

The output is similar to the following:

```
run 01M3FBB6SRKX0B7GMSF2WMBJVW (hello)
...
run 01M3FBB6SRKX0B7GMSF2WMBJVW — done
step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
source  csv/source   0   3    -      0       -         -       $0       -
score   demo/enrich  3   3    -      0       -         -       $0.0300  -
keep    sql/filter   3   1    -      0       2         -       $0       -
out     csv/deliver  1   1    -      0       -         -       $0       -
total: $0.0300 (estimated) spent
```

The table at the end is the receipt. It and the progress lines go to the terminal's error stream, so a script or an agent reading the command's standard output gets only data ([SPEC §8](/spec#8-cli-surface--decided)).

## What you just saw

Here's the file that produced it:

```yaml
name: hello
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
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 70

  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      score: demo.score
      note: demo.note
    idempotency: email
```

**Each receipt row is one step, named by its id, and each column is a count the [ledger](/concepts/ledger) can back up.** A `-` in `empty`, `filtered`, or `failed` means zero. `cached` prints `0`, and `avoided` shows `-` when the step skipped nothing.

| Column | What it counts |
|---|---|
| `in` | Records eligible at the step, before any gate or cache. A source has nothing upstream, so its `in` is 0. |
| `out` | Records the step contributed to: it wrote a field, passed a filter, or made a delivery. |
| `empty` | Records that moved on without the step writing any field. Only steps that write fields count it. |
| `cached` | Records skipped because the ledger already had the answer: a fresh field, the same judgment, or a delivery already made. |
| `filtered` | Records a filter step stopped. |
| `failed` | Records that errored at the step. Each distinct reason prints on its own line under the table. |
| `cost` | What the step spent in this run, summed from its cost rows. |
| `avoided` | The adapter's per-record estimate for each record the step skipped. `?` means the adapter publishes no estimate. |

Every row after the source reconciles: `in` equals `out + empty + cached + filtered + failed`, plus records a `when:` gate passed over or `on_missing:` skipped. Read the `keep` row that way and you get 3 = 1 + 2. In a dry-run, the `out` row names what it held back, such as `1 held (dry run)`. The [steps and roles](/concepts/steps-and-roles) page covers which role can fill which column.

**The total line carries its basis.** `demo/enrich` multiplies a configured rate, so its dollars are estimated and the total says `(estimated)`. A vendor that reports its own charge records a measured cost, which prints bare, and a run with both splits into `$X ($Y measured + $Z estimated)`. A second run adds what the cache saved, and the ledger page walks through that receipt.

## What a run leaves behind

**A run is a ULID and the rows written under it.** A ULID is an id that sorts by the time it was made. The rows sit in the ledger's runs layer: `runs`, `run_records`, `step_events`, `costs`, and `deliveries` ([SPEC §3](/spec#3-ledger-schema--decided)). Here are this run's `run_records` rows:

```sh
gtme query "SELECT i.identity_key, r.state, r.verdicts FROM run_records r
            JOIN identities i ON i.id = r.identity_id"
```

```
{"identity_key":"jane.doe@acme.com","state":"out","verdicts":"{\"keep\":\"pass\"}"}
{"identity_key":"bob@globex.io","state":"score","verdicts":"{\"keep\":\"fail\"}"}
{"identity_key":"carol@initech.dev","state":"score","verdicts":"{\"keep\":\"fail\"}"}
3 rows
```

Jane finished `out`. Bob and Carol stopped after `score`, with a fail verdict from `keep`.

`gtme runs` lists every run, newest first:

```sh
gtme runs
```

```
run                         pipeline  status  started                   records  in flight
01M3FBB6SRKX0B7GMSF2WMBJVW  hello     done    2026-09-26T17:13:58.840Z  3        -
```

`gtme runs last` prints the same run from the ledger's side, totaled across resumes, and `gtme show --run last` prints its records as JSON, one per line.

## Delivery idempotency

**A delivery whose key, target, and scope already have a row isn't made again.** That's idempotency: doing it again has the same effect as doing it once. Here's the row this run wrote:

```sh
gtme query "SELECT target, scope, idempotency, status FROM deliveries"
```

```
{"idempotency":"jane.doe@acme.com","scope":"out.csv","status":"accepted","target":"csv/deliver"}
1 rows
```

The key is the field named by `idempotency:`, here `email`. The target is the [adapter](/concepts/adapter-tiers). The scope is the setting the adapter names: the path for `csv/deliver`, the campaign for Instantly.

A repeat is skipped with the reason `already_delivered` ([SPEC §8](/spec#deliver-idempotency)). A different path or campaign delivers again, per the decision record, [ADR-044](/decisions#adr-044). A rule that holds across every target is a suppression [group](/concepts/groups).

`accepted` means the target took the request. A delivery becomes `sent` only when the provider attests it, that is, reports back that it sent ([ADR-036](/decisions#adr-036)). A target that updates in place delivers again when the delivered values change, and `attio/assert` is the only shipped target that does ([ADR-045](/decisions#adr-045)).

## Resume

**`--resume` continues the same run from each record's last completed step.** To see it, point `out` at a folder that doesn't exist:

```yaml
  - id: out
    use: csv/deliver
    with:
      path: sent/out.csv
    variables:
      score: demo.score
      note: demo.note
    idempotency: email
```

Run it in a fresh ledger:

```sh
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run hello.yaml
```

The output is similar to the following:

```
run 01M3FBBGVWXNYMVJEQRT9039NM (hello)
...
run 01M3FBBGVWXNYMVJEQRT9039NM — failed
step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
source  csv/source   0   3    -      0       -         -       $0       -
score   demo/enrich  3   3    -      0       -         -       $0.0300  -
keep    sql/filter   3   1    -      0       2         -       $0       -
out     csv/deliver  1   0    -      0       -         1       $0       -
out: 1 failed — csv/deliver: open sent/out.csv: no such file or directory
total: $0.0300 (estimated) spent
gtme: runner: out: csv/deliver: open sent/out.csv: no such file or directory
```

Jane's `run_records` state is `keep`, the last step she completed. Make the folder and resume:

```sh
mkdir sent
gtme run hello.yaml --resume last
```

The output is similar to the following:

```
resuming run 01M3FBBGVWXNYMVJEQRT9039NM (hello)
...
run 01M3FBBGVWXNYMVJEQRT9039NM — done
step    adapter      in  out  empty  cached  filtered  failed  cost  avoided
source  csv/source   0   3    -      0       -         -       $0    -
score   demo/enrich  0   0    -      0       -         -       $0    -
keep    sql/filter   0   0    -      0       -         -       $0    -
out     csv/deliver  1   1    -      0       -         -       $0    -
total: $0 spent
```

It's the same run id, and `score` wasn't paid for twice. A rerun gets a new id and a second receipt, with the cache covering what was paid. Resume keeps one id and one `gtme runs` entry.

When a run fails, have your agent run `gtme runs last`, then `gtme run PIPELINE --resume last`, where `PIPELINE` is the pipeline file.

That's it. That's a run and its receipt.

## Why it's this way

**A receipt may not claim more than the run can prove.** `out` used to count every record that advanced, so a step that wrote nothing still reported full output. [ADR-053](/decisions#adr-053) split that into `out` and `empty` and made each row reconcile. The price is a column that's usually zero.

**Dollars say whether they were measured or estimated.** [ADR-046](/decisions#adr-046) put a basis on every cost row. Where a vendor's price depends on your plan, you set the rate, so an estimated total is only as good as that rate.

**A delivery says `accepted` until the provider proves more.** A vendor can answer "success" for a lead that was stored blank or never mailed, so the ledger records `accepted` until the provider attests the send.

**What it costs.** After a resume, the receipt counts only that invocation, so it says $0. `gtme runs` holds the whole run's cost.

## Where it shows up

- The gate ladder prints this receipt at simulate and dry-run with deliveries held back.
- [Recover](/guides/recover) is resume as a task, and [Report](/guides/report) reads receipts and cost rows across runs.
- [`gtme runs`](/reference/cli/runs) and [`gtme show`](/reference/cli/show) are the lookup pages.
