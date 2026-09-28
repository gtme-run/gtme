---
name: Write a process adapter
description: Write an adapter as a small program that speaks the wire protocol, for a job a YAML binding can't express, and run a pipeline through it
for: "Your integration needs a conditional, a computation, or several calls per record, so a binding can't express it, and you're ready to write a short script."
learn:
  - "write the manifest.json that gtme plan reads"
  - "write a program that reads and writes NDJSON messages, and test it by hand"
  - "put the adapter where gtme finds it, then plan, simulate, and run it"
  - "what gtme checks in your program's output, and what happens when it's wrong"
order: 18
roles: [extender]
links:
  - to: /concepts/adapter-tiers
    type: depends-on
    description: The test for when a job needs a process adapter instead of a binding, and the manifest surface both tiers share
  - to: /concepts/pipeline
    type: relates-to
    description: The pipeline file whose owner step names the process adapter by its id
  - to: /concepts/facts
    type: relates-to
    description: Each field the program returns becomes a fact with the adapter and version as its source
  - to: /start/add-a-vendor
    type: relates-to
    description: Writes the first tier, a binding, for a plain HTTP vendor API
  - to: /guides/add-a-binding
    type: relates-to
    description: The full binding walkthrough; start there if your vendor is a documented HTTP API
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The manifest's role decides which messages the program receives and must send
  - to: /concepts/canonical-fields
    type: relates-to
    description: Every field in needs and provides is canonical for the entity type or carries a prefix
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan reads the manifest, simulate runs the program for real, and armed writes to the ledger
  - to: /concepts/ledger
    type: relates-to
    description: The runner writes what the program returns, and the program never touches the ledger
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A bundle carries bindings and doesn't carry a process adapter
  - to: /reference/cli/adapters
    type: relates-to
    description: Lists installed adapters with their kind and source
  - to: /spec#5-wire-protocol--decided
    type: decided-by
    description: The message contract, the rule to ignore unknown types, and output validation against provides
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: The manifest keys, the discovery folder holding manifest.json and run, and credential injection
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: The graduation rule that sends conditionals and computation to a process adapter
  - to: /decisions#adr-022
    type: decided-by
    description: Why vendors are YAML by default and logic lives in a program
  - to: /decisions#adr-026
    type: decided-by
    description: Whoever defines an adapter's contract names it
---

# Write a process adapter

**Goal: an adapter that's a Python script, and a [pipeline](/concepts/pipeline) that runs contacts through it.**

The job is routing: give each contact an account owner, and write the result to a CSV. A named account goes to the rep who owns it. Every other company goes to one of your reps, picked from a hash of its domain, so a rerun picks the same one while the rep list stays the same.

Picking a rep takes a conditional and a hash, which a binding can't express ([SPEC §10a](/spec#10a-the-binding-tier--universal-steps--decided-adr-022027)). So this is a *process adapter*: a program in any language that exchanges records with gtme as NDJSON, newline-delimited JSON with one object per line.

## Before you start

- **Read [Two adapter tiers](/concepts/adapter-tiers)** for the test between the tiers. If your vendor is a documented HTTP API, [Add a vendor with a binding](/guides/add-a-binding) is the shorter path.
- **You need** gtme and Python 3 on your `PATH`. On macOS, `python3` comes with the Xcode Command Line Tools. Any language works; this guide uses Python because the script is short.
- **This guide spends $0 and sends nothing past your own disk.** The adapter calls no vendor, and the pipeline's last step writes a local `routed.csv`.

## Steps

1. Make a project folder with its own [ledger](/concepts/ledger), put its `adapters` folder on the search path, and fetch a practice CSV:

    ```sh
    mkdir -p routing/adapters/territory-owner && cd routing
    export GTME_LEDGER=$PWD/ledger.db
    export GTME_ADAPTER_PATH=$PWD/adapters
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
    ```

    gtme searches `GTME_ADAPTER_PATH` before `~/.gtme/adapters`. Set it in every shell you run gtme from.

