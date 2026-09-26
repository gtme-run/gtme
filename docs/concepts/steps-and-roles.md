---
name: Steps and roles
description: A step is an adapter in a role, and the role decides what the step reads from the ledger, what it writes back, and whether a record can stop there
for: "You've read a pipeline file and want to predict what each step will read, write, and cost, or `gtme plan` just told you a step needs a field nothing provides."
learn:
  - "the six roles a single-type pipeline uses, and what each one reads, writes, and costs"
  - "why only a filter writes a verdict, and what `when:` checks"
  - "how `gtme plan` checks each step's needs against what earlier steps provide"
  - "how to read a step's plan block: reads, of, provides, and est/record"
order: 2
roles: [builder, operator]
links:
  - to: /concepts/pipeline
    type: depends-on
    description: The file a step lives in; this page is about one entry in its steps list
  - to: /concepts/ledger
    type: depends-on
    description: Every step reads a projection from the ledger and writes back to it
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan is the rung that runs the needs check; dry-run holds back every deliver step
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The receipt has one row per step, and a deliver step's row is where idempotency shows up
  - to: /concepts/participants
    type: relates-to
    description: Who answers a filter, compose, or review step, whether a model, a person, or an agent
  - to: /concepts/types-and-traverse
    type: relates-to
    description: The traverse role, which changes the record type partway through a run
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Where the adapter behind a step comes from, built in or a binding
  - to: /reference/adapters
    type: relates-to
    description: Every shipped adapter with its role, needs, and provides
  - to: /reference/cli/plan
    type: relates-to
    description: The command that prints each step's role, projection, and provides
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: The planner's walk over available fields, and every rule a step's needs are checked by
  - to: /decisions#adr-048
    type: decided-by
    description: Filter, compose, and review are defined by what goes in, what comes out, and whether it gates
  - to: /decisions#adr-031
    type: decided-by
    description: Deliver is a role any step can have, at any position
---

# Steps and roles

Here's a pipeline with one step in each of the six roles a single-type pipeline uses. Save it as `roles.yaml` next to the `contacts.csv` from [See it run](/start/show-me):

```yaml
name: roles
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: score
    use: demo/enrich
    cache: 30d

  - id: fit
    use: ai/filter
    uses: [title, demo.score]
    with:
      template: Keep people who own outbound tooling decisions.

  - id: draft
    use: ai/compose
    when: fit.passed
    uses: [full_name, title]
    provides: [first_line]
    with:
      template: Write one opening line.

  - id: grade
    use: ai/review
    of: roles.first_line
    uses: [title]
    provides:
      grade: {enum: [A, B, C, D, F]}
    with:
      template: Grade the opening line for a VP audience.

  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      first_line: roles.first_line
      grade: roles.grade
    idempotency: email
```

Plan it. Planning needs no keys, makes no network calls, spends $0, and sends nothing:

```sh
gtme plan roles.yaml
```

The output is the following, trimmed with `...`:

```
pipeline roles (version 1)

1. source [source] — csv/source@1
     provides:  company_domain, email, full_name, title
...
2. score [enrich] — demo/enrich@1
     reads:     email, full_name
     provides:  demo.note, demo.score
...
3. fit [filter] — ai/filter@1
     reads:     title, demo.score
     provides:  (none)
...
4. draft [compose] — ai/compose@1
     when:      fit.passed
     reads:     full_name, title
     provides:  roles.first_line
...
5. grade [review] — ai/review@1
     reads:     title, roles.first_line
     provides:  roles.grade
     of:        roles.first_line (the value under review)
     est/record: ?
...
6. out [deliver] — csv/deliver@1
     reads:     roles.first_line, roles.grade
     provides:  (none)
...

send surface: 1 deliver step(s)
  out → csv/deliver (touch scope: roles)

available fields after the last step: company_domain, demo.note, demo.score, email, full_name, roles.first_line, roles.grade, title
plan ok — nothing has been spent
```

The role is in brackets after each step id.

## What you just saw

**A step is an [adapter](/concepts/adapter-tiers) in a role.** The `use:` line names the adapter, and the adapter's manifest, its declaration of what it needs and provides, names its role ([SPEC §6](/spec#6-adapter-manifest--decided)). The role decides what the step reads from the [ledger](/concepts/ledger), what it writes back, and whether a record can stop there. What a step reads is its projection, the current value of each field it declared, and nothing else.

SPEC §6 lists eight roles. The other two are `traverse`, which changes the record type partway through a run and has [its own page](/concepts/types-and-traverse), and `verify`. `verify` is in the manifest schema and caches like enrich, and no shipped adapter uses it.

