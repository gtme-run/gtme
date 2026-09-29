---
name: Recover
description: Finish a run that died partway, under its own id, without paying twice for any step, and decide what happens to the few sends it can't confirm
for: "A run stopped before its receipt, because a laptop closed, a terminal was killed, or a vendor went down, and you want it finished without paying or sending twice."
learn:
  - "how to tell that a run died, and how far each record got"
  - "how to resume a run by its id"
  - "which records a deliver step holds after a crash, and how to settle or release them"
  - "how to prove from the ledger that no step was paid for twice"
order: 9
roles: [operator]
links:
  - to: /concepts/runs-and-receipts
    type: depends-on
    description: What a run leaves in the ledger, what resume does, and what the receipt columns count
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/ledger
    type: relates-to
    description: The run_records, step_events, costs, and deliveries tables the steps read
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The deliver role is the step whose sends this guide checks
  - to: /concepts/pipeline
    type: relates-to
    description: What the practice file declares, step by step
  - to: /concepts/gate-ladder
    type: relates-to
    description: The practice run is armed, and the ladder says what an armed run spends and sends
  - to: /reference/cli/run
    type: relates-to
    description: Every flag of gtme run, including --resume
  - to: /reference/cli/runs
    type: relates-to
    description: Every flag and output of gtme runs, the command that shows a run's status and per-step counts
  - to: /reference/ledger-schema/run_records
    type: relates-to
    description: The table that holds each record's last completed step, which is what resume reads
  - to: /reference/ledger-schema/deliveries
    type: relates-to
    description: The table a deliver step checks before it sends, where a held record's row says unconfirmed
  - to: /spec#recover
    type: decided-by
    description: The Recover story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The run_records, step_events, and costs tables that resume and the proof read
  - to: /spec#deliver-idempotency
    type: decided-by
    description: A deliver step writes its delivery row after the target responds, holds a record that was in flight at a crash, and sends it again only on release
  - to: /spec#interrupted-runs--the-run-lock-adr-061
    type: decided-by
    description: A running run holds a lock, so a dead one reads as interrupted and a live one can't be resumed
  - to: /decisions#adr-002
    type: decided-by
    description: Steps read from and write to the ledger, which is where resume comes from
  - to: /decisions#adr-060
    type: decided-by
    description: A delivery in flight at a crash is held, not sent again
  - to: /decisions#adr-061
    type: decided-by
    description: A run holds a lock while it executes, and a dead running run reads as interrupted
---

# Recover

