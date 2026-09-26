---
name: Your CSV
description: Your own CSV of people, judged by an AI filter, given two intro lines each, and written to a local CSV, for one model key and cents on the model
for: "You have a CSV of people and an Anthropic key, and you want them judged and written with your own prompts."
learn:
  - "map CSV columns to gtme's field names"
  - "plan and simulate before spending anything"
  - "run armed and read what the model cost"
  - "change a prompt and re-run"
order: 3
roles: [operator, builder]
links:
  - to: /start/show-me
    type: relates-to
    description: See it run has a pipeline of the same shape with no key and no CSV of your own
  - to: /guides/iterate
    type: relates-to
    description: After the first armed run, the next job is tuning the filter prompt and running again
  - to: /start/my-stack
    type: relates-to
    description: Your stack swaps the local CSV for vendor keys and a real delivery target
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate and plan are the $0 rungs this page climbs before the armed run
  - to: /concepts/ledger
    type: relates-to
    description: The armed run writes every judgment to the ledger, which is why a second run re-judges nobody
  - to: /concepts/canonical-fields
    type: relates-to
    description: The columns mapping ties your CSV headers to the canonical names every step reads
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: What each column of the receipt counts
  - to: /decisions#adr-039
    type: decided-by
    description: Why an AI step skips a record it already judged with the same prompt and the same inputs
  - to: /decisions#adr-018
    type: decided-by
    description: Why CSV headers are mapped once at the source and every later step reads canonical names
---

# Your CSV

**This needs a CSV of people with a header row, and an Anthropic API key.** If you haven't seen a pipeline run yet, [See it run](/start/show-me) needs neither.

**It spends cents on the model for the armed run, and $0 to simulate and plan.** Nothing sends, because the delivery target is a file on your machine. Each row's name, title, and company domain go to Anthropic with your prompts for the filter and compose steps. Nothing else leaves the machine.

Make a new folder with its own ledger, so nothing from See it run counts as already delivered, and fetch the pipeline with its three sample people:

```sh
mkdir -p gtme-mine && cd gtme-mine
export GTME_LEDGER=./ledger.db
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/my-csv.yaml
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
```

## The file you fetched

**`my-csv.yaml` reads a CSV, keeps the people a prompt says fit, writes two lines for each, and appends them to `out.csv`.** Here it is with the comments stripped:

```yaml
name: my-csv
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns:
      full_name: Full Name
      email: Email
      title: Title
      company_domain: Company Website

steps:
  - id: fit
    use: ai/filter
    uses: [full_name, title, company_domain]
    with:
      template: >
        Keep people who plausibly own outbound tooling decisions.

  - id: lines
    use: ai/compose
    when: fit.passed
    uses: [full_name, title, company_domain]
    with:
      template: >
        Write first_line and ps_line for a short, honest intro email.

  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      first_line: first_line
      ps_line: ps_line
    idempotency: email
```

`columns:` maps each [canonical field](/concepts/canonical-fields), the standard name every step reads, to your header for it. It already matches `contacts.csv`, so run it as shipped first.

## Run the sample

1. Simulate it for $0:

    ```sh
    gtme run my-csv.yaml --simulate
    ```

    The output is similar to the following:

    ```
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    ...
    out: 3 in, 0 out, 0 cached, 0 filtered, 0 failed, 3 held (dry run)
    ...
    out: resolved variables for 3 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        first_line: "Fixture first line for jane.doe@acme.com"
        ps_line: "Fixture ps line for jane.doe@acme.com"
    ...
    total: $0 (estimated) spent
    ```

    [Simulate](/concepts/gate-ladder) answers the AI steps from fixtures, canned answers that say so. It holds delivery the same way a dry-run does, and "held (dry run)" is the CLI's wording for that. The resolved variables are the values each kept row would get.

1. Store your key. `gtme` prompts for it, so the key never appears on the command line:

    ```sh
    gtme secret set ANTHROPIC_API_KEY
    ```

1. Plan:

    ```sh
    gtme plan my-csv.yaml
    ```

    A missing key is a warning at plan time, not an error. Without one, each AI step shows the following:

    ```
         warning:   optional credential ANTHROPIC_API_KEY is not set; this step will fail at run time if it needs it
         warning:   optional credential ANTHROPIC_WORKSPACE_ID is not set; this step will fail at run time if it needs it
         est/record: ?
    ```

    The `ANTHROPIC_WORKSPACE_ID` warning stays; ignore it unless your key is workspace-scoped. Plan prints a cost per record when a step declares one. The `?` means the model cost is metered, so the receipt is where you see it.

