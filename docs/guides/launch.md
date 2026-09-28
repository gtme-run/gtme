---
name: Launch
description: Run a finished pipeline on people your ledger has never seen, with plan and the dry-run as the go/no-go, then check the ledger knows every one
for: "Someone handed you a finished pipeline or a bundle folder, and you're about to run it on fresh data for the first time."
learn:
  - "what to read in plan and the dry-run receipt before you launch"
  - "what the launch run leaves in gtme runs"
  - "how to check that the ledger knows every person the run sourced, delivered or not"
order: 3
roles: [operator]
links:
  - to: /concepts/gate-ladder
    type: depends-on
    description: Plan and the dry-run are the rungs you read before the launch, which is the armed run
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /start/show-me
    type: relates-to
    description: Runs the same hello.yaml to show a receipt and a second, cheaper run
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: What each column of the launch receipt counts, and what a run leaves in the ledger
  - to: /concepts/ledger
    type: relates-to
    description: The identities and field_values tables the last two steps read
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: What a bundle folder carries and why gtme run checks its hashes before it runs
  - to: /start/my-stack
    type: relates-to
    description: The same climb on a live Apollo-to-Instantly pipeline, where the dry-run spends real money
  - to: /start/for-agents
    type: relates-to
    description: The rules an agent follows before it arms, which the paste line on this page relies on
  - to: /concepts/pipeline
    type: relates-to
    description: What the file you were handed declares, step by step
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The deliver role is the step the dry-run holds and the launch arms
  - to: /concepts/facts
    type: relates-to
    description: Which source and run wrote each field that gtme show prints
  - to: /concepts/groups
    type: relates-to
    description: Pins the exact list a dry-run showed, so a launch on a live source sends to no one unread
  - to: /reference/cli/query
    type: relates-to
    description: Every flag of the command the ledger check uses
  - to: /reference/cli/run
    type: relates-to
    description: Every flag of the launch command, including --resume and --dry-run
  - to: /spec#launch
    type: decided-by
    description: The Launch story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The terminal receipt, gtme runs, and a dry-run as its own run with no deliveries
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The identities, field_values, and run_records tables the ledger check reads
  - to: /decisions#adr-019
    type: decided-by
    description: The dry-run's resolved variables are the approval artifact read before arming
  - to: /decisions#adr-029
    type: decided-by
    description: A bundle is a portable folder that gtme run accepts in place of a pipeline file
---

# Launch

**Goal: a `done` run of a pipeline you didn't write, and a [ledger](/concepts/ledger) that now knows every person it sourced.** SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Launch story](/spec#launch):

> starting a new pipeline against fresh data produces a receipted run and a ledger that now knows those identities.

## Before you start

**You need `gtme` and a pipeline with its input.** [Install](/start/install) covers the first. This guide launches `hello.yaml`, the three-person file from [See it run](/start/show-me), because it runs with no keys. A real [pipeline](/concepts/pipeline) has more steps and bigger numbers, and you read them the same way.

**This guide needs no keys and sends nothing past your own disk.** Its receipts show $0.03 spent, and that money is pretend: `demo/enrich` prices itself at $0.01 per record and calls no vendor. The one [deliver](/concepts/steps-and-roles) step writes to `out.csv` in your folder.

**If you were handed a [bundle](/concepts/campaign-is-a-folder) folder, run the steps from inside it with `.` in place of the file name.** `gtme run` checks the bundle's hashes before it reads a record. Every shipped bundle calls a model or a vendor, so its armed run needs a key, and [Your stack](/start/my-stack) shows how to store one.

## Steps

1. Make a folder with its own ledger, and fetch the pipeline and its input:

    ```sh
    mkdir -p launch && cd launch
    export GTME_LEDGER=./ledger.db
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/hello.yaml
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
    ```

    On a real launch, leave `GTME_LEDGER` unset, and run `unset GTME_LEDGER` if you exported it earlier, because your main ledger is the one that should learn these people. Here it keeps the practice launch out of your main ledger.

    Here's the file you were handed, without its comments. In a file you didn't write, read the source, the filter's threshold, and the deliver target first:

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

1. Check that the data is fresh. Ask the ledger for one person from the CSV:

    ```sh
    gtme show jane.doe@acme.com
    ```

    It prints:

    ```
    gtme: no identity known by key "jane.doe@acme.com"
    ```

    This checks one person, so treat it as a spot check; with a search source, check one person from the dry-run's list instead. If `gtme show` prints a record, an earlier run already sourced this person. That's not a stop: the launch pays only for what's missing or past its cache window, and `idempotency: email` means no one already delivered to this target gets it twice.