**Goal: a run that died partway reaches `done` under its own id, with no step paid for twice.** Resume holds the few records that were mid-send when it died instead of sending them again, and you check the target to decide whether they need a second send. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Recover story](/spec#recover):

> a killed run picks up where it left off, never redoing completed, paid-for work.

## Before you start

**You need `gtme` ([Install](/start/install)) and a run that died or failed partway.** [Runs and receipts](/concepts/runs-and-receipts) covers what a run leaves in the [ledger](/concepts/ledger) and what `--resume` does. Work in the ledger the run used: if it set `GTME_LEDGER`, set it again in your new terminal. On your own dead run, go straight to the steps. To practice, you also need `python3`, which runs a stand-in CRM so you can see what the target received, and the outputs will match this page.

**This guide needs no API keys, and its practice run spends a pretend $0.12 and sends only to your own machine.** `demo/enrich` charges a pretend $0.01 per record and calls no vendor. The one [deliver](/concepts/steps-and-roles) step posts each lead to a small server on `127.0.0.1`. When you resume, the records that were mid-send at the crash are held rather than sent again, and step 4 checks the target for them.

## Build a practice history

**The practice file scores 12 trial signups and posts each one to a stand-in CRM.** Make a folder with its own ledger:

```sh
mkdir -p recover && cd recover
export GTME_LEDGER=./ledger.db
```

Save this as `signups.csv`:

```
Full Name,Email,Title,Company Website
Jane Doe,jane.doe@acme.com,VP Marketing,acme.com
Bob Stone,bob@globex.io,Head of Growth,globex.io
Carol Diaz,carol@initech.dev,RevOps Lead,initech.dev
Dana Park,dana@contoso.com,CMO,contoso.com
Eli Moss,eli@umbrella.co,Growth Manager,umbrella.co
Fay Chen,fay@wayne.io,Demand Gen Lead,wayne.io
Gus Lee,gus@hooli.com,VP Sales,hooli.com
Hana Sato,hana@stark.io,Marketing Ops,stark.io
Ivan Roy,ivan@soylent.co,Head of Sales,soylent.co
Jo Kim,jo@vandelay.com,SDR Manager,vandelay.com
Kai Ward,kai@tyrell.io,CRO,tyrell.io
Lena Ruiz,lena@cyberdyne.dev,RevOps Manager,cyberdyne.dev
```

Save this [pipeline](/concepts/pipeline) as `signups.yaml`:

```yaml
name: signups
version: 1

source:
  use: csv/source
  with:
    path: signups.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: score
    use: demo/enrich
    with:
      cost_per_record_usd: 0.01

  - id: crm
    use: http/deliver
    with:
      url: http://127.0.0.1:8765/leads
    variables:
      email: email
      full_name: full_name
      score: demo.score
    idempotency: email
```

`idempotency: email` tells `crm` to skip anyone whose email it has already delivered to this target.

Save the stand-in CRM as `target.py`. It takes a second per lead, so the run is slow enough to kill, and it logs each email it receives to `received.log`:

```python
import http.server, json, time

class Target(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        lead = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        time.sleep(1)
        with open("received.log", "a") as log:
            log.write(lead["email"] + "\n")
        try:
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"{}")
        except (BrokenPipeError, ConnectionResetError):
            pass  # the run died before it heard back

    def log_message(self, *args):
        pass

http.server.ThreadingHTTPServer(("127.0.0.1", 8765), Target).serve_forever()
```

In a second terminal, in the same folder, start it and leave it running:

```sh
python3 target.py
```

Back in the first terminal, crash the run. This starts it [armed](/concepts/gate-ladder), waits two seconds, and kills the process the way a killed terminal or a crashed machine would:

```sh
gtme run signups.yaml & sleep 2; kill -9 $!
```

The output is similar to the following:

```
run 01M3QACQCVRSH6ERF9DMWKG261 (signups)
source [info]: read 12 rows from signups.csv
source: sourced 12 records
score: 12 in, 12 out, 0 cached, 0 filtered, 0 failed
```

There's no receipt. The process died while `crm` was sending.

## Steps

1. Find the dead run:

    ```sh
    gtme runs
    ```

    The output is similar to the following:

    ```
    run                         pipeline  status       started                   records  in flight
    01M3QACQCVRSH6ERF9DMWKG261  signups   interrupted  2026-09-29T19:31:15.483Z  12       4
    ```

    **A run whose process died shows as `interrupted`, and `in flight` counts the sends it never heard back on.** A `gtme run` holds a lock on its run for as long as the process lives, so a free lock means nothing is running it ([SPEC §8](/spec#interrupted-runs--the-run-lock-adr-061)). A run whose process is still alive says `running`, and `--resume` refuses it and changes nothing, so two processes never work one run. If a vendor went down instead, the run ends `failed` and prints the error, and the steps from step 2 on are the same. Copy the id; the next steps call it `RUN_ID`.

    Resume instead of starting over. A plain `gtme run signups.yaml` prints the resume command and starts a new run, which also holds the records that were mid-send at the crash, but it scores all 12 people again, pays for it, and splits the work across two run ids.

1. See how far each record got:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the id from the previous step. The output is similar to the following:

    ```
    run 01M3QACQCVRSH6ERF9DMWKG261
    pipeline: signups
    status:   interrupted (was pid 27 on laptop)
    started:  2026-09-29T19:31:15.483Z

    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   12   -      0       -         -       $0       -
    score   demo/enrich   12  12   -      0       -         -       $0.1200  -
    crm     http/deliver  8   4    -      0       -         -       $0       -
    total: $0.1200 (estimated) spent
    records: 12 (crm=4 score=8)
    config:  2 steps recorded (`gtme freeze 01M3QACQCVRSH6ERF9DMWKG261` rebuilds the pipeline)
    crm: 4 sent to http/deliver with no answer before the run stopped; the resume, or the next run to that target, holds them
    resume:   gtme run signups.yaml --resume 01M3QACQCVRSH6ERF9DMWKG261
    ```

    **This is the receipt the run never printed, rebuilt from the ledger.** `score` cost $0.1200 for all 12, the money resume must not spend again. `crm` took 8 and put 4 out; the `crm:` line says the other 4 got no response before the process died. The `records` line counts records by the last step each one completed, read from `run_records` ([SPEC §3](/spec#3-ledger-schema--decided)). The last line resumes the run.

1. Resume the run by its id. Use the id: `--resume last` takes the newest run of the `signups` pipeline, which is a different run if the pipeline has run again after the crash. Resume runs `signups.yaml` as it is on disk, so fix anything that caused the failure, such as a URL, first:

    ```sh
    gtme run signups.yaml --resume RUN_ID
    ```

    Replace `RUN_ID` with the id from step 1. The output is similar to the following:

    ```
    resuming run 01M3QACQCVRSH6ERF9DMWKG261 (signups)
    source: already sourced (12 records)
    score: 0 in, 0 out, 0 cached, 0 filtered, 0 failed
    crm: 8 in, 4 out, 0 cached, 0 filtered, 0 failed, 4 unconfirmed

    run 01M3QACQCVRSH6ERF9DMWKG261 — done
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   12   -      0       -         -       $0    -
    score   demo/enrich   0   0    -      0       -         -       $0    -
    crm     http/deliver  8   4    -      0       -         -       $0    -
    crm: 4 record(s) may have reached http/deliver before run 01M3QACQCVRSH6ERF9DMWKG261 stopped and were not sent again:
      eli@umbrella.co
      fay@wayne.io
      gus@hooli.com
      hana@stark.io
    Check the target, then: gtme run signups.yaml --resume 01M3QACQCVRSH6ERF9DMWKG261 --resend-unconfirmed
    (or --settle-unconfirmed for the ones it already has; either takes =KEY,… to name some)
    total: $0 spent
    ```

    **`crm` sent the 4 people it never got to and held the 4 it sent without hearing back.** gtme writes a delivery row only after the target responds, so for the held 4 it can't know whether the target has them. A deliver step logs each record before it sends it, and resume finds the ones with no response. It writes each a `deliveries` row with status `unconfirmed` and doesn't send it, in this run or any later one ([SPEC §8](/spec#deliver-idempotency)). The receipt names them.

    A step only takes records the step before it finished, so `score` had 0 in: all 12 were already past it. This receipt counts only this session, so it says $0. Step 6 reads the whole run.

1. Check the target for the held records. Here, the stand-in CRM logs what it got:

    ```sh
    cat received.log
    ```

    The output is similar to the following:

    ```
    dana@contoso.com
    jane.doe@acme.com
    bob@globex.io
    carol@initech.dev
    gus@hooli.com
    hana@stark.io
    fay@wayne.io
    eli@umbrella.co
    ivan@soylent.co
    jo@vandelay.com
    kai@tyrell.io
    lena@cyberdyne.dev
    ```

    **The target has all 4 held people: Eli, Fay, Gus, and Hana.** They reached it before the process died. At most 4 records per crash land in this gap, because a deliver step sends one record per request, 4 at a time by default. On a real target, look up each person the receipt named.

1. Settle the held records the target already has. That's all 4 here. `--settle-unconfirmed` records that you found them there, and sends nothing:

    ```sh
    gtme run signups.yaml --resume RUN_ID --settle-unconfirmed
    ```

    The output is similar to the following:

    ```
    resuming run 01M3QACQCVRSH6ERF9DMWKG261 (signups)
    settled 4 held deliveries: found at the target, not sent
    source: already sourced (12 records)
    score: 0 in, 0 out, 0 cached, 0 filtered, 0 failed
    crm: 4 in, 0 out, 0 cached, 0 filtered, 0 failed, 4 already delivered

    run 01M3QACQCVRSH6ERF9DMWKG261 — done
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   12   -      0       -         -       $0    -
    score   demo/enrich   0   0    -      0       -         -       $0    -
    crm     http/deliver  4   0    -      0       -         -       $0    -
    crm: 4 already delivered
    total: $0 spent
    ```

    **A settled record counts as `already delivered`, in this run and every later one to the same target.** Its `deliveries` row changes from `unconfirmed` to `settled`.

    **If the target is missing some, settle the ones it has by email and release the rest.** Given both flags, the one without a list takes every held record the other didn't name:

    ```sh
    gtme run signups.yaml --resume RUN_ID \
        --settle-unconfirmed=EMAIL,EMAIL --resend-unconfirmed
    ```

    Replace each `EMAIL` with a held email the target already has. An email the run doesn't hold is refused, and nothing changes. Alone, `--resend-unconfirmed` sends every held record, so use it that way only when a second send is harmless, such as a CRM that updates a contact it matches by email. A target that declares `idempotency: native`, meaning it updates in place, isn't held at all: resume sends its in-flight records again. `--resume` refuses a `done` run, and these two flags are the exception.

1. Prove nothing was paid for twice:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the same id. The output is similar to the following:

    ```
    run 01M3QACQCVRSH6ERF9DMWKG261
    pipeline: signups
    status:   done
    started:  2026-09-29T19:31:15.483Z
    finished: 2026-09-29T19:31:27.890Z

    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   12   -      0       -         -       $0       -
    score   demo/enrich   12  12   -      0       -         -       $0.1200  -
    crm     http/deliver  12  8    -      0       -         -       $0       -
    crm: 4 already delivered
    total: $0.1200 (estimated) spent
    records: 12 (crm=12)
    config:  2 steps recorded (`gtme freeze 01M3QACQCVRSH6ERF9DMWKG261` rebuilds the pipeline)
    ```

    **The `score` row is identical to step 2: 12 in, 12 out, $0.1200.** The table counts each record once, by its latest outcome, across all three sessions, so resume called `demo/enrich` for no one and wrote no cost. `crm` shows 8 out and 4 already delivered, the settled ones. `gtme runs` prints a `held:` line while a record is `unconfirmed`, and there's none.

1. Count what the target got:

    ```sh
    sort received.log | uniq -c
    ```

    The output is similar to the following:

    ```
       1 bob@globex.io
       1 carol@initech.dev
       1 dana@contoso.com
       1 eli@umbrella.co
       1 fay@wayne.io
       1 gus@hooli.com
       1 hana@stark.io
       1 ivan@soylent.co
       1 jane.doe@acme.com
       1 jo@vandelay.com
       1 kai@tyrell.io
       1 lena@cyberdyne.dev
    ```

    Everyone got one send. The 4 held people reached the target once, before the crash, and resume didn't send them again.

## What you have now

**The run that died is `done` under its first id, `score` was paid for once, and nobody was sent twice.** `gtme runs` now lists it as finished:

```
run                         pipeline  status  started                   records  in flight
01M3QACQCVRSH6ERF9DMWKG261  signups   done    2026-09-29T19:31:15.483Z  12       -
```

Stop `target.py` in its terminal when you're done.

To have Claude Code recover the next dead run this way, paste this line:

```text
A gtme run of PIPELINE died. Find it with gtme runs and use its run id; never start the pipeline again without --resume. Show me gtme runs for that id, resume it by id, and prove with gtme runs that every paid step's in, out, and cost match what they were before the resume. If the resume held any records as unconfirmed, list them and stop so I can check which of them the target already has. Then settle the ones I name with --settle-unconfirmed=KEY,... and don't run --resend-unconfirmed unless I say so.
```

Replace `PIPELINE` with the pipeline file.

## Next

- [`gtme run`](/reference/cli/run) lists every flag of the command, including `--resume`, `--settle-unconfirmed`, and `--resend-unconfirmed`.
- [The `run_records` table](/reference/ledger-schema/run_records) holds the last completed step that resume reads.
- [The `deliveries` table](/reference/ledger-schema/deliveries) lists the target, scope, key, and status a deliver step checks before it sends.
