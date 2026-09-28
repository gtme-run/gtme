---
name: Put a human in the loop
description: Add a person to a pipeline as a filter, compose, or review step, answer what's waiting with gtme answer, and collect it with the next run
for: "You're building a pipeline and want a person to approve records or write a line before anything is delivered, with no model and no key."
learn:
  - "which `human/*` adapter fits approving, writing, or grading"
  - "how to see what's waiting and answer it from the command line"
  - "what happens to a rejection, a partial batch of answers, and an invalid answer"
  - "how an agent does the same job with `agent/*` steps"
order: 13
roles: [builder, operator]
links:
  - to: /reference/adapters/human-compose
    type: relates-to
    description: The keys for a step where a person writes a field
  - to: /reference/cli/show
    type: relates-to
    description: Reads what's pending and who wrote each value
  - to: /concepts/participants
    type: depends-on
    description: What a participant, a pending run, an answer, and collection are; this guide is the procedure
  - to: /concepts/steps-and-roles
    type: depends-on
    description: The filter, compose, and review roles a person fills
  - to: /concepts/pipeline
    type: relates-to
    description: The YAML file this guide adds two human steps to
  - to: /concepts/ledger
    type: relates-to
    description: Where pending records wait and where a collected answer lands as a fact
  - to: /start/show-me
    type: relates-to
    description: The source of contacts.csv, the three fictional people this guide approves
  - to: /reference/cli/answer
    type: relates-to
    description: Every flag on gtme answer
  - to: /reference/adapters/human-filter
    type: relates-to
    description: The config keys the approve step accepts, including prompt
  - to: /reference/adapters/human-review
    type: relates-to
    description: The review adapter, for a person grading a value instead of approving a record
  - to: /start/for-agents
    type: relates-to
    description: How an agent reads pending records and answers them from its own session
  - to: /concepts/groups
    type: relates-to
    description: The pattern for a pipeline with a person in it that also runs on a schedule
  - to: /decisions#adr-049
    type: decided-by
    description: People and agents are adapters, and gtme answer is the one write path
  - to: /decisions#adr-050
    type: decided-by
    description: An agent answers as the driver of gtme, never as a subprocess gtme launches
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The normative rules for asking, waiting, answering, and collect-first
---

# Put a human in the loop

By the end of this guide, a person approves each record in your [pipeline](/concepts/pipeline) and writes an opening line for each one they keep, before anything is delivered.

## Before you start

You need gtme [installed](/start/install) and the `contacts.csv` from [See it run](/start/show-me) in an empty folder. If a pending run or collecting is new to you, read [Participants](/concepts/participants) first.

No keys. This guide spends nothing and sends nothing: the one [deliver](/concepts/steps-and-roles) step writes a local file, `approved.csv`. The `--cost` flag in step 8 records a figure in the [ledger](/concepts/ledger), and no money moves.

Answers are written to the ledger the pipeline runs against, so the people answering work on the machine, or the shared ledger file, that runs it. gtme doesn't notify anyone. Send each person the command to run.

## Steps

1. **Pick the role.** Each role has a `human/*` [adapter](/concepts/adapter-tiers):

    | Adapter | The person | Answers with |
    |---|---|---|
    | [`human/filter`](/reference/adapters/human-filter) | approves or rejects a record, and a rejected record goes no further | `pass=true\|false` and a `reason` |
    | [`human/compose`](/reference/adapters/human-compose) | writes a line | the fields in `provides:` |
    | `human/review` | grades a value an earlier step produced, named by `of:`, and never drops a record | the labels in `provides:` |

    We'd put the person after any cheaper `sql/*` or `ai/*` filter, so they only see the records that are left.

1. **Write the pipeline.** This one approves, then writes. Save it as `approvals.yaml`:

    ```yaml
    name: approvals
    version: 1
    source:
      use: csv/source
      with:
        path: contacts.csv
        columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }
    steps:
      - id: approve
        use: human/filter
        uses: [full_name, title, company_domain]
        with:
          prompt: never
      - id: opener
        use: human/compose
        uses: [full_name, title, company_domain]
        provides: [first_line]
        with:
          prompt: never
      - id: out
        use: csv/deliver
        with:
          path: approved.csv
        variables:
          first_line: approvals.first_line
        idempotency: email
    ```

    `uses:` is what the person sees for each record, and the step id, `approve` or `opener`, is what they enter, so pick ids that read well. A field a step provides is stored under the pipeline's name, so `out` reads `approvals.first_line`. `idempotency: email` means nobody is written to `approved.csv` twice.

    `prompt: never` means the run never asks, even at a terminal: records wait in the ledger for `gtme answer`. We use it so the run behaves the same whether you, an agent, or a scheduler starts it. The default, `prompt: tty`, has `gtme run` walk the records itself at a terminal. If you use it, finish the walk: stopping it with Ctrl-C can list a record you already rejected as pending again.