1. Plan it. [Plan](/concepts/gate-ladder), the first rung of the gate ladder, reads the file with no network and spends nothing. With a bundle, `gtme plan` doesn't accept the folder yet, so run `gtme run . --simulate`, an offline rehearsal that spends and keeps nothing, and read its problems and resolved variables instead:

    ```sh
    gtme plan hello.yaml
    ```

    It prints:

    ```
    pipeline hello (version 1)
    ...
    2. score [enrich] — demo/enrich@1
    ...
         cache:     30d
         est/record: $0.0100
    ...
    send surface: 1 deliver step(s)
      out → csv/deliver (touch scope: hello)
    ...
    plan ok — nothing has been spent
    ```

    Read three lines. `plan ok` means every field and credential the file needs resolves. Multiply each `est/record` by your row count to get the most that step spends: here $0.0100 times 3 rows, so $0.03. A `?` means the step is unpriced or metered, and the dry-run receipt shows its cost.

    `send surface` lists every step that sends when you launch. Here that's `out`, and its target is a file. A step on that list you didn't expect is a no-go, so ask whoever handed you the file.

1. Dry-run it. This spends the pretend $0.03 and sends nothing. On a pipeline with vendors, this is the rung that spends real money:

    ```sh
    gtme run hello.yaml --dry-run
    ```

    The output is similar to the following:

    ```
    dry run: deliver steps will resolve and receipt their variables, but nothing sends
    ...
    run 01M3K26QK8Y3VJZP3CX0T6G5XC — done (dry run — nothing sent)
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

    This receipt is the go/no-go. The `source` row's `out` should match the rows you were handed, and the total should sit inside plan's bound. `filtered 2` on `keep` should be a number you can explain: Bob and Carol scored under 70.

    Then read each resolved variable, because that's exactly what the launch writes. The decision record, [ADR-019](/decisions#adr-019), makes these variables the thing you approve. On a live target such as an Instantly campaign, the dry-run also prints a `preflight` line. It checks that the campaign is active and uses every variable, and a blocked check is a no-go.

1. Launch it. This is the armed run, the command with no flag. It spends only on what the ledger doesn't already have, and it writes Jane to `out.csv`:

    ```sh
    gtme run hello.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3K26QM67AH93C1PDRTQ6R4P (hello)
    ...
    out [info]: csv/deliver: wrote 1 row(s) to out.csv
    ...
    run 01M3K26QM67AH93C1PDRTQ6R4P — done
    step    adapter      in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source   0   3    -      0       -         -       $0    -
    score   demo/enrich  3   0    -      3       -         -       $0    $0.0300
    keep    sql/filter   3   1    -      0       2         -       $0    -
    out     csv/deliver  1   1    -      0       -         -       $0    -
    total: $0 spent, $0.0300 avoided via cache (3 records skipped)
    ```

    That's the [receipt](/concepts/runs-and-receipts) the invariant asks for: status `done`, 3 records sourced, and a total with its cost. `score` shows `cached 3` because the dry-run already paid for those scores. `out 1` is the delivery the dry-run held.

    The source read the CSV again. A live source, such as a CRM search, can return people between the dry-run and the launch that you never read, and a [group](/concepts/groups) pins the list you dry-ran. If the receipt says `failed` instead, it names the step and the reason. Fix that, then finish the same run with `gtme run hello.yaml --resume RUN_ID`, using the id on the receipt's first line.

1. Check that the ledger knows every person the launch sourced. This query lists each identity in the newest run, the last step it finished, and how many fields the ledger holds for it:

    ```sh
    gtme query "SELECT i.identity_key, r.state,
                count(DISTINCT f.field) AS fields
                FROM run_records r
                JOIN identities i ON i.id = r.identity_id
                LEFT JOIN field_values f ON f.identity_id = i.id
                WHERE r.run_id = (SELECT max(id) FROM runs)
                GROUP BY i.id"
    ```

    It prints:

    ```
    {"fields":6,"identity_key":"jane.doe@acme.com","state":"out"}
    {"fields":6,"identity_key":"bob@globex.io","state":"score"}
    {"fields":6,"identity_key":"carol@initech.dev","state":"score"}
    3 rows
    ```

    There's one row per CSV row, and each person has six fields: four from the CSV and two from `score`. A `0` in `fields`, or fewer rows than the `source` row's `out`, would mean the ledger missed someone. Bob and Carol stopped at `score`, since `keep` filtered them, and the ledger kept their scores anyway, so `gtme show bob@globex.io` prints all six of his fields.

## What you have now

**A launched pipeline: one `done` run with a receipt, and a ledger that knows all three people from the CSV.** `out.csv` holds the one person the pipeline delivered:

```sh
cat out.csv
```

```
identity_key,note,score
jane.doe@acme.com,synthetic — demo/enrich called no vendor,100
```

To have Claude Code launch the next file this way, paste this line. The agent climbs to the dry-run and leaves an armed run on a live target to you ([For agents](/start/for-agents)):

```text
Launch PIPELINE: check with gtme show that a sourced person is new, plan it, dry-run it, then stop and show me the dry-run receipt. After I arm it, confirm with gtme runs and gtme query that the ledger knows every sourced person.
```

Replace `PIPELINE` with the file or bundle folder you were handed.

## Next

- [`gtme run`](/reference/cli/run) lists every flag of the launch command, including `--resume`.
- [Facts have provenance](/concepts/facts) explains which source and run wrote each field `gtme show` prints.
- [`gtme query`](/reference/cli/query) lists every flag of the command the ledger check used.
