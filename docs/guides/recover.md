---
name: Recover
description: Finish a run that died partway, under its own id, without paying twice for any step, and find the few sends that may have gone out twice
for: "A run stopped before its receipt, because a laptop closed, a terminal was killed, or a vendor went down, and you want it finished without paying or sending twice."
learn:
  - "how to tell that a run died, and how far each record got"
  - "how to resume a run by its id, and why not with last"
  - "how to prove from the ledger that no step was paid for twice"
  - "which records a deliver step may have sent twice, and how to find them"
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
    description: The table a deliver step checks before it sends
  - to: /spec#recover
    type: decided-by
    description: The Recover story's invariant and acceptance criteria, which this guide walks through
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The run_records, step_events, and costs tables that resume and the proof read
  - to: /spec#deliver-idempotency
    type: decided-by
    description: A deliver step writes its delivery row after the target responds, and skips a record that already has one
  - to: /decisions#adr-002
    type: decided-by
    description: Steps read from and write to the ledger, which is where resume comes from
---

# Recover

**Goal: a run that died partway reaches `done` under its own id, with no step paid for twice.** You also find the few records that were mid-send when it died, because resume sends those again. SPEC.md, the file that fixes what gtme does, states the promise this guide checks in its [Recover story](/spec#recover):

> a killed run picks up where it left off, never redoing completed, paid-for work.

## Before you start

**You need `gtme` ([Install](/start/install)) and a run that died or failed partway.** [Runs and receipts](/concepts/runs-and-receipts) covers what a run leaves in the [ledger](/concepts/ledger) and what `--resume` does. Work in the ledger the run used: if it set `GTME_LEDGER`, set it again in your new terminal. On your own dead run, go straight to the steps. To practice, you also need `python3`, which runs a stand-in CRM so you can see what the target received, and the outputs will match this page.

**This guide needs no API keys, and its practice run spends a pretend $0.12 and sends only to your own machine.** `demo/enrich` charges a pretend $0.01 per record and calls no vendor. The one [deliver](/concepts/steps-and-roles) step posts each lead to a small server on `127.0.0.1`. When you resume, the few records that were mid-send at the crash are sent a second time, and step 4 finds them first.

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
run 01M3K87YHJJQ3ZCWGYY1FJ0M0T (signups)
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
    run                         pipeline  status   started                   records  in flight
    01M3K87YHJJQ3ZCWGYY1FJ0M0T  signups   running  2026-09-28T05:36:44.082Z  12       -
    ```

    **A run that says `running` when no `gtme` is running has died.** A killed process can't write its own status, so the ledger still has the one it started with. Check that no `gtme run` process is left:

    ```sh
    pgrep -fl "gtme run"
    ```

    It prints nothing when none is running. Resume only after the first process is gone, or both send. If a vendor went down instead, the run ends `failed` and prints the error, and the steps from step 2 on are the same. Copy the id; the next steps call it `RUN_ID`.

1. See how far each record got:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the id from the previous step. The output is similar to the following:

    ```
    run 01M3K87YHJJQ3ZCWGYY1FJ0M0T
    pipeline: signups
    status:   running
    started:  2026-09-28T05:36:44.082Z

    step    claimed  done  cached  failed  cost
    source  -        1     -       -       $0
    score   12       12    -       -       $0.1200
    crm     12       4     -       -       $0
    total: $0.1200 (estimated)
    records: 12 (crm=4 score=8)
    ...
    ```

    **The `records` line counts records by the last step each one completed, read from `run_records`** ([SPEC §3](/spec#3-ledger-schema--decided)). Four people finished `crm`. Eight finished `score` and stopped there. `score` is done for all 12 and cost $0.1200, and that's the money resume must not spend again. `claimed` counts records queued for a step, so it can't tell you which were mid-send.

1. List the records `crm` hadn't finished:

    ```sh
    gtme query "SELECT i.identity_key FROM run_records r
                JOIN identities i ON i.id = r.identity_id
                WHERE r.run_id = 'RUN_ID' AND r.state = 'score'"
    ```

    Replace `RUN_ID` with the id from step 1; `identity_key` is each person's email here. The output is similar to the following:

    ```
    {"identity_key":"bob@globex.io"}
    {"identity_key":"carol@initech.dev"}
    {"identity_key":"eli@umbrella.co"}
    {"identity_key":"fay@wayne.io"}
    {"identity_key":"hana@stark.io"}
    {"identity_key":"ivan@soylent.co"}
    {"identity_key":"kai@tyrell.io"}
    {"identity_key":"lena@cyberdyne.dev"}
    8 rows
    ```

    These eight have no delivery row, so resume sends each of them. On your own run, use the step before your deliver step in place of `score`.

1. Check the target for those eight. Here, the stand-in CRM logs what it got:

    ```sh
    cat received.log
    ```

    The output is similar to the following:

    ```
    jane.doe@acme.com
    gus@hooli.com
    dana@contoso.com
    jo@vandelay.com
    bob@globex.io
    kai@tyrell.io
    hana@stark.io
    eli@umbrella.co
    ```

    **The target has eight people, and the ledger has four deliveries.** Bob, Kai, Hana, and Eli were mid-send when the process died. The target took them, but gtme never heard back, so it wrote no delivery row. gtme writes that row only after the target responds ([SPEC §8](/spec#deliver-idempotency)).

    **At most 4 records per crash land in this gap, because gtme sends 4 at a time by default.** On a real target, look up the people from step 3. gtme has no way to mark them delivered by hand, so decide before you resume. A CRM that matches contacts by email updates the existing contact instead of adding a duplicate, so resending is harmless there. For a target that doesn't, such as a sequence that emails on add, remove those people from it first, so each lands once.

1. Resume the run by its id. Use the id, because `--resume last` picks the newest run of any pipeline, and after another pipeline has run it resumes the wrong one. Resume runs `signups.yaml` as it is on disk, so fix anything that caused the failure, such as a URL, first:

    ```sh
    gtme run signups.yaml --resume RUN_ID
    ```

    Replace `RUN_ID` with the id from step 1. The output is similar to the following:

    ```
    resuming run 01M3K87YHJJQ3ZCWGYY1FJ0M0T (signups)
    source: already sourced (12 records)
    score: 0 in, 0 out, 0 cached, 0 filtered, 0 failed
    crm: 8 in, 8 out, 0 cached, 0 filtered, 0 failed

    run 01M3K87YHJJQ3ZCWGYY1FJ0M0T — done
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   12   -      0       -         -       $0    -
    score   demo/enrich   0   0    -      0       -         -       $0    -
    crm     http/deliver  8   8    -      0       -         -       $0    -
    total: $0 spent
    ```

    **A step only takes records the step before it finished**, so `score` had 0 in: all 12 were already past it. `crm` took the eight at `score`. This receipt counts only this session, so it says $0. Step 6 reads the whole run.

1. Prove nothing was paid for twice:

    ```sh
    gtme runs RUN_ID
    ```

    Replace `RUN_ID` with the same id. The output is similar to the following:

    ```
    run 01M3K87YHJJQ3ZCWGYY1FJ0M0T
    pipeline: signups
    status:   done
    started:  2026-09-28T05:36:44.082Z
    finished: 2026-09-28T05:36:49.445Z

    step    claimed  done  cached  failed  cost
    source  -        1     -       -       $0
    score   12       12    -       -       $0.1200
    crm     20       12    -       -       $0
    total: $0.1200 (estimated)
    records: 12 (crm=12)
    ...
    ```

    **The `score` row is identical to step 2: 12 claimed, 12 done, $0.1200.** `gtme runs` sums the `step_events` and `costs` rows for the id, so an identical row means resume called `demo/enrich` for no one and wrote no cost. `crm` went from 4 done to 12, one per person, and its 20 claims are the 12 from before plus the 8 it took on resume. The status is `done`, and every record reached `crm`.

1. Count what the target got:

    ```sh
    sort received.log | uniq -c
    ```

    The output is similar to the following:

    ```
       2 bob@globex.io
       1 carol@initech.dev
       1 dana@contoso.com
       2 eli@umbrella.co
       1 fay@wayne.io
       1 gus@hooli.com
       2 hana@stark.io
       1 ivan@soylent.co
       1 jane.doe@acme.com
       1 jo@vandelay.com
       2 kai@tyrell.io
       1 lena@cyberdyne.dev
    ```

    The four confirmed before the crash, Jane, Gus, Dana, and Jo, got one send each, because a record with a row in `deliveries` is never sent again. The four from step 4 got two. On a real target, this is the check from step 4, now for duplicates.

## What you have now

**The run that died is `done` under its first id, `score` was paid for once, and you know who was sent twice.** `gtme runs` now lists it as finished:

```
run                         pipeline  status  started                   records  in flight
01M3K87YHJJQ3ZCWGYY1FJ0M0T  signups   done    2026-09-28T05:36:44.082Z  12       -
```

Stop `target.py` in its terminal when you're done.

To have Claude Code recover the next dead run this way, paste this line:

```text
A gtme run of PIPELINE died. Find it with gtme runs and use its run id, never last. Check with pgrep that no gtme run is still going. Show me gtme runs for that id, list the records that hadn't passed the deliver step, and stop so I can check which of them the target already has. After I say go, resume it by id and prove with gtme runs that every paid step's claimed, done, and cost match what they were before the resume.
```

Replace `PIPELINE` with the pipeline file.

## Next

- [`gtme run`](/reference/cli/run) lists every flag of the command, including `--resume`.
- [The `run_records` table](/reference/ledger-schema/run_records) holds the last completed step that resume reads.
- [The `deliveries` table](/reference/ledger-schema/deliveries) lists the target, scope, and key a deliver step checks before it sends.
