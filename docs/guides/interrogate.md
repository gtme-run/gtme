---
name: Interrogate
description: Find what gtme knows about one person, where each value came from, and why it's current, with gtme show --provenance, and fix a wrong value at its source
for: "Someone asks why a record says what it says, such as a title that doesn't match the opener you sent, and you need the answer from the ledger."
learn:
  - "how to read every current value of one record with its source, confidence, and run"
  - "how to trace a value to the pipeline and run that wrote it, and see every value the field has held"
  - "what a person's answer records beside its value, and what it was about"
  - "how to fix a wrong value where it came from, and check that gtme show wrote nothing"
order: 6
roles: [operator]
links:
  - to: /concepts/facts
    type: depends-on
    description: What source, confidence, and run mean on a fact, and the rule that picks the current value
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/ledger
    type: relates-to
    description: The field_values table and current_fields view that gtme show reads
  - to: /concepts/identity-keys
    type: relates-to
    description: The key you hand gtme show, and why any key a record has held finds it
  - to: /concepts/participants
    type: relates-to
    description: The human step in outreach.yaml, and how gtme answer records a grade with its note
  - to: /concepts/pipeline
    type: relates-to
    description: What outreach.yaml and crm.yaml are made of, a source and a list of steps
  - to: /concepts/steps-and-roles
    type: relates-to
    description: What a review's referent is, the value it was about
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The run id on every fact, and the receipt each run prints
  - to: /start/show-me
    type: relates-to
    description: Supplies the contacts.csv this guide's practice record starts from
  - to: /reference/cli/show
    type: relates-to
    description: Every form and flag of gtme show, including --run and --pending
  - to: /reference/cli/runs
    type: relates-to
    description: Lists runs with their pipeline, which names the run behind a value
  - to: /reference/cli/query
    type: relates-to
    description: Read-only SQL, used for a field's history and a referent lookup
  - to: /reference/ledger-schema/field_value_ranks
    type: relates-to
    description: The view that ranks every row of a field, which the history query reads
  - to: /reference/cli/answer
    type: relates-to
    description: How a person's answer, like the grade here, is recorded
  - to: /spec#interrogate
    type: decided-by
    description: The Interrogate story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: gtme show prints the current-value projection with provenance and must not write to the ledger
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The field_values table, the ranking view, and current_fields that gtme show reads
  - to: /decisions#adr-006
    type: decided-by
    description: gtme show is a standalone, strictly read-only inspector
  - to: /decisions#adr-003
    type: decided-by
    description: The current value is one SQL view, so gtme show and gtme query agree
  - to: /decisions#adr-049
    type: decided-by
    description: A participant's note is kept with the answer and shown by gtme show --provenance
---

# Interrogate

