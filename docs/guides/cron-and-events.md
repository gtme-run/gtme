---
name: Run on cron and events
description: Schedule a pipeline over a CSV that a webhook receiver appends to, so each new event is handled on the next run and each person is delivered once
for: "Something outside gtme, such as a signup webhook, should set off a pipeline, and you want it to run on a schedule with nobody at a terminal."
learn:
  - "how a receiver-written CSV and a scheduled run stand in for an event trigger"
  - "why re-reading the whole file every run pays for and delivers nothing twice"
  - "what a crontab line needs that your terminal gives you for free"
  - "which exit codes a scheduler can alert on, and why a human step belongs in its own pipeline"
order: 14
roles: [builder, operator]
links:
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH, and tells you where it lives for the crontab line
  - to: /concepts/runs-and-receipts
    type: depends-on
    description: What the receipt's cached column counts, and why a delivery with the same key, target, and scope isn't made twice
  - to: /concepts/ledger
    type: relates-to
    description: The one file every scheduled run reads and writes, which is why cron must point at the same one
  - to: /concepts/identity-keys
    type: relates-to
    description: Why a receiver's duplicate row lands on the person the ledger already has
  - to: /concepts/participants
    type: relates-to
    description: Why a run with a human step ends pending, and how the next run collects instead of sourcing
  - to: /concepts/groups
    type: relates-to
    description: The group source a scheduled pipeline reads when a person reviews in a separate pipeline
  - to: /concepts/campaign-is-a-folder
    type: example-of
    description: The shipped events-cron bundle is this guide's pipeline with a model writing the note and an Instantly send
  - to: /reference/cli/run
    type: relates-to
    description: Every flag of the command the crontab line runs
  - to: /reference/cli/runs
    type: relates-to
    description: Lists each scheduled run with its status, including pending
  - to: /reference/cli/secret
    type: relates-to
    description: Writes keys to ~/.gtme/secrets, where a cron run can find them
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The event recipe, gtme's exit codes, and collect-first for pending runs
  - to: /decisions#adr-009
    type: decided-by
    description: Events arrive through a receiver you already have and a scheduled run, with no daemon
  - to: /decisions#adr-055
    type: decided-by
    description: The CSV recipe is what ships, and an NDJSON spool adapter is deferred
  - to: /decisions#adr-044
    type: decided-by
    description: Delivery dedupe is keyed by target, scope, and key, which absorbs a replayed event
  - to: /decisions#adr-038
    type: decided-by
    description: A run of a pipeline whose last run is pending resumes that run instead of sourcing
---

# Run on cron and events

**Goal: cron runs a pipeline over an events file every 15 minutes.** Each person is delivered once, however often their event arrives. Something you already have catches each event and writes it as one CSV row. Cron runs gtme over the whole file, and nothing of gtme's runs in the background waiting (the decision record, [ADR-009](/decisions#adr-009)).

The file has to be on the machine cron runs on. A hosted tool such as Zapier or Make runs elsewhere. Have it append to a file you can download, and fetch that with `curl` at the start of the cron line.

## Before you start

**You need `gtme` ([Install](/start/install)) and a machine with cron, such as macOS or Linux, that stays awake.** Cron skips runs while the machine sleeps. The next run re-reads the whole file, so nobody is lost, only late, and we'd use an always-on machine for anything promised within the hour. [Runs and receipts](/concepts/runs-and-receipts) covers the receipt columns and delivery idempotency, which this guide leans on.

**This guide needs no API keys and sends nothing past your own disk.** Every step is built in and free: `sql/filter` picks the paid trials, `text/compose` fills a template, and a [deliver](/concepts/steps-and-roles) step, `csv/deliver`, writes `welcome.csv`. The shipped `events-cron` [bundle](/concepts/campaign-is-a-folder) is the same pipeline with `ai/compose` writing the note and an Instantly send, and it needs keys to arm.

## Steps

1. Make a folder with its own [ledger](/concepts/ledger). Give the ledger an absolute path, because cron has to use the same file:

    ```sh
    mkdir -p events && cd events
    export GTME_LEDGER="$PWD/ledger.db"
    ```

    In a new terminal, export it again before any manual run, or that run uses a different ledger.

