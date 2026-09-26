---
name: Types and traverse
description: A type is a file that says what kind of record something is and how it's keyed, and a traverse step turns records of one type into related records of another
for: "You want a pipeline that starts from people and ends on their companies, or on posts and the people who reacted to them, and you want to predict what the run keeps."
learn:
  - "what a type file declares, and the difference between a subject and a signal"
  - "what a traverse step does to the records in a run, and how its receipt line reads"
  - "why a relation is a fact and group membership is a decision"
order: 10
roles: [builder]
links:
  - to: /concepts/identity-keys
    type: depends-on
    description: A type file lists its key tiers, and that page explains what a tier is and the order they're tried in
  - to: /concepts/groups
    type: relates-to
    description: A group holds one type, the type of the run's last leg; everything else about groups lives there
  - to: /concepts/ledger
    type: relates-to
    description: The identities a traverse mints and the relations it follows are rows in the ledger's identity layer
  - to: /concepts/adapter-tiers
    type: relates-to
    description: A vendor traverse is a binding shaped like a source, with a from type and a relation
  - to: /concepts/canonical-fields
    type: relates-to
    description: The type file is the same file as the canonical field registry for that type
  - to: /concepts/steps-and-roles
    type: relates-to
    description: Traverse is the eighth role, and that page covers the other seven
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The traverse line sits under the receipt table that page explains
  - to: /guides/traverse
    type: relates-to
    description: Builds a traverse into your own pipeline, step by step
  - to: /guides/multi-stage
    type: relates-to
    description: The two-pipeline form, for a human or a schedule between stages
  - to: /reference/fields
    type: relates-to
    description: Every type file and its fields
  - to: /decisions#adr-054
    type: decided-by
    description: Types as files, subjects and signals, the traverse role, typed legs, and typed groups
  - to: /decisions#adr-037
    type: decided-by
    description: The chain of pipelines joined by groups that a traverse replaced as the default
  - to: /spec#4a-canonical-field-registry--decided
    type: decided-by
    description: What a type file declares, where types are discovered, and the three adapter-type checks
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: How plan walks typed legs and checks a group's type
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The traverse receipt line and what each of its counts means
---

# Types and traverse

Here's `companies.yaml`. It crosses from the three people in the repo's `examples/contacts.csv` to their companies:

```yaml
name: companies
version: 1
source:
  use: csv/source
  with:
    path: contacts.csv
    columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }
steps:
  - id: to-company
    use: sql/traverse
    with:
      entity_type: company
      query: >
        SELECT r.to_id AS identity_id, r.from_id AS parent_id
        FROM relations r WHERE r.relation = 'works_at'
group: target-accounts
```

The query follows each person's `works_at` link to a company. The person is the *parent* and the company is its *child*.

Ask plan what it read. It spends nothing and sends nothing:

```sh
gtme plan companies.yaml
```

The output is the following, trimmed:

```
pipeline companies (version 1)

1. source [source] — csv/source@1
     entity:    person
     writes:    works_at → company (from company_domain)
...

2. to-company [traverse] — sql/traverse
     traverse:  person → company via sql/traverse (follows an existing relation; mints nothing)
...

ends in group "target-accounts" as company (records that complete the run are added)
...
plan ok — nothing has been spent
```

`entity:` is the type of record the step works on. `writes:` is a relation the step records in the [ledger](/concepts/ledger): each person `works_at` a company. `traverse:` is the type change and its adapter; to mint is to create a new identity. `ends in group` names the `group:` a record that completes the run lands in, and the type it lands as. Now run it:

```sh
gtme run companies.yaml
```

The output is similar to the following:

```
run 01M3FBD01V4PXHS9XNA0JA487M — done
step        adapter       in  out  empty  cached  filtered  failed  cost  avoided
source      csv/source    0   3    -      0       -         -       $0    -
to-company  sql/traverse  3   3    -      0       -         -       $0    -
group "target-accounts": 3 record(s) added
to-company: 3 parent(s) in, 3 out, 0 empty — 3 traversed (company), 0 already in this run
total: $0 spent
```

The whole of `companies.yaml` is what you'd hand your agent, with the relation and `entity_type` changed to fit.

## What you just saw

**A type is a file that says what kind of record something is and how it's keyed.** Here's part of the repo's `spec/fields/person.json`:

```json
{
  "entity_type": "person",
  "version": 1,
  "kind": "subject",
...
  "identity": [
    {
      "field": "email"
    },
    {
      "field": "linkedin_url"
    },
...
      "name": "company_domain",
...
      "reference": {
        "type": "company",
        "relation": "works_at",
        "fields": [
          "company_domain",
          "company_name"
        ]
      }
```

`identity` lists the fields a record's [identity key](/concepts/identity-keys) can come from, strongest first. A `reference` says a field names a record of another type: `company_domain` names a company, related by `works_at`, which is the plan's `writes:` line. The file is also the [canonical field](/concepts/canonical-fields) registry for its type.

**There are two kinds of type.** A subject, `person` or `company`, has `kind: subject` and is what a pipeline delivers to. A signal, like `post`, is what a pipeline finds and traverses from. Plan refuses a deliver step, one that writes to a CRM or a sequencer, aimed at a signal.

**A traverse step takes records of one type in and emits related records of a type out.** It's the eighth role in [Steps and roles](/concepts/steps-and-roles). After `to-company`, only companies move forward. Each stretch between type changes is a *leg*.

**The traverse line counts parents and children separately.** `in` counts parents, `out` counts parents that yielded at least one child, and `empty` counts parents that yielded none. `traversed` counts children. `already in this run` counts children another parent brought in first, so each stays one record.

The `posts-to-engagers` [bundle](/concepts/campaign-is-a-folder) goes from people to their posts to the people who reacted. Its `posts` and `engagers` steps are paid vendor calls, so these lines come from an offline `--simulate` run ([the gate ladder](/concepts/gate-ladder)):

```
posts: 2 parent(s) in, 2 out, 0 empty — 2 traversed (post), 1 already in this run
engagers: 2 parent(s) in, 2 out, 0 empty — 4 traversed (person), 2 already in this run
```

Bob's one post is a repost of Jane's, so it was already in the run. Among the engagers, Bob came in from the source and Dave reacted to both posts, so the receipt counts each as already in this run.

**`sql/traverse` is the built-in traverse, and it's free.** It follows a relation the ledger already holds, mints nothing, and reruns its query every run. A vendor traverse, like the bundle's `harvest/profile-posts`, is a [binding](/concepts/adapter-tiers) shaped like a source, with a `from:` type and a `relation:` it writes, and it spends like one.

That's it. A run is a list of typed legs, and a traverse is the only thing that changes the type.

## So what?

**Pick the association by what it means.** A relation is a fact, recorded per record and readable from any SQL step. Group membership is a decision, added and removed on purpose, with each change logged. Who works where is a relation. Which accounts you're targeting is a group.

## Why it's this way

**People to posts to engagers used to take three pipelines.** They were chained by groups (the decision record, [ADR-037](/decisions#adr-037)), with the type crossing hidden in a SQL string. [ADR-054](/decisions#adr-054) put the chain in one file, where plan prints every type change. Two pipelines still fit when a person or a schedule belongs between stages.

**Types are discovered like adapters.** A type is built in, placed in `~/.gtme/types/`, or shipped beside a binding. No binding can redefine how a person, company, or post is keyed. Plan fails any source or traverse that can't key what it emits, before anything is billed ([SPEC §4a](/spec#4a-canonical-field-registry--decided), [§7](/spec#7-contract-validation--the-planner--decided)).

**Few candidates qualify as types.** It needs a key that dedupes across sources, fields that mean the same thing to every adapter, and records that are what a run works on.

| Not a type | What it is instead |
|---|---|
| Account | A company in a group |
| Deal | A CRM's record, whose stages the CRM owns |
| Campaign | A pipeline's own fields, prefixed with its name, plus a group |
| Segment | A saved query over the ledger |
| Reply, open, bounce, meeting | An event about an identity you already have |
| Persona, offer, value proposition | Content a prompt reads, versioned as a file |

**Relations can't end yet.** `works_at` can't say someone left. A related record's field can't be named in `uses:` or `variables:`, the fields a prompt reads. To put a company's industry in a person's email, copy it onto the person first with a `sql/transform` step.

## Where it shows up

- [Traverse a relation](/guides/traverse) adds a traverse to your own pipeline.
- [Multi-stage campaigns with groups](/guides/multi-stage) is the two-pipeline form.
- [Canonical field registry](/reference/fields) lists every type file and its fields.
