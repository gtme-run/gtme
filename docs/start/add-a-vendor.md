---
name: Add a vendor
description: Write an adapter for an API gtme doesn't ship as one YAML file, verify it offline against a recorded response, and simulate a pipeline through it
for: "You use a vendor gtme doesn't ship and want to see what an adapter is before handing the job to Claude Code."
learn:
  - "search the registry before writing anything"
  - "write a binding from an API doc, record a fixture, and verify it offline"
  - "where an API key goes"
order: 5
roles: [extender]
links:
  - to: /start/my-stack
    type: relates-to
    description: The previous page runs vendors gtme already ships; this one adds a vendor it doesn't
  - to: /start/for-agents
    type: relates-to
    description: The next page hands the same work to an agent, which reads the binding contract and writes the file for you
  - to: /concepts/adapter-tiers
    type: relates-to
    description: A binding is the declarative tier of adapter; a process adapter is the tier for integrations that need logic
  - to: /concepts/canonical-fields
    type: relates-to
    description: A binding's extracted fields are canonical names or vendor-namespaced ones, which is what lets later steps use them
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate is the rung that answers the binding's requests from its fixtures instead of the network
  - to: /guides/add-a-binding
    type: relates-to
    description: The full walkthrough for a real vendor, with auth, pagination, errors, and cost
  - to: /reference/binding-manifest
    type: relates-to
    description: Every key a binding.yaml can declare, including the transform rules
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: A binding is data the engine interprets, and anything that needs logic graduates to a process adapter
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: Where a declared credential comes from, the environment or gtme secret set, and never the manifest
  - to: /spec#4a-canonical-field-registry--decided
    type: decided-by
    description: The named normalization rules a binding's transform may use
  - to: /decisions#adr-042
    type: decided-by
    description: Bindings live in a registry that gtme adapters search and add read, and verify gates every install
  - to: /decisions#adr-047
    type: decided-by
    description: Why limit works on every source binding whether or not the binding declares it
---

# Add a vendor

**This needs gtme, `jq`, and one network request to a free public API.** No keys, no accounts. If you don't have gtme yet, [install it](/start/install) first.

**It spends $0 and sends nothing.** If gtme already ships your vendors, go to [Your stack](/start/my-stack) instead.