1. **Run it until it's pending.**

    ```sh
    gtme run approvals.yaml
    ```

    The output is similar to the following, trimmed with `...`:

    ```
    ...
    run 01M3MFA6GD2T5243AY8R086PVA — pending — ended awaiting human/filter: `gtme answer approvals` records, the next `gtme run approvals` collects
    step     adapter        in  out  empty  cached  filtered  failed  cost  avoided
    source   csv/source     0   3    -      0       -         -       $0    -
    approve  human/filter   3   0    -      0       -         -       $0    -
    opener   human/compose  0   0    -      0       -         -       $0    -
    out      csv/deliver    0   0    -      0       -         -       $0    -
    approve: 3 in, 0 out — 3 awaiting human/filter; `gtme answer approvals` records, the next `gtme run approvals` collects (or `gtme show --run 01M3MFA6GD2T5243AY8R086PVA --pending approve` to read them)
    total: $0 spent
    ```

    All 3 records stopped at `approve`, and the [run](/concepts/runs-and-receipts) ended `pending`.

1. **Read what's waiting.** Run the command the receipt printed, or name the run as `last`:

    ```sh
    gtme show --run last --pending approve
    ```

    The output is similar to the following, trimmed to Jane with `...`:

    ```
    approve: 3 awaiting human/filter — `gtme answer approvals approve <identity-key> --set pass=true|false --set reason=<text>`

      jane.doe@acme.com
        full_name: "Jane Doe"
        title: "VP Marketing"
        company_domain: "acme.com"
    ...
    ```

    The first line is the answer command with its menu. Each record is named by its [identity key](/concepts/identity-keys), here the email, and a JSON line after each record carries the same thing for an agent.

1. **Approve Jane.** `--as` names who answered, here Alex, the SDR:

    ```sh
    gtme answer approvals approve jane.doe@acme.com --set pass=true \
        --set reason="owns the budget" --as alex
    ```

    ```
    approve: jane.doe@acme.com answered by human/alex — the next `gtme run approvals` collects it
    ```

    Without `--as`, gtme records your login name. The answer is checked against the step's outputs on the spot. A bad one is refused, nothing is recorded, and the record stays pending. `--set pass=maybe` prints:

    ```
    gtme: pass must be true or false (got maybe)
    ```

    `--set score=5` prints:

    ```
    gtme: score: not an output of this step — it takes pass=true|false, reason=<text>
    ```

1. **Reject Bob, and leave Carol for later.**

    ```sh
    gtme answer approvals approve bob@globex.io --set pass=false \
        --set reason="growth, not marketing" --as alex
    ```

    ```
    approve: bob@globex.io answered by human/alex — the next `gtme run approvals` collects it
    ```

    To change a verdict, answer the record again before the next run, and the second answer replaces the first. After the run collects it, `gtme answer` refuses: the run has nothing pending for that record.

1. **Collect.** Run the pipeline again. It resumes the pending run instead of reading the CSV again:

    ```sh
    gtme run approvals.yaml
    ```

    The output is similar to the following, trimmed with `...`:

    ```
    collecting run 01M3MFA6GD2T5243AY8R086PVA — the latest run of "approvals" ended with a step in flight
    ...
    step     adapter        in  out  empty  cached  filtered  failed  cost  avoided
    source   csv/source     0   3    -      0       -         -       $0    -
    approve  human/filter   3   1    -      0       1         -       $0    -
    opener   human/compose  1   0    -      0       -         -       $0    -
    out      csv/deliver    0   0    -      0       -         -       $0    -
    ...
    ```

    Jane passed and now waits at `opener`. Bob was rejected, so he's counted under `filtered` and goes no further. Carol, unanswered, is still pending at `approve`, and the run stays `pending` until she's answered. If nobody will answer a record, answer it `pass=false` with a reason, so the run can finish.

1. **Write Jane's opening line at the terminal.** Two steps are pending now, so name the step. Leave out the identity key and `gtme answer` walks the pending records one by one. If the person bills for the work, `--cost` records it, and `--note` is kept with the answer:

    ```sh
    gtme answer approvals opener --as sam --cost 0.50 --note "contract writer"
    ```

    The output is the following, with Sam's line typed at the prompt:

    ```
    opener (human/compose): 1 record(s) to answer — Ctrl-C leaves the rest pending

    [1/1] jane.doe@acme.com
      full_name: "Jane Doe"
      title: "VP Marketing"
      company_domain: "acme.com"
      approvals.first_line [<text>]: Saw Acme's Q3 launch; congrats on the pipeline numbers.
    opener: 1 answered by human/sam — the next `gtme run approvals` collects
    ```

    Quotes and apostrophes are fine at the prompt. Leave the step name out while two are pending and gtme asks for it:

    ```
    gtme: run 01M3MFA6GD2T5243AY8R086PVA has 2 steps pending (approve, opener) — name the one to answer: `gtme answer approvals <step> ...`
    ```