| Role | Reads | Writes | Costs | In this file |
|---|---|---|---|---|
| source | a file, an API, or a [group](/concepts/groups)'s members | new records and their fields | the vendor's price per search, no vendor call for a CSV | `source` |
| enrich | the fields its manifest requires, printed as `reads:` | fields | the vendor's price per record, skipped while the fields are fresher than `cache:`, and no vendor call for `sql/transform` | `score` |
| filter | the `uses:` fields | a verdict, pass or fail with a reason, plus any declared fields | model tokens for `ai/filter`, no model for `sql/filter` | `fit` |
| compose | the `uses:` fields | new field values | model tokens for `ai/compose`, no model for `text/compose` | `draft` |
| review | the `of:` value, with `uses:` as context | labels about that value, as fields | model tokens for `ai/review`, no model for `human/review` | `grade` |
| deliver | the values named in `variables:` | a delivery row, and the send itself | $0 declared for `csv/deliver`, and this is the step that sends | `out` |

Plan prints `?` for a step whose adapter publishes no per-record estimate. That's every AI step, and every no-vendor step except `csv/deliver`, the only one that declares $0. An AI step's model spend is measured and shows on the receipt.

**Read `grade` against its plan block.** `reads:` is what it reads, `title` and `roles.first_line`. The `roles.` prefix is the pipeline's name on a field an AI step declared, so two campaigns can each write a person's first line (the decision record, [ADR-033](/decisions#adr-033)). `of:` names the value under review, and that value joins the cache key, so a changed first line gets re-reviewed. `provides:` is what it writes, `roles.grade`, an enum, meaning a fixed set of allowed values, and it costs model tokens with no plan estimate.

**Only a filter writes a verdict.** A record that fails stays in the ledger ([SPEC §7](/spec#7-contract-validation--the-planner--decided)).

A fail already stops a record at `fit`. `when: fit.passed` also holds the ones `fit` never judged, such as a record it skipped for a missing field. It also shows a reviewer, in the file, that the paid step waits on the judgment. `when:` accepts only `STEP_ID.passed` ([SPEC §9](/spec#9-pipelineyaml--decided)).

A review writes a grade and never gates, so the planner refuses a gate on one. Add `when: grade.passed` to `out` and plan again:

```
gtme: step "out": when: grade.passed reads the filter role only — "grade" is a review and never gates; add a sql/filter on its labels and gate on that
```

So gate with a filter. This `sql/filter` goes after `grade`, keeps only A and B grades, and runs no model:

```yaml
  - id: top
    use: sql/filter
    with:
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'roles.grade' AND value IN ('A', 'B')
```

That's the step you'd hand your agent.

**Who answers is in the adapter id.** `ai/`, `human/`, and `agent/` adapters fill the same three roles of filter, compose, and review ([ADR-048](/decisions#adr-048)). [Participants](/concepts/participants) covers who can answer and how a run waits for them.

**A deliver step can sit anywhere in `steps:`.** A pipeline can have no deliver steps, one, or several, and each sends exactly the records that survived everything before it ([ADR-031](/decisions#adr-031)). The plan lists every one under `send surface:`, where `touch scope: roles` means a delivery is scoped to this pipeline. [Runs and receipts](/concepts/runs-and-receipts) shows how idempotency keeps a re-run from sending twice. Dry-run versus [armed](/concepts/gate-ladder) is a property of the run, so no step can switch it off.

## How the planner checks needs

**The planner walks the steps in order, keeping a list of fields that are available so far.** The source's provides start the list, and each step adds its own. Before adding, it checks that everything the step needs is already there: the manifest's required fields, `uses:`, `of:`, and a deliver step's `variables:`. The last line of the plan is the finished list. Delete the `score` step and plan again:

```
gtme: step "fit": needs demo.score, which no earlier step provides (available: company_domain, email, full_name, title); installed adapters provide it: demo.score ← demo/enrich
```

Plan catches it before a run pays for anything.

## Why it's this way

**Roles are defined by what goes in, what comes out, and whether it gates.** [ADR-048](/decisions#adr-048) settled on filter, compose, and review because every judgment or writing task fits one of the three. A new kind of participant becomes a new adapter prefix, with no new grammar. The cost: a review that should gate needs one more step, a `sql/filter` on the grade. We think that's the right trade, because a grade stays a fact you can query later.

**Deterministic work goes to SQL steps.** [ADR-027](/decisions#adr-027) added `sql/enrich`, renamed `sql/transform` by [ADR-037](/decisions#adr-037), and `sql/filter`, so splitting a name or keeping scores over 70 costs nothing and runs the same every time. Both are ordinary steps in the enrich and filter roles.

## Where it shows up

- [A pipeline is a YAML file](/concepts/pipeline) covers the file these steps live in.
- [Put a human in the loop](/guides/human-in-the-loop) swaps an AI step for a `human/*` one in the same role.
- [Adapter catalog](/reference/adapters) lists every shipped adapter with its role, and [`gtme plan`](/reference/cli/plan) is the lookup page for the output.
