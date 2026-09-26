---
name: A pipeline is a YAML file
description: A pipeline.yaml file is a source that brings records in and a list of steps the runner executes in order, each naming an adapter
for: "You've run an example pipeline and want to write or read your own, and predict what each key does before you run it."
learn:
  - "what each top-level key and each step key does"
  - "which keys are valid on which kind of step, and how plan checks them"
  - "why a vendor's field names appear only at the two ends of a file"
order: 1
links:
  - to: /concepts/steps-and-roles
    type: relates-to
    description: Each step's adapter has a role, and the role decides which step keys are valid
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: A run executes this file, and its receipt has one row per step id
  - to: /concepts/ledger
    type: relates-to
    description: Steps read their fields from the ledger and write their results back to it
  - to: /concepts/canonical-fields
    type: relates-to
    description: Inside a pipeline, every field has a canonical or namespaced name
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan is the first check on this file, and the ladder is the rest of the way to a real run
  - to: /concepts/groups
    type: relates-to
    description: The group keys this page leaves out, top-level group and require and exclude on a step
  - to: /start/show-me
    type: relates-to
    description: Runs the same cache.yaml twice, armed, at $0
  - to: /guides/first-pipeline
    type: relates-to
    description: Writes a pipeline like this one from your own CSV, key by key
  - to: /reference/pipeline-yaml
    type: relates-to
    description: Every key, its type, and where it's valid
  - to: /reference/cli/plan
    type: relates-to
    description: The command that reads a pipeline file and checks it without spending
  - to: /spec#9-pipelineyaml--decided
    type: decided-by
    description: The normative grammar and schema rules for pipeline.yaml
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: Everything plan checks, step by step
  - to: /decisions#adr-018
    type: decided-by
    description: Why mapping to a vendor's field names happens only at the source and at deliver steps
  - to: /decisions#adr-019
    type: decided-by
    description: Why uses and variables are declared, so plan can check fields a manifest can't know
  - to: /decisions#adr-031
    type: decided-by
    description: Why a deliver step is an ordinary step at any position
---

# A pipeline is a YAML file

Here's `cache.yaml`, an example that ships with gtme, minus its header comment:

```yaml
name: cache
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv          # three fictional people, beside this file
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

steps:
  - id: score
    use: demo/enrich            # $0.01 per record, pretend; cached for 30 days
    with:
      cost_per_record_usd: 0.01

  - id: keep                    # deterministic judgment in SQL: no model, no spend
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
    idempotency: email          # re-runs append nothing twice
```

It reads three people from a CSV, scores each one, keeps anyone scoring 70 or more, and writes the keepers to `out.csv`. If you ran [See it run](/start/show-me), the file is already in your folder, and if it isn't, that page has the two download lines. Ask plan what it read. It spends nothing and sends nothing:

```sh
gtme plan cache.yaml
```

The output is the following:

```
pipeline cache (version 1)

1. source [source] — csv/source@1
     entity:    person
     writes:    works_at → company (from company_domain)
     provides:  company_domain, email, full_name, title
     est/record: ?

2. score [enrich] — demo/enrich@1
     entity:    person
     projects:  email, full_name
     requires:  any of email | full_name
     provides:  demo.note, demo.score
     cache:     30d
     est/record: $0.0100

3. keep [filter] — sql/filter
     entity:    person
     projects:  (none)
     provides:  (none)
     est/record: ?

4. out [deliver] — csv/deliver@1
     record:    touched → cache
     entity:    person
     projects:  demo.note, demo.score
     requires:  demo.note, demo.score
     provides:  (none)
     idempotency: email
     variables: note ← demo.note, score ← demo.score
     on_missing: skip
     note:      needs vendor-namespaced field "demo.note" — this pipeline is coupled to that vendor
     note:      needs vendor-namespaced field "demo.score" — this pipeline is coupled to that vendor
     est/record: $0.0000

send surface: 1 deliver step(s) (ADR-031)
  out → csv/deliver (touch scope: cache)

available fields after the last step: company_domain, demo.note, demo.score, email, full_name, title
plan ok — nothing has been spent
```

Each entry is a step, with its role in brackets and its adapter's version after `@`. `projects:` is what the step reads, its *projection*: the current value of each field it asked for. `requires:` must already exist, and `provides:` is what the step writes. `est/record:` is the adapter's price per record, and `?` means it publishes none. `sql/filter` and `csv/source` cost nothing, and an AI step's spend shows on the [receipt](/concepts/runs-and-receipts).

`writes:` is a relation the source records between each person and their company. `touched → cache` scopes a delivery to this pipeline, so another pipeline can still deliver the same person. `send surface` lists every step that sends. The two `note:` lines aren't a problem to fix. `demo.score` is namespaced, its name prefixed with the vendor's, and plan is telling you this file depends on `demo/enrich`.

## What you just saw

**Two top-level keys name the file and two do the work.** `name` is required, and it's recorded on every run and prefixes the fields the pipeline declares for itself. `version` is the format version, `1`. `source:` brings records in, and `steps:` does everything after that.

