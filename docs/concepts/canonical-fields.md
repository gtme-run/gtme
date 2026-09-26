---
name: Canonical fields
description: A canonical field is one name with one meaning across every adapter, and any other field name carries a vendor's or a pipeline's prefix
for: "You've seen `demo.score` or `hello.first_line` beside `email` in the ledger and want to know who writes which, or `gtme plan` just said your pipeline is coupled to a vendor."
learn:
  - "the three kinds of field name, and who writes each"
  - "how `canonical: true` lands a step's output on a canonical field"
  - "what plan's coupling note means, and when to act on it"
order: 7
roles: [builder]
links:
  - to: /concepts/pipeline
    type: depends-on
    description: The file whose columns, provides, and variables name every field on this page
  - to: /concepts/ledger
    type: depends-on
    description: Every fact is stored in the ledger under one of these field names
  - to: /concepts/steps-and-roles
    type: relates-to
    description: Filter, compose, and review steps are the ones that declare provides and write pipeline-namespaced fields
  - to: /concepts/identity-keys
    type: relates-to
    description: The identity-tier canonical fields are what a record's key is built from
  - to: /concepts/facts
    type: relates-to
    description: What decides the current value when two adapters write the same canonical field
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Built-in adapters and bindings both map a vendor's names to canonical ones at their own boundary
  - to: /reference/fields
    type: relates-to
    description: Every canonical field per record type, with its type and normalization rule
  - to: /reference/adapters
    type: relates-to
    description: Each shipped adapter's provides, canonical and namespaced
  - to: /reference/cli/plan
    type: relates-to
    description: The command that prints the namespace and coupling notes
  - to: /guides/add-a-binding
    type: relates-to
    description: A new vendor's provides have to be canonical or namespaced before plan accepts them
  - to: /spec#4a-canonical-field-registry--decided
    type: decided-by
    description: The registry, the three tiers, the rule of two, and how names are enforced
  - to: /decisions#adr-017
    type: decided-by
    description: Why adapters share one vocabulary of field names
  - to: /decisions#adr-018
    type: decided-by
    description: Why mapping to a vendor's names happens only at the source and at deliver steps
  - to: /decisions#adr-033
    type: decided-by
    description: Why a step's declared outputs land under the pipeline's name unless marked canonical
---

# Canonical fields

After you add one `text/compose` step to `hello.yaml` from [See it run](/start/show-me) and run it, here's every field name the [ledger](/concepts/ledger) holds and what wrote it:

```sh
gtme query "SELECT DISTINCT field, source FROM current_values ORDER BY field"
```

The output is the following:

```
{"field":"hello.first_line","source":"text/compose @ #fa5f59e6f6ac"}
{"field":"company_domain","source":"csv/source@1"}
{"field":"demo.note","source":"demo/enrich@1"}
{"field":"demo.score","source":"demo/enrich@1"}
{"field":"email","source":"csv/source@1"}
{"field":"full_name","source":"csv/source@1"}
{"field":"title","source":"csv/source@1"}
7 rows
```

`current_values` is the ledger's view of each field's winning value. `field` is the name a fact is stored under. `source` is what wrote it, an adapter with its version after `@`. For `text/compose`, it's a signature after `#`, a fingerprint of the step's template and settings, so a changed template counts as a new source.

`hello.first_line` came from this step between `keep` and `out`, which writes an opening line from a template:

```yaml
  - id: line
    use: text/compose
    uses: [full_name, title]
    provides: [first_line]
    with:
      template: "{{ record.full_name }}, a quick note for a {{ record.title }}."
```

Add it and run the file. It needs no keys, calls no vendor, and writes only to `out.csv`:

```sh
gtme run hello.yaml
```

## What you just saw

**Every field name is one of three kinds, and a dot tells you which.**

| Kind | Looks like | Who writes it |
|---|---|---|
| Canonical | `email`, `title`, `company_domain` | Any adapter, in the registry's form, and a step's output marked `canonical: true` |
| Vendor-namespaced | `demo.score`, `apollo.id` | The vendor's adapter, for a fact only that vendor has (a convention plan doesn't check) |
| Pipeline-namespaced | `hello.first_line` | A step in the pipeline named `hello` that declares `provides:` |

**A canonical field is one name with one meaning, whichever [adapter](/concepts/adapter-tiers) wrote it.** Two adapters can write the same canonical field, like `title` from your CSV and from Apollo's enrich step. The ledger keeps both rows, and at equal confidence a step reads the newest. The opening query's `source` column shows who wrote the winner, and [Facts have provenance](/concepts/facts) covers which value wins and how to change it. Two vendors can still write different words for the same `seniority`, and the registry only lowercases them.