1. Save the events file. In production the receiver writes it, one row per event under a header written once:

    ```sh
    cat > events.csv <<'EOF'
    email,full_name,company_domain,event,plan,occurred_at
    jane.doe@acme.com,Jane Doe,acme.com,trial_started,team,2026-09-06T14:02:11Z
    bob@globex.io,Bob Stone,globex.io,trial_started,free,2026-09-06T14:05:40Z
    carol@initech.dev,Carol Ray,initech.dev,invited_teammate,team,2026-09-06T14:09:03Z
    EOF
    ```

    Headers that match a [canonical field](/concepts/canonical-fields) map themselves. The rest arrive as `csv.event`, `csv.plan`, and `csv.occurred_at`.

1. Save this [pipeline file](/concepts/pipeline) as `trial-welcome.yaml`:

    ```yaml
    name: trial-welcome
    version: 1

    source:
      use: csv/source
      with:
        path: events.csv

    steps:
      - id: paid-trial
        use: sql/filter
        with:
          query: |
            SELECT e.identity_id FROM current_values e
            JOIN current_values p ON p.identity_id = e.identity_id
              AND p.field = 'csv.plan' AND p.value IN ('team', 'business')
            WHERE e.field = 'csv.event' AND e.value = 'trial_started'

      - id: note
        use: text/compose
        uses: [full_name, csv.plan]
        provides: [line]
        with:
          template: "Welcome to the {{ record['csv.plan'] }} trial, {{ record.full_name }}."

      - id: welcome
        use: csv/deliver
        with:
          path: welcome.csv
        variables:
          name: full_name
          line: trial-welcome.line
        idempotency: email
    ```

    `paid-trial` keeps anyone whose event is `trial_started` on a paid plan. Each row of `current_values` is one field of one person, so checking two fields joins it to itself; change `trial_started` and the plan names for your own events. `note` renders one line per person, which the file stores as `trial-welcome.line`, and `welcome` writes it to `welcome.csv`, keyed on `email`.

1. Run it:

    ```sh
    gtme run trial-welcome.yaml
    ```

    The output is similar to the following:

    ```
    ...
    run 01M3MGDV591CMVPNZV81R1GX4T — done
    step        adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source      csv/source    0   3    -      0       -         -       $0    -
    paid-trial  sql/filter    3   1    -      0       2         -       $0    -
    note        text/compose  1   1    -      0       -         -       $0    -
    welcome     csv/deliver   1   1    -      0       -         -       $0    -
    total: $0 spent
    ```

    Bob is on the free plan and Carol invited a teammate, so `paid-trial` filtered both. Jane is in `welcome.csv`.

1. Append two events the way the receiver would, and run again. The second Jane row is a receiver retry, the same event delivered twice:

    ```sh
    cat >> events.csv <<'EOF'
    dave@newco.example,Dave Kim,newco.example,trial_started,business,2026-09-06T14:11:57Z
    jane.doe@acme.com,Jane Doe,acme.com,trial_started,team,2026-09-06T14:02:11Z
    EOF
    gtme run trial-welcome.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3MGDV6BWJBVEYJGS7TTEBCC (trial-welcome)
    source [info]: read 5 rows from events.csv
    source: sourced 4 records (1 already in the ledger)
    ...
    step        adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source      csv/source    0   4    -      0       -         -       $0    -
    paid-trial  sql/filter    4   2    -      0       2         -       $0    -
    note        text/compose  2   1    -      1       -         -       $0    ?
    welcome     csv/deliver   2   1    -      0       -         -       $0    -
    welcome: 1 already delivered
    total: $0 spent, $0.0000+? avoided via cache (1 records skipped)
    ```

    Five rows became four records, because Jane's second row carries her [identity key](/concepts/identity-keys) and lands on the person the ledger already has. `note` counts Jane as `cached`, and `welcome: 1 already delivered` is Jane too, so only Dave is new.