**The source and every step name an [adapter](/concepts/adapter-tiers) with `use:` and configure it with `with:`.** Plan checks everything under `with:` against the keys the adapter accepts, so a misspelled key fails before the run starts. On a source, `columns:` maps your CSV headers to [canonical field](/concepts/canonical-fields) names, which is how `Company Website` arrives as `company_domain`. `limit:` caps how many records a source emits.

**Steps run strictly in order, top to bottom.** Each step's `id` labels its row in the plan and the receipt. Steps don't hand records to each other. Each reads from the [ledger](/concepts/ledger) and writes back, so `keep` reads the `demo.score` that `score` wrote.

A deliver step like `out` is an ordinary entry, so a pipeline can have none, one, or several, anywhere in the list (the decision record, [ADR-031](/decisions#adr-031)).

**A step's other keys say what it reads, writes, and remembers, and when it runs.** Which keys are valid depends on the adapter's role, and plan rejects the rest.

| Key | Valid on | What it does |
|---|---|---|
| `uses:` | Filter, compose, and review steps, as [Steps and roles](/concepts/steps-and-roles) defines them | Lists the fields the step reads. Plan checks that an earlier step provides each one, and the step sees only these. |
| `provides:` | The same steps | Declares the fields the step writes. A bare name like `subject` lands as `cache.subject`. |
| `template:` under `with:` | The same steps | Holds the step's text: a prompt for an `ai/*` step, or copy rendered per record for `text/compose`. It's a string or `{file: path}`. |
| `when:` | Any step after the source | Takes `STEP_ID.passed`, where `STEP_ID` is an earlier filter step. A fail verdict already stops a record at the filter, so `when:` holds the records it never judged, like one skipped for a missing field. It also shows a reviewer that the paid step comes after the judgment. |
| `cache:` | Enrich, verify, and AI steps | Sets a freshness window like `30d`, overriding the adapter's default. Enrich and verify steps skip a record whose value is still current, and an AI step reuses a judgment only that long. |
| `variables:` | Deliver steps | Maps the target's field names to ledger fields, as in `score: demo.score`. |
| `idempotency:` | Deliver steps | Names the field that keys a delivery, defaulting to the [identity key](/concepts/identity-keys). A later run skips the same value to the same target (the exception is a target that updates in place, [ADR-045](/decisions#adr-045)). |

## So what?

**Plan checks every field name before anything spends.** Here's a `text/compose` step added to `cache.yaml` between `keep` and `out`. It writes an email subject from a template, at no cost:

```yaml
  - id: subject
    use: text/compose
    uses: [full_name, company_name]
    provides: [subject]
    with:
      template: "{{ record.full_name }}, a note for {{ record.company_name }}"
```

```sh
gtme plan cache.yaml
```

The output is the following:

```
gtme: step "subject": needs company_name, which no earlier step provides (available: company_domain, demo.note, demo.score, email, full_name, title); installed adapters provide it: company_name ← apollo/enrich|apollo/search
```

The CSV has a company domain and no company name, so nothing upstream provides it. Change both mentions of `company_name` to `company_domain`, run plan again, and the new step reads:

```
4. subject [compose] — text/compose@1
     entity:    person
     projects:  full_name, company_domain
     requires:  full_name, company_domain
     provides:  cache.subject
     est/record: ?
```

`provides: [subject]` became `cache.subject`. To write it to the file, add `subject: cache.subject` under the `out` step's `variables:`. That step and that `variables:` line are what you'd hand your agent.

Running the edited file again doesn't write Jane a second time, because `out.csv` isn't a target that updates in place. `gtme help --agent` prints every installed adapter's keys and fields, the same list as the [Adapter catalog](/reference/adapters).

That's it. Whatever file you or your agent writes next gets the same check before it spends anything.

## Why it's this way

**Fields a prompt or template mentions are declared, so plan can check them.** An adapter's manifest, its declaration of what it needs and provides, can't know which fields a prompt or an email template mentions. [ADR-004](/decisions#adr-004) added `uses:` for AI steps, and [ADR-019](/decisions#adr-019) applied the same rule to `variables:` on deliver steps. The cost is typing a field name twice, in the text and in `uses:`.

**A vendor's field names appear only at the two ends.** `columns:` on the source and `variables:` on a deliver step are the only places a file maps someone else's names to gtme's ([ADR-018](/decisions#adr-018)). Every step between them uses canonical or namespaced names, so plan can prove the file holds together. A computed field gets its own `sql/transform` step.

**The runner accepts only the keys the schema lists.** SPEC.md [§9](/spec#9-pipelineyaml--decided) is the whole format, with no loops and no branches. We think that's the right trade for a file an agent writes and a person approves.

The gap we notice is routing the records a filter rejected. Because `when:` names only `.passed`, sending rejects somewhere else takes a second `sql/filter` over the verdict the ledger already holds. That works, and reads less plainly than a `.failed` gate would.

## Where it shows up

- [Build your first pipeline from a CSV](/guides/first-pipeline) writes a file like this one from your own rows.
- [The gate ladder](/concepts/gate-ladder) takes a file from plan to a run that spends and sends.
- [Groups and segments](/concepts/groups) covers the keys this page leaves out: top-level `group:`, and `require:` and `exclude:` on a step.
- [pipeline.yaml keys](/reference/pipeline-yaml) and [`gtme plan`](/reference/cli/plan) are the lookup pages.