The names live in a registry, one file per record type. Here's one entry from `spec/fields/person.json`:

```json
{
  "name": "first_line",
  "tier": "core",
  "type": "string",
  "normalization": "trim",
  "description": "Composed personalized opening line, consumed by deliver steps.",
  "example": "Loved your post on onboarding flows —"
}
```

Every entry fixes a type and a normalization rule, the function that puts a value in its stored form. `company_domain` uses `domain`, which is why Jane's `https://www.acme.com` is stored as `acme.com`. Fields like `email` that an [identity key](/concepts/identity-keys) is built from are tier `identity`, and the rest are `core`.

**One adapter can write both kinds.** `apollo/search` writes `title` under its canonical name and `apollo.id` under Apollo's, because no other vendor has Apollo's record id.

**A pipeline-namespaced field is a judgment one campaign made.** `first_line` is in the registry, yet the step's output landed as `hello.first_line`. A bare name in a step's `provides:` always takes the pipeline's name as its prefix, so two campaigns can each write Jane an opening line without overwriting each other.

## The canonical flag

**Plan tells you when a declared name matches a canonical one.** Run `gtme plan hello.yaml`, and the `line` block reads:

```
4. line [compose] — text/compose@1
     entity:    person
     reads:     full_name, title
     requires:  full_name, title
     provides:  hello.first_line
     note:      provides: "first_line" lands as "hello.first_line" (per-campaign, ADR-033); the canonical person field "first_line" is untouched — add canonical: true to write it instead
     est/record: ?
```

`note:` names the canonical field the step isn't touching. The other labels are read on [A pipeline is a YAML file](/concepts/pipeline).

To write the shared field, mark it in the step:

```yaml
    provides:
      first_line: {canonical: true}
```

Plan again, and the note is gone, and `provides:` reads `first_line`.

The name has to be in the registry. Mark a made-up name like `opener` canonical and plan exits with code 2:

```
gtme: step "line": provides: "opener" is marked canonical but is not a canonical person field (see spec/fields/person.json)
```

We recommend `canonical: true` for a fact every campaign should share, and the default otherwise. An opening line is usually one campaign's judgment, so in practice this one stays bare.

## Vendor coupling in the plan

**Plan notes each namespaced field a step declares it needs.** With `provides:` bare again, add one line to the `out` step's `variables:`:

```yaml
    variables:
      score: demo.score
      note: demo.note
      line: hello.first_line
```

Plan again, and the `out` block carries three notes:

```
5. out [deliver] — csv/deliver@1
...
     note:      needs this pipeline's own judgment field "hello.first_line" (declared by an earlier AI step, ADR-033)
     note:      needs vendor-namespaced field "demo.note" — this pipeline is coupled to that vendor
     note:      needs vendor-namespaced field "demo.score" — this pipeline is coupled to that vendor
...
```

None is an error, and plan still passes. The first says the file writes that field itself. The other two say the file works only while `demo/enrich` is in it. Swap it for another scorer and plan fails at `out`, naming `demo.note` and `demo.score`.

A canonical `title` can come from the CSV today and `apollo/enrich` next month, and `out` still plans. Plan notes a namespaced need on any step whose needs come from `uses:` or `variables:`: an AI or compose step, or a deliver step. Each note is a line you'd edit to change vendors.

## Why it's this way

**Adapters match fields by exact name, so they have to agree on the names.** The decision record, [ADR-017](/decisions#adr-017), cites Singer, an open-source connector standard that fixed how tools talk and left field names to each vendor, so its parts didn't compose.

**Each adapter maps a vendor's names to canonical ones at its own boundary.** That's [ADR-018](/decisions#adr-018), and the pipeline page covers why a file names a vendor only at its two ends.

**Facts are shared and judgments are per campaign.** That's [ADR-033](/decisions#adr-033).

**The registry grows by the rule of two.** A namespaced field becomes canonical when a second adapter provides the same fact. Adding a field leaves existing pipelines working, and renaming one would break them ([SPEC §4a](/spec#4a-canonical-field-registry--decided)). The cost is that a one-vendor fact keeps that vendor's name in your file.

The registry declares no allowed values yet. We'd rather wait for real vendors to converge than guess a list and break it later.

## Where it shows up

- [Canonical field registry](/reference/fields) lists every field per record type.
- [Add a vendor with a binding](/guides/add-a-binding) maps a new vendor onto these names.
- [Adapter catalog](/reference/adapters) and [`gtme plan`](/reference/cli/plan) are the lookup pages.