1. Run it for real. This run is armed. It spends on the model for `fit` and `lines`, and it sends nothing.

    ```sh
    gtme run my-csv.yaml
    ```

    ```
    run 01M3D1R0CE22N0DJ4DCTFC0R7D (my-csv)
    source [info]: read 3 rows from contacts.csv
    source: sourced 3 records
    fit: 3 in, 3 out, 0 cached, 0 filtered, 0 failed
    lines: 3 in, 3 out, 0 cached, 0 filtered, 0 failed
    ...
    run 01M3D1R0CE22N0DJ4DCTFC0R7D — done
    step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source   0   3    -      0       -         -       $0       -
    fit     ai/filter    3   3    -      0       -         -       $0.0027  -
    lines   ai/compose   3   3    -      0       -         -       $0.0051  -
    out     csv/deliver  3   3    -      0       -         -       $0       -
    total: $0.0078 (estimated) spent
    ```

    Read `cost` on the `fit` and `lines` rows for what the model cost: under a cent for three people. On the `out` row, `out` counts the rows written to `out.csv`. All three passed the filter this time; the ledger keeps the verdict either way.

1. Run it again:

    ```sh
    gtme run my-csv.yaml
    ```

    ```
    run 01M3D1R9N6HMB6TF7K2NNCERN3 (my-csv)
    source [info]: read 3 rows from contacts.csv
    source: sourced 3 records
    fit: 3 in, 0 out, 3 cached, 0 filtered, 0 failed
    lines: 3 in, 0 out, 3 cached, 0 filtered, 0 failed
    out: 3 in, 0 out, 3 cached, 0 filtered, 0 failed

    run 01M3D1R9N6HMB6TF7K2NNCERN3 — done
    step    adapter      in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source   0   3    -      0       -         -       $0    -
    fit     ai/filter    3   0    -      3       -         -       $0    ?
    lines   ai/compose   3   0    -      3       -         -       $0    ?
    out     csv/deliver  3   0    -      3       -         -       $0    $0.0000
    total: $0 spent, $0.0000+? avoided via cache (9 records skipped)
    ```

    On both AI rows, `cached` matches `in` and the cost is $0, because every judgment is in [the ledger](/concepts/ledger). The `?` under `avoided` is the model's metered price, which the receipt can't restate; the first receipt already told you it was $0.0078. The `out` row writes nothing twice.

## Run your file

**We recommend 10 rows first.** Trim your CSV, then point `path:` at the sample:

```sh
head -n 11 CSV_FILE > sample.csv
```

Replace `CSV_FILE` with your file's name.

Edit `columns:` for your headers, rewrite both prompts, then simulate, plan, and run. A header like "Full Name" maps itself to `full_name`. The three fields in `uses:` must be mapped or plan stops, naming the field and listing the headers it found. A blank cell still goes to the model, and the [receipt](/concepts/runs-and-receipts) counts it, such as `1 missing title`. Any other header is kept as `csv.<header>`.

When the sample reads right, set `path:` to the full file. The full run skips the 10 rows it already judged, and `out.csv` gets none of them twice.

## Questions after the first run

- **Does a new prompt re-judge?** Yes, because a changed `template:` changes the step's judgment signature, and the cache matches only the same prompt on the same inputs.
- **What's in `out.csv`?** It holds the kept rows only: `identity_key`, `first_line`, and `ps_line`. Add `full_name: full_name` under `variables:` to carry the name.
- **Why was someone filtered out?** `gtme show KEY` prints the fields, not the verdict. The model's reason is in the ledger:

    ```sh
    gtme query "SELECT i.identity_key, e.detail ->> 'reason' AS reason
      FROM step_events e JOIN identities i ON i.id = e.identity_id
      WHERE e.step_id = 'fit' AND e.event = 'done' AND e.detail ->> 'pass' = 0"
    ```

## Next

**[Iterate](/guides/iterate) covers tuning the filter and running again.** If your next step is your vendors, [Your stack](/start/my-stack) swaps the CSV for them.
