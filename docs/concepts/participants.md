---
name: Participants
description: Whoever answers a filter, compose, or review step is a participant, and the adapter id says who, a model, a person, or an agent
for: "You want a person or an agent to judge records inside a pipeline, or a run just ended `pending` with records awaiting someone."
learn:
  - "how `ai/*`, `human/*`, and `agent/*` fill the same three roles"
  - "where a record waits when nobody's at a terminal, and how `gtme answer` and the next run pick it up"
  - "what an answer's provenance says, and why a person isn't asked the same question twice"
  - "what a human step costs a pipeline under cron"
order: 11
roles: [operator, builder]
links:
  - to: /concepts/steps-and-roles
    type: depends-on
    description: The three roles a participant fills, filter, compose, and review, and the of referent a review is about
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: A run with unanswered records ends pending, and the receipt names who it's waiting for
  - to: /concepts/ledger
    type: relates-to
    description: Pending records wait in the ledger, and a collected answer is written back to it as a fact
  - to: /concepts/identity-keys
    type: relates-to
    description: An answer names its record by identity key
  - to: /concepts/facts
    type: relates-to
    description: An answer lands as a fact whose source names the participant
  - to: /concepts/groups
    type: relates-to
    description: The cron pattern, a reviewing pipeline that ends in a group and a scheduled pipeline that sources from it
  - to: /concepts/gate-ladder
    type: relates-to
    description: Under simulate a human or agent step is a gap, and records pass through it untouched
  - to: /guides/human-in-the-loop
    type: relates-to
    description: Swaps an AI step for a human one and answers it, start to finish
  - to: /guides/cron-and-events
    type: relates-to
    description: Scheduling a pipeline, including one that waits on a person
  - to: /guides/claude-code
    type: relates-to
    description: How an agent drives gtme and answers agent steps from its own session
  - to: /start/for-agents
    type: relates-to
    description: The agent's way in, including reading pending records and answering them
  - to: /reference/cli/answer
    type: relates-to
    description: Every flag on the write path for a participant's answer
  - to: /decisions#adr-048
    type: decided-by
    description: Three roles, any participant, and the referent a review is about
  - to: /decisions#adr-049
    type: decided-by
    description: People and agents are adapters, and gtme answer is the one write path
  - to: /decisions#adr-050
    type: decided-by
    description: Why gtme stopped calling an agent as a subprocess and lets the agent drive instead
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The normative rules for asking, waiting, answering, and collection
---

# Participants

Here's a pipeline where a person grades each record's title. Save it as `grading.yaml` next to the `contacts.csv` from [See it run](/start/show-me):

```yaml
name: grading
version: 1
source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }
steps:
  - id: grade
    use: human/review
    of: title
    uses: [full_name]
    provides:
      grade: { type: string, enum: [A, B, C] }
    with:
      prompt: never
```

`uses:` lists the fields shown alongside the title, and `enum` limits the grade to A, B, or C. Plan it, which spends and sends nothing:

```sh
gtme plan grading.yaml
```

The output is the following, trimmed with `...`:

```
...
2. grade [review] — human/review@1
...
     provides:  grading.grade
     of:        title (the value under review)
     render:    the of: value
     prompt:    never — records wait in the ledger; `gtme answer` records, the next `gtme run` collects
...
```

`[review]` is the step's role and `@1` the adapter's version. `provides:` is the field the step writes, prefixed with the pipeline's name. `of:` is the value under review. `render:` is what the person sees, and `prompt:` says whether the run asks. Run it:

```sh
gtme run grading.yaml
```

The output is similar to the following, trimmed with `...`:

```
...
run 01M3FBDG4ZB2THFN1DB7ZM6AEB — pending — ended awaiting human/review: `gtme answer grading` records, the next `gtme run grading` collects
step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
source  csv/source    0   3    -      0       -         -       $0    -
grade   human/review  3   0    -      0       -         -       $0    -
grade: 3 in, 0 out — 3 awaiting human/review; `gtme answer grading` records, the next `gtme run grading` collects (or `gtme show --run 01M3FBDG4ZB2THFN1DB7ZM6AEB --pending grade` to read them)
total: $0 spent
```

## What you just saw