1. Write the manifest, `adapters/territory-owner/manifest.json`:

    ```json
    {
      "id": "territory/owner",
      "version": 1,
      "role": "enrich",
      "entity_type": "person",
      "needs": {
        "type": "object",
        "required": ["company_domain"],
        "properties": {
          "company_domain": {"type": "string"}
        }
      },
      "provides": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "territory.owner": {"type": "string"},
          "territory.rule": {"type": "string", "enum": ["named", "round_robin"]}
        }
      },
      "config_schema": {
        "type": "object",
        "additionalProperties": false,
        "required": ["reps"],
        "properties": {
          "reps": {"type": "array", "items": {"type": "string"}, "minItems": 1},
          "named": {"type": "object", "additionalProperties": {"type": "string"}}
        }
      },
      "cost_estimate_usd": 0
    }
    ```

    The manifest is the whole contract `gtme plan` sees ([SPEC §6](/spec#6-adapter-manifest--decided)). `role: enrich` means records come in and fields go out. `needs` is the one field the program reads, `provides` is what it writes, and `config_schema` is what a step may set under `with:`. All three are JSON Schema: `required` lists fields that must be present, `enum` lists the only allowed values, and `additionalProperties: false` refuses any field not listed.

    Every field name is a [canonical field](/concepts/canonical-fields) or carries a prefix. You own this contract, so the id and the prefix are `territory`, not a vendor's name (the decision record, [ADR-026](/decisions#adr-026)). The folder's name is the id with the slash as a dash. `cost_estimate_usd` is what plan prints per record. A program that calls a paid API also sends a `COST` message per record, and declares its key under `credentials`, which gtme passes in as an environment variable ([SPEC §6](/spec#6-adapter-manifest--decided)).

1. Write the program, `adapters/territory-owner/run`:

    ```python
    #!/usr/bin/env python3
    # territory/owner: give each person an account owner.
    # A named account goes to its owner. Every other company goes to one of
    # the reps, picked by a hash of its domain, so a rerun picks the same rep.

    import hashlib
    import json
    import os
    import sys

    HERE = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(HERE, "manifest.json")) as f:
        PROVIDES = json.load(f)["provides"]


    def send(msg):
        print(json.dumps(msg), flush=True)


    def route(domain, reps, named):
        if domain in named:
            return named[domain], "named"
        n = int(hashlib.sha256(domain.encode()).hexdigest(), 16)
        return reps[n % len(reps)], "round_robin"


    config = {}
    routed = 0
    for line in sys.stdin:
        msg = json.loads(line)
        if msg["type"] == "OPEN":
            config = msg["config"]
            send({"type": "SCHEMA", "provides": PROVIDES})
        elif msg["type"] == "RECORD":
            domain = msg["fields"]["company_domain"]
            owner, rule = route(domain, config["reps"], config.get("named", {}))
            send({"type": "RECORD", "key": msg["key"],
                  "fields": {"territory.owner": owner, "territory.rule": rule}})
            routed += 1
        elif msg["type"] == "END":
            break
        # Ignore any other message type, as the protocol requires.

    send({"type": "LOG", "level": "info", "msg": f"routed {routed} record(s)"})
    send({"type": "END"})
    ```

    Make it executable:

    ```sh
    chmod +x adapters/territory-owner/run
    ```

    That's the wire protocol for an enrich step ([SPEC §5](/spec#5-wire-protocol--decided)). gtme sends `OPEN` with the step's `with:` values, one `RECORD` per person, and `END`. The program replies with `SCHEMA` first, then a `RECORD` per person echoing the `key` it was handed, and `END`.

    Standard output carries messages only. Write debugging text to standard error, `print(..., file=sys.stderr)` in Python, which gtme prints prefixed with the folder name. A program that exits non-zero fails the records it was handed, and the run reports the exit status.

1. Test the program by hand before gtme runs it. Save these three lines as `jane.ndjson`. They're what gtme sent for Jane, copied from a real run:

    ```json
    {"type":"OPEN","step_id":"owner","run_id":"01M3MKAZP42VHWDJMEVR1ND9NM","config":{"named":{"acme.com":"Dana"},"reps":["Dana","Luis","Priya"]}}
    {"type":"RECORD","key":{"entity_type":"person","identity_key":"jane.doe@acme.com"},"fields":{"company_domain":"acme.com"}}
    {"type":"END"}
    ```

    The `RECORD` carries only `company_domain`, the one field `needs` declares, although gtme knows Jane's name, email, and title too. The `key` is Jane's [identity key](/concepts/identity-keys), and the program echoes it back unchanged. For your own adapter, write these lines by hand from your manifest's `needs`. Pipe the file in:

    ```sh
    adapters/territory-owner/run < jane.ndjson
    ```

    The output is the following:

    ```
    {"type": "SCHEMA", "provides": {"type": "object", "additionalProperties": false, "properties": {"territory.owner": {"type": "string"}, "territory.rule": {"type": "string", "enum": ["named", "round_robin"]}}}}
    {"type": "RECORD", "key": {"entity_type": "person", "identity_key": "jane.doe@acme.com"}, "fields": {"territory.owner": "Dana", "territory.rule": "named"}}
    {"type": "LOG", "level": "info", "msg": "routed 1 record(s)"}
    {"type": "END"}
    ```

1. Check that gtme finds it:

    ```sh
    gtme adapters
    ```

    The output is the following:

    ```
    ID               VERSION  ROLE    KIND     SOURCE
    territory/owner  1        enrich  process  installed by hand
    ```

    `gtme adapters verify` runs a binding's recorded responses, so it doesn't apply here. For a process adapter, the hand test and simulate are the checks.

1. Save the pipeline as `route.yaml`:

    ```yaml
    name: route
    version: 1

    source:
      use: csv/source
      with:
        path: contacts.csv
        columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

    steps:
      - id: owner
        use: territory/owner
        with:
          reps: [Dana, Luis, Priya]
          named:
            acme.com: Dana

      - id: out
        use: csv/deliver
        with:
          path: routed.csv
        variables:
          owner: territory.owner
          rule: territory.rule
        idempotency: email
    ```

    `owner` routes each person, and `out` writes a row per person with the fields named under `variables:`. `idempotency: email` writes each person to this file once, however many times the pipeline runs.

1. Plan it:

    ```sh
    gtme plan route.yaml
    ```

    The output is similar to the following:

    ```
    pipeline route (version 1)
    ...
    2. owner [enrich] — territory/owner@1 (external: /tmp/gtme-demo/routing/adapters/territory-owner)
         entity:    person
         reads:     company_domain
         requires:  company_domain
         provides:  territory.owner, territory.rule
         cache:     off (no freshness_days, no cache:)
         est/record: $0.0000
    ...
    plan ok — nothing has been spent
    ```

    Plan read the manifest without starting the program. It checked the `with:` block against `config_schema`, and that `company_domain` exists by this step. `cache: off` means every run routes everyone again, which is what you want after the rep list changes. Add `"freshness_days": 30` to the manifest when a result stays good for 30 days.

1. Run it under simulate:

    ```sh
    gtme run route.yaml --simulate
    ```

    The output is similar to the following:

    ```
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    run 01M3MKJZ4GZPZ8H3D9AVV9NKEM (route)
    source [info]: read 3 rows from contacts.csv
    source: sourced 3 records
    owner [info]: routed 1 record(s)
    owner [info]: routed 1 record(s)
    owner [info]: routed 1 record(s)
    owner: 3 in, 3 out, 0 cached, 0 filtered, 0 failed
    out: 3 in, 0 out, 0 cached, 0 filtered, 0 failed, 3 held (dry run)

    run 01M3MKJZ4GZPZ8H3D9AVV9NKEM — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
    step    adapter          in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source       0   3    -      0       -         -       $0    -
    owner   territory/owner  3   3    -      0       -         -       $0    -
    out     csv/deliver      3   0    -      0       -         -       $0    -
    out: resolved variables for 3 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        owner: "Dana"
        rule: "named"
      bob@globex.io
        owner: "Priya"
        rule: "round_robin"
      carol@initech.dev
        owner: "Luis"
        rule: "round_robin"
    total: $0 spent
    ```

    The three `routed 1 record(s)` lines are three copies of your program. The runner splits a step's records across parallel sessions, so a program can't count or rank across the whole run. That's why this one hashes the domain instead of taking the next rep in line.

    Simulate holds every deliver step the way a dry run does, which is why the receipt says `3 held (dry run)` and mentions `--dry-run`: `out` wrote no rows. This guide skips the dry-run rung of the [gate ladder](/concepts/gate-ladder) because nothing here spends. Simulate ran your program for real, because it declares no credential, so anything it fetched would be fetched. A program that declares `credentials` doesn't run under simulate; the [receipt](/concepts/runs-and-receipts) reports it as a simulation gap, a step simulate couldn't exercise.

1. Optional: break it on purpose. Change the round-robin label to a value `provides` doesn't allow, and simulate again:

    ```sh
    perl -pi -e 's/"round_robin"$/"roundrobin"/' adapters/territory-owner/run
    gtme run route.yaml --simulate
    ```

    The receipt ends with something similar to the following:

    ```
    ...
    step    adapter          in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source       0   3    -      0       -         -       $0    -
    owner   territory/owner  3   1    -      0       -         2       $0    -
    out     csv/deliver      1   0    -      0       -         -       $0    -
    owner: 2 failed — output does not match provides: jsonschema: '/territory.rule' does not validate with file:///tmp/gtme-demo/routing/territory/owner/provides#/properties/territory.rule/enum: value must be one of "named", "round_robin"
    ...
    ```

    The runner checks every `RECORD` against `provides` before it writes anything, fails the records that don't match, and carries on with the rest. Put the label back:

    ```sh
    perl -pi -e 's/"roundrobin"$/"round_robin"/' adapters/territory-owner/run
    ```

1. Run it armed, with no flag. This writes to the ledger and to `routed.csv`, and spends nothing:

    ```sh
    gtme run route.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3MKJZBVMPHGFWCGS1ABYKDD (route)
    ...
    out [info]: csv/deliver: wrote 1 row(s) to routed.csv
    out [info]: csv/deliver: wrote 1 row(s) to routed.csv
    out [info]: csv/deliver: wrote 1 row(s) to routed.csv
    out: 3 in, 3 out, 0 cached, 0 filtered, 0 failed

    run 01M3MKJZBVMPHGFWCGS1ABYKDD — done
    step    adapter          in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source       0   3    -      0       -         -       $0    -
    owner   territory/owner  3   3    -      0       -         -       $0    -
    out     csv/deliver      3   3    -      0       -         -       $0    -
    total: $0 spent
    ```

    Read the file:

    ```sh
    cat routed.csv
    ```

    The output is the following:

    ```
    identity_key,owner,rule
    jane.doe@acme.com,Dana,named
    carol@initech.dev,Luis,round_robin
    bob@globex.io,Priya,round_robin
    ```

    A rerun after you change the rep list updates each owner in the ledger, but `idempotency: email` keeps `routed.csv` as it is. Point `path:` at a new file for a fresh list. Adding a rep also moves some accounts already assigned, because the hash picks from a longer list.

## What you have now

**An adapter that runs as its own program, and its results in the ledger as facts with its name on them.** Check one of the round-robin people:

```sh
gtme show bob@globex.io
```

The output is similar to the following:

```json
{
...
  "entity_type": "person",
  "fields": {
    "company_domain": "globex.io",
    "email": "bob@globex.io",
    "full_name": "Bob Stone",
    "territory.owner": "Priya",
    "territory.rule": "round_robin",
    "title": "Head of Growth"
  },
  "identity_key": "bob@globex.io",
  "identity_key_tier": "email"
}
```

Your program never opened the ledger. The runner wrote what it returned, and each [fact](/concepts/facts) names the adapter and version that produced it:

```sh
gtme query "SELECT DISTINCT field, source FROM current_values \
    WHERE field LIKE 'territory.%'"
```

The output is the following:

```
{"field":"territory.owner","source":"territory/owner@1"}
{"field":"territory.rule","source":"territory/owner@1"}
2 rows
```

Bump `version` in the manifest when you change what the program returns, so old and new facts stay distinguishable.

The folder `adapters/territory-owner` is the whole adapter. Commit it with the pipeline. It doesn't travel inside a [bundle](/concepts/campaign-is-a-folder), so whoever runs your campaign installs it from there by hand. To use it from every folder on this machine, copy it to `~/.gtme/adapters/territory-owner`.

To have Claude Code write one for your own job, paste this line from the `routing` folder:

```text
Using adapters/territory-owner as the model, write a gtme process adapter with id ID that JOB. Test its run file by piping OPEN, RECORD, and END lines into it, then plan and simulate a pipeline through it with GTME_ADAPTER_PATH set to the adapters folder. If it needs an API key, declare it under credentials and tell me the variable name. Don't arm anything.
```

Replace the following:

- `ID`: the adapter's id, such as `territory/owner`
- `JOB`: what the adapter does, starting with a verb

## Next

- [Steps and roles](/concepts/steps-and-roles) says what each role does, which decides the messages a program for it sends.