**Goal: everything the [ledger](/concepts/ledger) knows about one person, from one command.** You'll also trace a value to where it came from, and fix it at that source when it's wrong. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Interrogate story](/spec#interrogate):

> what the system knows about one record is always one command away.

## Before you start

**You need `gtme` ([Install](/start/install)) and a record whose [facts](/concepts/facts) came from more than one place.** On your own ledger, go straight to the steps with the person's [identity key](/concepts/identity-keys), usually their email. Otherwise, build the practice record first, and the outputs will match this page.

**This guide needs no API keys, spends nothing, and sends nothing.** Steps 1 through 5 only read the ledger. Step 6 writes one corrected fact.

## Build a practice record

Make a folder with its own ledger, so the practice stays out of your real one, and fetch a three-person CSV:

```sh
mkdir -p interrogate && cd interrogate
export GTME_LEDGER=./ledger.db
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
```

Save this [pipeline](/concepts/pipeline) in the folder as `outreach.yaml`. The `opener` step writes each person a line from a template, and `grade` asks a person to grade each title:

```yaml
name: outreach
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: opener
    use: text/compose
    uses: [full_name, title]
    provides: [opener]
    with:
      template: "Hi {{record.full_name}}, congrats on the {{record.title}} role."

  - id: grade
    use: human/review
    of: title
    uses: [full_name]
    provides:
      grade: { type: string, enum: [A, B, C] }
    with:
      prompt: never
```

Run it, grade Jane with [`gtme answer`](/reference/cli/answer), and run it again to collect the grade ([Participants](/concepts/participants) covers the waiting and collecting):

```sh
gtme run outreach.yaml
gtme answer outreach grade jane.doe@acme.com --set outreach.grade=A \
    --as grader --note "owns the budget"
gtme run outreach.yaml
```

The last run reports Jane graded and the other two still waiting:

```
grade: 3 in, 1 out — 2 awaiting human/review; `gtme answer outreach` records, the next `gtme run outreach` collects (or `gtme show --run 01M3K5QSF03CZ4DTX6XYGPFDF8 --pending grade` to read them)
```

Now a CRM export arrives with a different title for Jane. Save it, and a second pipeline that reads it as `crm.yaml`, then run that:

```sh
cat > crm-export.csv <<'EOF'
Full Name,Email,Title,Company Website
Jane Doe,jane.doe@acme.com,Marketing Manager,acme.com
EOF
```

```yaml
name: crm
version: 1

source:
  use: csv/source
  with:
    path: crm-export.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps: []
```

```sh
gtme run crm.yaml
```

Jane's record now holds values from three [adapters](/concepts/adapter-tiers), and her title disagrees with the opener the pipeline wrote for her.

## Steps

1. Ask the ledger what it knows about Jane:

    ```sh
    gtme show jane.doe@acme.com --provenance
    ```

    The output is similar to the following, trimmed with `...`:

    ```json
    {
      "entity_type": "person",
      "fields": {
    ...
        "outreach.grade": {
          "confidence": 1,
          "created_at": "2026-09-28T04:52:57.514Z",
          "note": "owns the budget",
          "referent": "01M3K5QSF2B2YSA760WXH41RKM",
          "run_id": "01M3K5QSF03CZ4DTX6XYGPFDF8",
          "source": "human/review @ grader#9bc1c6eed369",
          "value": "A"
        },
        "outreach.opener": {
          "confidence": 1,
          "created_at": "2026-09-28T04:52:57.444Z",
          "run_id": "01M3K5QSF03CZ4DTX6XYGPFDF8",
          "source": "text/compose @ #de9117fbd54a",
          "value": "Hi Jane Doe, congrats on the VP Marketing role."
        },
        "title": {
          "confidence": 1,
          "created_at": "2026-09-28T04:52:57.550Z",
          "run_id": "01M3K5QSJE484B2ZQ92KSKATGT",
          "source": "csv/source@1",
          "value": "Marketing Manager"
        }
      },
      "identity_key": "jane.doe@acme.com",
      "identity_key_tier": "email"
    }
    ```

    **Every field prints its current value with its provenance: who wrote it, how sure it was, when, and in which run.** `source` is the adapter and its version. For the grade and the opener, the hash after `#` identifies the step's definition, so an edited step gets a new hash. `grader` is who answered, and `note` is the reason they typed. Fields the file itself declared, like `outreach.grade`, carry the pipeline's name.

    The title is `Marketing Manager` because every row here has confidence 1, so the newest one wins. Facts have provenance covers when a lower confidence changes that.

1. Find the pipeline that wrote the title. Every fact carries its [run](/concepts/runs-and-receipts) id, and `gtme runs` pairs each id with its pipeline:

    ```sh
    gtme runs
    ```

    ```
    run                         pipeline  status   started                   records  in flight
    01M3K5QSJE484B2ZQ92KSKATGT  crm       done     2026-09-28T04:52:57.550Z  1        -
    01M3K5QSF03CZ4DTX6XYGPFDF8  outreach  pending  2026-09-28T04:52:57.440Z  3        2
    ```

    The title's `run_id` is the `crm` run. The `outreach` run is `pending` because Bob and Carol still wait on a grade.

1. See every value the title has held, ranked the way the ledger ranks them. This query joins each title row to its run, so each row names its pipeline:

    ```sh
    gtme query "SELECT f.value, f.source, r.pipeline, f.created_at, f.rank
                FROM field_value_ranks f JOIN runs r ON r.id = f.run_id
                WHERE f.field = 'title'
                  AND f.identity_id = (SELECT id FROM identities WHERE identity_key = 'jane.doe@acme.com')"
    ```

    ```
    {"created_at":"2026-09-28T04:52:57.550Z","pipeline":"crm","rank":1,"source":"csv/source@1","value":"\"Marketing Manager\""}
    {"created_at":"2026-09-28T04:52:57.442Z","pipeline":"outreach","rank":2,"source":"csv/source@1","value":"\"VP Marketing\""}
    2 rows
    ```

    Rank 1 is what `gtme show` printed, and the ledger kept the old row. Values print with their own quotes because the ledger stores each one as JSON. Swap `title` for any field to trace it the same way.

1. Check what the grade was about. A [review](/concepts/steps-and-roles) records its referent, the id of the exact value it judged:

    ```sh
    gtme query "SELECT f.value, r.pipeline FROM field_values f
                JOIN runs r ON r.id = f.run_id WHERE f.id = 'REFERENT'"
    ```

    Replace `REFERENT` with the grade's `referent` from step 1, here `01M3K5QSF2B2YSA760WXH41RKM`. It prints:

    ```
    {"pipeline":"outreach","value":"\"VP Marketing\""}
    1 rows
    ```

    The A was for `VP Marketing`, and so was the opener. A step that read Jane's title before the export saw the old value, and nothing reruns it by itself.

1. Before you trust the rest of the export, list every title the run wrote:

    ```sh
    gtme show --run RUN_ID --fields title
    ```

    Replace `RUN_ID` with the title's `run_id`, here `01M3K5QSJE484B2ZQ92KSKATGT`. It prints one line per record:

    ```
    {"entity_type":"person","fields":{"title":"Marketing Manager"},"identity_key":"jane.doe@acme.com","identity_key_tier":"email","state":"sourced"}
    1 record(s) from run 01M3K5QSJE484B2ZQ92KSKATGT
    ```

1. Fix the value where it came from. gtme has no command that edits a value in place, so a fix is always a new fact from a source you can name.

    If the CRM is stale and Jane is still VP Marketing, correct her in the CRM and export again. Here, edit the export and run the pipeline that reads it:

    ```sh
    sed -i.bak 's/Marketing Manager/VP Marketing/' crm-export.csv
    gtme run crm.yaml
    gtme show jane.doe@acme.com --provenance --fields title
    ```

    ```json
    {
      "entity_type": "person",
      "fields": {
        "title": {
          "confidence": 1,
          "created_at": "2026-09-28T04:53:08.796Z",
          "run_id": "01M3K5R4HVVYMHSN2HKDK3C2MH",
          "source": "csv/source@1",
          "value": "VP Marketing"
        }
      },
      "identity_key": "jane.doe@acme.com",
      "identity_key_tier": "email"
    }
    ```

    If the CRM was right, `contacts.csv` is the stale source, and the opener and the grade came from it. Correct Jane's title in `contacts.csv` too, or the next `outreach` run writes the old title back as the newest value. The `outreach` run still pending collects its answers before it reads the CSV again, so answer Bob and Carol first. The next run then prints:

    ```
    opener: 3 in, 1 out, 2 cached, 0 filtered, 0 failed
    grade: 3 in, 0 out, 2 cached, 0 filtered, 0 failed, 1 awaiting human/review
    ```

    Only Jane's input changed, so only her opener is written again, now with `Marketing Manager`, and only her grade is asked for again.

## What you have now

**Any record's current values, each traced to its adapter, run, and pipeline, and a fix made at the source.** `gtme show` only reads. To check, compare the ledger file's checksum around it:

```sh
shasum -a 256 ledger.db
gtme show jane.doe@acme.com --provenance > /dev/null
shasum -a 256 ledger.db
```

The output is similar to the following:

```
7270198106267d33af7948617ee2809135e5278733397c3f0b1fd24789ed312d  ledger.db
7270198106267d33af7948617ee2809135e5278733397c3f0b1fd24789ed312d  ledger.db
```

The two lines match, so the file didn't change by a byte ([SPEC §8](/spec#8-cli-surface--decided), and the decision record [ADR-006](/decisions#adr-006)). On Linux, use `sha256sum`.

To have Claude Code answer these questions for you, paste this line:

```text
Interrogate IDENTITY_KEY in gtme: run gtme show IDENTITY_KEY --provenance, then for FIELD name the pipeline and run that wrote the current value with gtme runs, list every value FIELD has held from field_value_ranks, and tell me which file or source to fix. Only read the ledger.
```

Replace the following:

- `IDENTITY_KEY`: the person's email, LinkedIn URL, or any key the record has held
- `FIELD`: the field someone asked about, such as `title`

## Next

- [`gtme show`](/reference/cli/show) lists every form of the command, including `--run` and `--pending`.
- [`gtme query`](/reference/cli/query) lists every flag of the command the history checks use.
- [The `field_value_ranks` view](/reference/ledger-schema/field_value_ranks) lists every column the history query reads.