**Whoever answers a step is a participant, and the [adapter](/concepts/adapter-tiers) id says who.** `ai/*` is a model, `human/*` is a person, and `agent/*` is an agent driving gtme. Each fills filter, compose, and review under one contract (the decision record, [ADR-048](/decisions#adr-048)). Swap `ai/review` for `human/review` and `of:`, `uses:`, and `provides:` stay as they are.

**With nobody to ask, the run waits in the [ledger](/concepts/ledger).** At a terminal, the default `prompt: tty` asks you record by record, and Ctrl-C leaves the rest pending. Here `prompt: never` parks all 3 records and the run ends `pending`. In the [receipt](/concepts/runs-and-receipts), `in` and `out` count records into and out of a step, and awaiting records appear in no other column.

Under [simulate](/concepts/gate-ladder), nobody is asked, and records pass through a `human/*` or `agent/*` step untouched.

## Answers and collection

**An agent reads the pending records first:**

```sh
gtme show --run last --pending grade
```

The output is similar to the following, trimmed to Jane with `...`:

```
grade: 3 awaiting human/review — `gtme answer grading grade <identity-key> --set grading.grade=A|B|C`

  jane.doe@acme.com
    title (the value under review): "VP Marketing"
    full_name: "Jane Doe"
{"adapter":"human/review","identity_key":"jane.doe@acme.com","outputs":["grading.grade=A|B|C"],"role":"review","run_id":"01M3FBDG4ZB2THFN1DB7ZM6AEB","step":"grade","surface":"title (the value under review): \"VP Marketing\"\nfull_name: \"Jane Doe\"\n","token":"01M3FBDG4ZB2THFN1DB7ZM6AEB/grade"}
...
```

The indented text is what a person sees, and the JSON line is the same record for an agent. `jane.doe@acme.com` is Jane's [identity key](/concepts/identity-keys).

**`gtme answer` is the write path.** Grade Jane, naming the run by its pipeline:

```sh
gtme answer grading grade jane.doe@acme.com --set grading.grade=A \
    --note "owns the budget"
```

```
grade: jane.doe@acme.com answered by human/trevor — the next `gtme run grading` collects it
```

That pair, the pending listing and the `gtme answer` line, is what you'd hand your agent. `trevor` is the logged-in user, the default participant name. The answer is checked on the spot, so a grade outside the enum is refused:

```
gtme: grading.grade must be one of A, B, C (got "D")
```

A filter step takes `pass=true|false` and a `reason` instead. The answer is saved, but nothing moves and nothing sends until the next run, and a second answer replaces the first.

**The next run collects.** Run `gtme run grading.yaml` again. It resumes the pending run without re-reading the CSV and moves answered records on:

```
...
grade   human/review  3   1    -      0       -         -       $0    -
grade: 3 in, 1 out — 2 awaiting human/review; `gtme answer grading` records, the next `gtme run grading` collects (or `gtme show --run 01M3FBDG4ZB2THFN1DB7ZM6AEB --pending grade` to read them)
...
```

Jane's grade is now a fact, with this row in `gtme show jane.doe@acme.com --provenance`:

```
...
    "grading.grade": {
      "confidence": 1,
      "created_at": "2026-09-26T17:15:14.075Z",
      "note": "owns the budget",
      "referent": "01M3FBDG50QFV6HXF0VFG2EMJV",
      "run_id": "01M3FBDG4ZB2THFN1DB7ZM6AEB",
      "source": "human/review @ trevor#9638bda88be1",
      "value": "A"
    },
...
```

`source` names the adapter, the participant, and a hash of the step's definition. `referent` is the id of the graded `title` fact. [Facts have provenance](/concepts/facts) covers the rest.

**Agents answer the same way.** An `agent/*` adapter works like its `human/*` twin but never prompts. The agent passes `--as NAME`, and `--cost USD` records what it spent. The prefix follows the adapter, so an answer to a `human/*` step is recorded as `human/NAME` whoever typed it, and `--as` names who answered.

That's it. That's how a participant answers.

## So what?

**A person's answer is cached like a model's.** The cache key is the step's definition plus the value under review. The participant's name isn't in it, because the cache is checked before anyone answers. After all 3 were answered, a fresh run showed `grade` with 3 cached and asked nobody. A changed title is asked again, and `cache: 0d` on the step asks everyone.

**Under cron, a human step waits for its person.** A pending run resumes and reads no new rows, so a scheduled pipeline with a human step picks up nothing new until someone answers. The fix is two pipelines: one a person runs that ends in a [group](/concepts/groups), and a scheduled one that sources from that group, as [Run on cron and events](/guides/cron-and-events) shows.

## Why it's this way

**Who answers belongs in the name.** An early plan put a person behind `ai/filter`, a name that says a model answers. [ADR-049](/decisions#adr-049) made people and agents adapters of their own, with `gtme answer` as the one write path ([SPEC §8](/spec#8-cli-surface--decided)). [ADR-050](/decisions#adr-050) stopped gtme from launching Claude Code per batch, because an agent running gtme already has its own model.

**A human step holds the run open until its last answer.** Answered records move on at each collection, but the run stays `pending` until the slowest one is in. A review also never gates, so dropping C grades takes the `sql/filter` shown on Steps and roles.

## Where it shows up

- [Put a human in the loop](/guides/human-in-the-loop) swaps an AI step for a human one and answers it.
- [Use gtme from Claude Code](/guides/claude-code) and [For agents](/start/for-agents) show an agent answering.
- [`gtme answer`](/reference/cli/answer) is the lookup page for every flag.