1. **Collect again.**

    ```sh
    gtme run approvals.yaml
    ```

    Jane reaches `out` while Carol still waits. The output is similar to the following, trimmed with `...`:

    ```
    ...
    opener   human/compose  1   1    -      0       -         -       $0.5000  -
    out      csv/deliver    1   1    -      0       -         -       $0       -
    total: $0.5000 (estimated) spent
    ```

    The cost is `estimated` because a person reported it. An agent that knows its spend adds `--measured`, and the receipt shows it as measured.

1. **Answer Carol at the terminal.** A wrong entry at the `pass [y/n]` prompt is refused after the reason, and the record stays pending:

    ```sh
    gtme answer approvals approve --as alex
    ```

    With `maybe` typed at the prompt, the walk ends:

    ```
      reason: ops, not a buyer
      refused: pass must be true or false (got maybe)
    approve: 0 answered by human/alex, 1 still pending — the next `gtme run approvals` collects
    ```

    Run the same command again and answer `n`:

    ```
    approve (human/filter): 1 record(s) to answer — Ctrl-C leaves the rest pending

    [1/1] carol@initech.dev
      full_name: "Carol Reyes"
      title: "Marketing Operations Manager"
      company_domain: "initech.dev"
      pass [y/n]: n
      reason: ops, not a buyer
    approve: 1 answered by human/alex — the next `gtme run approvals` collects
    ```

1. **Collect the last answer.**

    ```sh
    gtme run approvals.yaml
    ```

    The run ends `done`:

    ```
    ...
    run 01M3MFA6GD2T5243AY8R086PVA — done
    ...
    ```

## Let an agent answer instead

**An `agent/*` step is the same step with an agent answering.** In `approvals.yaml`, change `use: human/filter` to `use: agent/filter` and `use: human/compose` to `use: agent/compose`. An agent step never prompts, so `prompt: never` is redundant there. Run the pipeline, and the agent reads the same pending listing and answers the same way, naming itself and what it spent:

```sh
gtme answer approvals approve jane.doe@acme.com --set pass=true \
    --set reason="VP Marketing at a fit domain" \
    --as claude-code --cost 0.01 --measured
```

```
approve: jane.doe@acme.com answered by agent/claude-code — the next `gtme run approvals` collects it
```

gtme doesn't launch the agent. The agent runs gtme from its own session and answers on its own schedule (the decision record, [ADR-050](/decisions#adr-050)). [For agents](/start/for-agents) has the rules it follows.

## What you have now

`approved.csv` holds the one record a person approved and wrote for:

```
identity_key,first_line
jane.doe@acme.com,Saw Acme's Q3 launch; congrats on the pipeline numbers.
```

Check who wrote it with [`gtme show`](/reference/cli/show) `jane.doe@acme.com --provenance`. The line's [fact](/concepts/facts) carries its source, `human/compose` and the person, and the note:

```
...
    "approvals.first_line": {
      "confidence": 1,
      "created_at": "2026-09-28T16:59:48.541Z",
      "note": "contract writer",
      "run_id": "01M3MFA6GD2T5243AY8R086PVA",
      "source": "human/compose @ sam#372662fff872",
      "value": "Saw Acme's Q3 launch; congrats on the pipeline numbers."
    }
...
```

Run it again and nobody is asked, and nothing is delivered twice. A record whose shown fields change, such as a new title, is asked again:

```
...
approve  human/filter   3   0    -      3       2         -       $0    ?
opener   human/compose  1   0    -      1       -         -       $0    ?
out      csv/deliver    1   0    -      1       -         -       $0    $0.0000
...
```

The `?` means a person's answer has no price to count as saved. Hand `approvals.yaml` to your agent as the pattern for any step that needs a person's sign-off; [ADR-049](/decisions#adr-049) and [SPEC §8](/spec#8-cli-surface--decided) have the rules.

## Next

- [`gtme answer`](/reference/cli/answer) lists every flag.
- [`human/review`](/reference/adapters/human-review) has the keys for a step that grades instead of approving.
- [Groups and segments](/concepts/groups) shows how to review into a group in one pipeline and send from it in another, so a scheduled run never waits on a person.