Most people have Claude Code write the adapter. This page shows what it writes so you can read it and check it, and [the last section](#hand-it-to-claude-code) gives you the line to hand over.

For a vendor API over plain HTTP, an [adapter](/concepts/adapter-tiers) is a *binding*: one YAML file the runner interprets, with no code in it. This page uses JSONPlaceholder, a free fake API, so every step runs as written.

## Write and verify a binding

1. Search the registry, the [gtme-bindings repository](https://github.com/gtme-run/gtme-bindings) on GitHub where people submit the bindings they've written:

    ```sh
    gtme adapters search jsonplaceholder
    ```

    ```
    no registry entries match "jsonplaceholder" (index: https://raw.githubusercontent.com/gtme-run/gtme-bindings/main/index.json)
    an agent that finds nothing writes a binding — `gtme help --bindings`
    ```

    A match prints its install command. `gtme adapters add` verifies a binding before installing, then pins it: it records the exact version, so an update is a choice you make ([ADR-042](/decisions#adr-042)).

1. Save the contract, one JSON document:

    ```sh
    gtme help --bindings > bindings.json
    ```

    Its `schema` key lists every key a binding may use, and `reference` is a complete binding gtme ships, ready to copy.

1. Make the adapter's folder. Its name is the adapter's id with the slash as a dash:

    ```sh
    mkdir -p ~/.gtme/adapters/jsonplaceholder-users/fixtures
    cd ~/.gtme/adapters/jsonplaceholder-users
    ```

    This is your real adapters folder, so every pipeline on this machine can use the adapter. To remove it later, run `rm -r ~/.gtme/adapters/jsonplaceholder-users`.

1. Write `binding.yaml`:

    ```yaml
    # jsonplaceholder/users: people from JSONPlaceholder's free fake API.
    # No auth, no cost. Docs: https://jsonplaceholder.typicode.com
    # Lines marked "yours" change for your vendor. The rest you copy.

    id: jsonplaceholder/users # yours: vendor/operation
    version: 1
    role: source # yours: source, enrich, or deliver
    entity_type: person # yours: person or company

    # provides: the fields this adapter hands to later steps. Yours.
    provides:
      type: object
      additionalProperties: false
      properties:
        full_name: { type: string }
        email: { type: string }
        company_name: { type: string }
        company_domain: { type: string }
        jsonplaceholder.username: { type: string }

    # config_schema: the settings a pipeline can pass under with:.
    config_schema:
      type: object
      properties:
        base_url: { type: string, default: "https://jsonplaceholder.typicode.com" } # yours: the vendor's host

    request:
      method: GET # yours
      url: "{{config.base_url}}/users" # yours: the endpoint's path

    # extract: where each field sits in the response. Yours.
    extract:
      records: "." # the list of records; "." means the whole response
      fields:
        full_name: name
        email: { path: email, transform: email } # transform: a cleanup rule from SPEC §4a
        company_name: company.name
        company_domain: { path: website, transform: domain }
        jsonplaceholder.username: username
    ```

    `extract` maps each response field to a [canonical field](/concepts/canonical-fields). Fields with no canonical name get the vendor's prefix. The valid `transform` rules are the normalization rules in [SPEC §4a](/spec#4a-canonical-field-registry--decided), and [Binding manifest](/reference/binding-manifest) lists them with every other key.

1. Record one real response as the fixture:

    ```sh
    curl -fsS https://jsonplaceholder.typicode.com/users \
        | jq '{responses: [{match: "GET /users", status: 200, body: .}]}' \
        > fixtures/conformance.json
    ```

    Against a paid vendor, this `curl` is one real call, so it costs whatever one call costs. After that, verify and simulate never call the vendor again.

1. Verify the binding. Verify prints what it would call and what it would ask for:

    ```sh
    gtme adapters verify jsonplaceholder/users
    ```

    ```
    jsonplaceholder/users v1 — source (person)
      calls:       jsonplaceholder.typicode.com
      demands:     none
      needs:       none
      provides:    company_domain, company_name, email, full_name, jsonplaceholder.username (* required)
      fixtures:    ok — 1 response(s) on file, 10 record(s) extracted
    ```

    Verify checks the schema and runs the fixture offline. For a vendor with a key, the binding names it under `credentials`, verify prints it under `demands`, and `gtme secret set` stores it. The key itself never goes in the YAML ([SPEC §6](/spec#6-adapter-manifest--decided)).

## Run a pipeline through it

1. In any folder, save this as `my-pipeline.yaml`:

    ```yaml
    name: my-pipeline
    version: 1

    source:
      use: jsonplaceholder/users
      with:
        limit: 3

    steps:
      - id: out
        use: csv/deliver
        with:
          path: out.csv
        variables:
          company: company_name
        idempotency: email
    ```

    `limit` isn't in the binding: gtme accepts it on every source and stops after that many records ([ADR-047](/decisions#adr-047)). `idempotency: email` is the [idempotency](/concepts/runs-and-receipts) rule: one delivery per email to this target, so a rerun never delivers anyone twice.

1. Run it under [simulate](/concepts/gate-ladder), which answers from your fixture:

    ```sh
    gtme run my-pipeline.yaml --simulate
    ```

    The receipt at the end is similar to the following:

    ```
    simulate: fixtures only — no network, no spend, nothing sends, nothing persists
    run 01M3CX29SWR8TS6ZEW6XEXWJBP (my-pipeline)
    source [info]: jsonplaceholder/users: 3 records
    source: sourced 3 records
    out: 3 in, 0 out, 0 cached, 0 filtered, 0 failed, 3 held (dry run)

    run 01M3CX29SWR8TS6ZEW6XEXWJBP — done (SIMULATED — fixtures only; nothing sent, nothing persisted)
    step    adapter                in  out  empty  cached  filtered  failed  cost  avoided
    source  jsonplaceholder/users  0   3    -      0       -         -       $0    -
    out     csv/deliver            3   0    -      0       -         -       $0    -
    out: resolved variables for 3 record(s) — review, then run again without --dry-run to arm:
      sincere@april.biz
        company: "Romaguera-Crona"
    ...
    total: $0 spent
    ```

**That's a new vendor in gtme.** The `source` row shows 3 records from `jsonplaceholder/users`, served from your fixture, and `company` came through your `extract` mapping. `held (dry run)` and "run again without --dry-run to arm" are the CLI's words under simulate too. Arming means running with no flag at all.

A real vendor adds auth, pagination, and cost to the same file, and [Add a vendor with a binding](/guides/add-a-binding) walks through one. An integration that needs conditionals, several calls per record, or an OAuth flow becomes a process adapter, a small program in any language ([SPEC §10a](/spec#10a-the-binding-tier--universal-steps--decided-adr-022027)).

## Hand it to Claude Code

**Paste this line into Claude Code for your own vendor:**

```text
Follow gtme.run/start.md, "Add a vendor": add VENDOR using its API docs at DOCS_URL.
```

Replace the following:

- `VENDOR`: the vendor's name
- `DOCS_URL`: the page that documents the endpoint you want

The agent writes the same folder and stops at a verified binding and a simulated receipt for you to read. With the gtme plugin installed, `/gtme:create-adapter` runs the same steps.

## Next

**[For agents](/start/for-agents) has the plugin install and the rules that keep an agent from spending or sending until you say so.**
