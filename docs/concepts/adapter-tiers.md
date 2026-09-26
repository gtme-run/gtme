---
name: Two adapter tiers
description: An adapter is what a step's use line names, either a YAML binding the runner interprets or a program speaking NDJSON, with one manifest surface
for: "You need a vendor gtme doesn't ship, or you're reading someone else's adapter and want to know what it can do before you install it."
learn:
  - "what a binding can and can't express, and when an integration becomes a process adapter"
  - "how the registry installs a binding pinned and verified"
  - "why plan treats both tiers the same, and what simulate does with each"
  - "what the built-in floor adapters are for"
order: 13
roles: [extender, builder]
links:
  - to: /concepts/steps-and-roles
    type: depends-on
    description: An adapter's manifest names its role, and roles work the same way in both tiers
  - to: /concepts/pipeline
    type: relates-to
    description: A step's use line names the adapter and its with block is checked against the adapter's config schema
  - to: /concepts/canonical-fields
    type: relates-to
    description: A binding's extract block lands each response field on a canonical or vendor-namespaced name
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate answers every binding from the fixtures it has to ship
  - to: /concepts/types-and-traverse
    type: relates-to
    description: A traverse binding is a source-shaped binding that also declares from and relation
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A bundle carries each binding at its pin, and a process adapter doesn't travel in one
  - to: /start/add-a-vendor
    type: relates-to
    description: Writes a binding by hand, records its fixture, and simulates a pipeline through it
  - to: /guides/add-a-binding
    type: relates-to
    description: The full binding walkthrough for a real vendor, with auth, pagination, errors, and cost
  - to: /guides/process-adapter
    type: relates-to
    description: Writes the second tier, a program that speaks the wire protocol
  - to: /guides/publish-to-registry
    type: relates-to
    description: Lists a binding in the registry index so gtme adapters search finds it
  - to: /reference/binding-manifest
    type: relates-to
    description: Every key a binding.yaml can declare
  - to: /reference/wire-protocol
    type: relates-to
    description: Every NDJSON message a process adapter reads and writes
  - to: /reference/cli/adapters
    type: relates-to
    description: The search, add, verify, and update verbs
  - to: /reference/adapters
    type: relates-to
    description: Every adapter the binary ships, with its role and kind
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: The two tiers, the graduation rule, engine unification, and the floor
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: The manifest surface both tiers present, and the folder the runner discovers them in
  - to: /decisions#adr-022
    type: decided-by
    description: Why vendors are YAML an engine interprets, and why a binding can never run code
  - to: /decisions#adr-042
    type: decided-by
    description: Why bindings live in a registry, pinned by URL and verified before they install
---

# Two adapter tiers

Here's a vendor the gtme binary doesn't ship. Search the registry, a public index of bindings on GitHub. This spends nothing and sends nothing:

```sh
gtme adapters search hubspot
```

The output is the following:

```
ID                      ROLE    TIER      INSTALL                                                                          DESCRIPTION
hubspot/contact-search  source  verified  gtme adapters add github.com/gtme-run/gtme-bindings/hubspot-contact-search@main  Source HubSpot contacts via the CRM v3 Search API, filtered…
```

Install it with the line in the `INSTALL` column:

```sh
gtme adapters add github.com/gtme-run/gtme-bindings/hubspot-contact-search@main
```

The output is similar to the following:

```
fetched github.com/gtme-run/gtme-bindings/hubspot-contact-search@main at af95211c7c3d
hubspot/contact-search v1 — source (person)
  calls:       api.hubapi.com
  demands:     HUBSPOT_ACCESS_TOKEN
  needs:       none
  provides:    company_name, email, first_name, hubspot.contact_id, hubspot.lifecycle_stage, last_name, title (* required)
  fixtures:    ok — 1 response(s) on file, 2 record(s) extracted
...
```

`add` verified the adapter before installing it. `calls:` is every host it contacts, `demands:` is its credential, and `needs:` and `provides:` are the fields it reads and writes (a `*` marks a required one). `fixtures:` says its recorded vendor response ran offline and produced records. In the search output, `TIER` is the registry's trust level: `verified` means the registry's own tests run the fixtures. The adapter's tier is `KIND`, in the next listing.

## What you just saw

**An adapter is whatever a step's `use:` line names, and it comes in two tiers.** Here's what's installed after also copying in the gtme repository's example process adapter, `adapters/mock-enrich-py`:

```sh
gtme adapters
```

The output is the following:

```
ID                      VERSION  ROLE    KIND     SOURCE
hubspot/contact-search  1        source  binding  github.com/gtme-run/gtme-bindings/hubspot-contact-search@main (af95211c7c3d)
mock-enrich-py          1        enrich  process  installed by hand
```

`SOURCE` shows the pinned commit, so `@main` only names the branch it came from. Both live in `~/.gtme/adapters/`.

**A binding is a YAML file the runner (`gtme` itself) interprets.** Here's the head of yours, trimmed:

```yaml
id: hubspot/contact-search
version: 1
role: source
entity_type: person

credentials: [HUBSPOT_ACCESS_TOKEN]
...
request:
  method: POST
  url: "{{config.base_url}}/crm/v3/objects/contacts/search"
  body:
    filterGroups:
      - filters:
          - propertyName: "{{config.known_property}}"
            operator: HAS_PROPERTY
...
extract:
  records: results
  fields:
    email:                   properties.email
    first_name:              properties.firstname
    last_name:               properties.lastname
    title:                   properties.jobtitle
    company_name:            properties.company
    hubspot.contact_id:      id
    hubspot.lifecycle_stage: properties.lifecyclestage
...
```

`request:` is a template filled from the step's `with:` values. `extract.fields` maps each response path to a [canonical field](/concepts/canonical-fields) or a `hubspot.` name. Auth, pagination, errors, retries, and cost are settings in the same file, and none of them can hold a formula or an if.

**A process adapter is a program**, here a folder holding `manifest.json` and `run`. Here's the manifest, trimmed:

```json
{
  "id": "mock-enrich-py",
  "version": 1,
  "role": "enrich",
  "entity_type": "person",
  "needs": {
...
  "provides": {
...
      "mock.score": {"type": "integer", "minimum": 0, "maximum": 100},
      "mock.note": {"type": "string"}
...
```

`run` is an executable in any language that reads and writes records as NDJSON, newline-delimited JSON with one object per line.

Here's the test for your vendor. A documented HTTP API you call with a key is a binding. Anything needing a conditional, several calls per record, OAuth or request signing (login schemes a fixed template can't perform), or its own computation is a process adapter.

**Both tiers present the same manifest surface**: id, version, [role](/concepts/steps-and-roles), entity type, needs, provides, the `with:` keys a step may set, and credentials. `gtme plan` reads only that surface. Neither tier touches [the ledger](/concepts/ledger); the runner writes what they return.

## So what?

**One pipeline can use both.** This file sources from the binding and scores with the process adapter:

```yaml
name: tiers
version: 1

source:
  use: hubspot/contact-search
  with:
    known_property: email
    limit: 2

steps:
  - id: score
    use: mock-enrich-py
```

`gtme plan tiers.yaml` stops here, because plan checks declared credentials ([setting one](/guides/connect-your-stack)):

```
gtme: step "source": missing credential HUBSPOT_ACCESS_TOKEN (set it in the environment or run `gtme secret set HUBSPOT_ACCESS_TOKEN`)
```

[Simulate](/concepts/gate-ladder) needs no key. It serves a binding from its fixture, and runs an uncredentialed process adapter like this one for real:

```sh
gtme run tiers.yaml --simulate
```

The [receipt](/concepts/runs-and-receipts) at the end is similar to the following:

```
run 01M3DMGEKBPTQ0VJ4VK74Q8678 — done (SIMULATED — fixtures only; nothing sent, nothing persisted)
step    adapter                 in  out  empty  cached  filtered  failed  cost  avoided
source  hubspot/contact-search  0   2    -      0       -         -       $0    -
score   mock-enrich-py          2   2    -      0       -         -       $0    -
total: $0 (estimated) spent
```

The `source` row is HubSpot's recorded response, and the `score` row is the Python program scoring it.

Hand your agent the `search` and `add` lines, because `add` installs nothing that fails verification. If your vendor isn't listed, hand it this, which prints the binding schema and a reference binding as JSON:

```sh
gtme help --bindings
```

That's it. That's how adapters work, and every vendor you add is one of these two.

## Why it's this way

**Most vendor APIs are plain HTTP, so most adapters are data.** The decision record, [ADR-022](/decisions#adr-022), runs every binding through one HTTP engine in the runner. A binding can't run code, so a stranger's binding is safe to read and `add` can list its hosts. SPEC calls the binding-or-program test the graduation rule ([SPEC §10a](/spec#10a-the-binding-tier--universal-steps--decided-adr-022027)).

**Vendors live in a registry outside the binary.** `gtme adapters update` moves a pin only when you ask, and every registry entry must ship fixtures ([ADR-042](/decisions#adr-042)).

**The binary ships a floor**: the `csv/*`, `http/*`, `sql/*`, `ai/*`, and `group/*` families (the last for [record groups](/concepts/groups)), plus the reference bindings in the repository's `spec/bindings/` folder. The floor is the crudest version of any integration on purpose, so anything is wireable today ([ADR-023](/decisions#adr-023)). Inline `http/*` config that repeats across pipelines should become a binding. We think that's the right trade: a pipeline runs on day one and improves when a binding replaces its floor step.

The binary also ships built-ins outside the floor: the [participant](/concepts/participants) adapters `human/*` and `agent/*`, `demo/enrich`, and `text/compose`.

**What it gave up.** A process adapter is code you install by hand, so read it first, and it can't travel inside a [bundle](/concepts/campaign-is-a-folder).

## Where it shows up

**Three guides and a start page write adapters, and four reference pages list them.**

- [Add a vendor](/start/add-a-vendor) and [Add a vendor with a binding](/guides/add-a-binding) write bindings.
- [Write a process adapter](/guides/process-adapter) and [Publish to the registry](/guides/publish-to-registry) cover the second tier and the index.
- [Binding manifest](/reference/binding-manifest), [Wire protocol](/reference/wire-protocol), [`gtme adapters`](/reference/cli/adapters), and the [Adapter catalog](/reference/adapters) are the lookup pages.