1. Check the `deliveries` table, one row per delivery made:

    ```sh
    gtme query "SELECT idempotency, run_id FROM deliveries ORDER BY created_at"
    ```

    ```
    {"idempotency":"jane.doe@acme.com","run_id":"01M3MGDV591CMVPNZV81R1GX4T"}
    {"idempotency":"dave@newco.example","run_id":"01M3MGDV6BWJBVEYJGS7TTEBCC"}
    2 rows
    ```

    Each person has one row, from the run that first saw them. A replayed event finds Jane's row and makes no second one (the decision record, [ADR-044](/decisions#adr-044)).

1. Add this line to your crontab with `crontab -e` (set `EDITOR=nano` first if you'd rather not use vi):

    ```
    */15 * * * * cd EVENTS_DIR && GTME_LEDGER=EVENTS_DIR/ledger.db lockf -t 0 EVENTS_DIR/gtme.lock PATH_TO_GTME run trial-welcome.yaml >> EVENTS_DIR/gtme.log 2>&1 || NOTIFY_COMMAND
    ```

    Replace the following:

    - `EVENTS_DIR`: the absolute path of this folder, which `pwd` prints
    - `PATH_TO_GTME`: the absolute path of `gtme`, which `command -v gtme` prints
    - `NOTIFY_COMMAND`: a command that tells you a run failed, such as a health-check service's `curl` ping or `mail` if the machine sends email

    `*/15 * * * *` means every 15 minutes; the five fields are minute, hour, day, month, and weekday, so `0 * * * *` is hourly. On Linux, use `flock -n EVENTS_DIR/gtme.lock` in place of `lockf -t 0 EVENTS_DIR/gtme.lock`. On macOS, keep the folder outside Documents, Desktop, and Downloads, which cron can't read by default.

    Cron runs the line with `/bin/sh`, your home directory, and a `PATH` of `/usr/bin:/bin`, and it doesn't read your shell profile. Here's what each part of the line is for:

    - A bare `gtme` fails with `/bin/sh: gtme: command not found` and exit code 127, because Homebrew and `go install` both put `gtme` outside that `PATH`.
    - Without `GTME_LEDGER`, the run uses `~/.gtme/ledger.db`, which has no history of this campaign. A run against it delivers Jane and Dave a second time.
    - A key exported in `~/.zshrc` isn't there. gtme looks in the environment first and then in `~/.gtme/secrets`, which [`gtme secret set`](/reference/cli/secret) writes and cron can read. A missing key stops the run before it spends, with exit code 3.
    - `>> gtme.log 2>&1` keeps every receipt, which gtme writes to the error stream.
    - `lockf` skips a run while the last one is still going, exiting 75. gtme doesn't stop two runs of one pipeline from overlapping, and two overlapping runs can both send to the same new person, because each checks for a delivery before it sends and records it after. The lock also keeps two first runs from racing to create a new ledger.
    - Cron itself alerts nobody, so `|| NOTIFY_COMMAND` runs whenever the line exits non-zero.

1. Test the line the way cron runs it: a new event, then the same command from another directory. `env -i` starts a shell with none of your settings, the way cron does:

    ```sh
    cat >> events.csv <<'EOF'
    erin@umbrella.co,Erin Lee,umbrella.co,trial_started,team,2026-09-06T15:20:44Z
    EOF
    cd /
    env -i HOME="$HOME" PATH=/usr/bin:/bin /bin/sh -c 'cd EVENTS_DIR &&
        GTME_LEDGER=EVENTS_DIR/ledger.db lockf -t 0 EVENTS_DIR/gtme.lock \
        PATH_TO_GTME run trial-welcome.yaml >> EVENTS_DIR/gtme.log 2>&1'
    echo $?
    ```

    ```
    0
    ```

    Then read the log:

    ```sh
    cat EVENTS_DIR/gtme.log
    ```

    The output is similar to the following:

    ```
    run 01M3MGDV7SBWHX3Z6FHP0EA8HG (trial-welcome)
    source [info]: read 6 rows from events.csv
    source: sourced 5 records (1 already in the ledger)
    ...
    step        adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source      csv/source    0   5    -      0       -         -       $0    -
    paid-trial  sql/filter    5   3    -      0       2         -       $0    -
    note        text/compose  3   1    -      2       -         -       $0    ?
    welcome     csv/deliver   3   1    -      0       -         -       $0    -
    welcome: 2 already delivered
    total: $0 spent, $0.0000+? avoided via cache (2 records skipped)
    ```

    Erin is the one new delivery. Jane and Dave are already delivered, and their lines are cached.

## When a scheduled run fails or waits

**Your notifier sees the exit code, and gtme's are fixed ([SPEC §8](/spec#8-cli-surface--decided)).** `75` comes from `lockf`, and means the last run was still going.

| Exit code | Means |
|---|---|
| `0` | The run finished, or ended `pending` |
| `1` | Any other error |
| `2` | Validation or contract error |
| `3` | Auth or credential error |
| `4` | Rate-limited |
| `5` | Network error |

If the events file is missing, the run exits 2 before it sources anything:

```
gtme: step "source": csv/source: opening events.csv: open events.csv: no such file or directory
```

When the file gets slow to re-read, cut it back to its header line rather than deleting it; a missing file fails every run. The `deliveries` table still holds who was welcomed, so an event replayed after the cut delivers nothing. Do it between runs, and have the receiver pause, or an event written in between is lost.

**A pending run exits 0, so check for it separately.** A run is `pending` when records are waiting on a person or an in-flight batch ([participants](/concepts/participants) covers both), and [`gtme runs`](/reference/cli/runs) shows it. A second cron line can count them and notify you when the count isn't 0:

```sh
gtme query "SELECT count(*) AS pending FROM runs
            WHERE pipeline = 'trial-welcome' AND status = 'pending'"
```

```
{"pending":0}
1 rows
```

**Keep human steps out of the scheduled pipeline.** To see why, copy `trial-welcome.yaml` to `with-review.yaml` and add this step between `paid-trial` and `note`:

```yaml
  - id: approve
    use: human/filter
    uses: [full_name, csv.plan]
```

`gtme plan with-review.yaml` then prints this note:

```
note: under cron this pipeline waits for a person: "welcome" follows the human/filter step "approve", and a pending run is resumed, not re-sourced, until every record is answered. The pattern: review into a group in one pipeline, send from the group in another.
```

Cron has no terminal, so every record the step judges waits, and the run ends `pending`. The next scheduled run resumes that run and doesn't read `events.csv`, so new events wait with it ([ADR-038](/decisions#adr-038)). Instead, put the review in its own pipeline, one a person runs that ends with `group: approved`. Then make that group the scheduled pipeline's source:

```yaml
source:
  group: approved
  once: true
```

`once: true` means each run picks up only the newly approved ([groups](/concepts/groups)). Run the reviewing pipeline once before you schedule this one, because a source group that doesn't exist yet fails [plan](/concepts/gate-ladder) with exit code 2.

## What you have now

**A pipeline cron can run every 15 minutes, with a log of every receipt and a ledger that delivers each person once.** List the runs:

```sh
gtme runs
```

```
run                         pipeline       status  started                   records  in flight
01M3MGDV7SBWHX3Z6FHP0EA8HG  trial-welcome  done    2026-09-28T17:19:00.345Z  5        -
01M3MGDV6BWJBVEYJGS7TTEBCC  trial-welcome  done    2026-09-28T17:19:00.299Z  4        -
01M3MGDV591CMVPNZV81R1GX4T  trial-welcome  done    2026-09-28T17:19:00.265Z  3        -
```

To swap in a model and a real send, start from the `events-cron` bundle, and set its keys with `gtme secret set` so cron finds them. To have your agent set this up, paste this into Claude Code:

```text
In EVENTS_DIR, adapt trial-welcome.yaml's paid-trial filter to my events file, run it once by hand with GTME_LEDGER=EVENTS_DIR/ledger.db, then show me the crontab line with lockf and a failure notifier, and test it with env -i from / before I install it.
```

Replace `EVENTS_DIR` with the absolute path of your events folder.

## Next

- [Your stack](/start/my-stack) arms a pipeline on your own keys, the step between this guide and the `events-cron` bundle.
- [The `deliveries` table](/reference/ledger-schema/deliveries) lists the target, scope, and key that keep a replayed event from delivering twice.
- [`gtme run`](/reference/cli/run) lists every flag the crontab line can take.
