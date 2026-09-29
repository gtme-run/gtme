---
name: Add a vendor with a binding
description: Turn one keyed, paginated vendor endpoint into a verified binding kept in your project, then plan, simulate, and run a pipeline through it
for: "You've read Add a vendor, your vendor isn't in the registry, and you want to write its binding by hand with auth, pagination, a list field, and cost."
learn:
  - "map a vendor response onto a binding's manifest, request, pagination, and extract blocks"
  - "turn a list in the response into a string array with each:"
  - "record fixtures that walk every page, and read what verify refuses"
  - "keep a binding in your project folder on GTME_ADAPTER_PATH"
order: 17
roles: [extender]
links:
  - to: /start/add-a-vendor
    type: depends-on
    description: The start page writes a keyless, single-page binding; this guide adds auth, pagination, a list field, errors, and cost
  - to: /concepts/adapter-tiers
    type: depends-on
    description: What a binding can express, and when an integration needs a process adapter instead
  - to: /concepts/canonical-fields
    type: relates-to
    description: Every field a binding provides is canonical for its entity type or carries the vendor's prefix, and verify refuses anything else
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan checks the credential, simulate answers from the fixtures, and armed calls the vendor
  - to: /concepts/pipeline
    type: relates-to
    description: The pipeline file whose source step names the binding by its id
  - to: /concepts/ledger
    type: relates-to
    description: The armed run writes each extracted field as a fact, and a list field stays a list
  - to: /concepts/types-and-traverse
    type: relates-to
    description: A traverse binding is this same source shape plus from and relation
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A bundle carries each binding it uses, with its fixtures
  - to: /reference/cli/adapters
    type: relates-to
    description: The verify verb this guide runs until it passes, and the verbs that install from the registry
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: The binding primitives, the each list form, and the fixtures every binding ships
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: The manifest surface a binding declares, and where credentials come from
  - to: /decisions#adr-041
    type: decided-by
    description: gtme help --bindings is the contract a binding author works from
  - to: /decisions#adr-042
    type: decided-by
    description: Nothing installs unverified, and verify runs a binding's fixtures offline
  - to: /decisions#adr-057
    type: decided-by
    description: The template dialect a binding's request and each template are written in
  - to: /decisions#adr-059
    type: decided-by
    description: The each extraction form that renders a list in the response as a string array
---

# Add a vendor with a binding

**You end with a verified binding for a keyed, paginated vendor endpoint and a [pipeline](/concepts/pipeline) that runs through it.**

## Before you start

- **Read [Add a vendor](/start/add-a-vendor) first.** It writes a keyless, single-page binding. This guide adds auth, pagination, a list field, errors, and cost. [Two adapter tiers](/concepts/adapter-tiers) has the test for when a vendor needs a process adapter instead.
- **You need** gtme, `curl`, `jq`, your vendor's API docs, and its key exported in your shell.
- **Spend:** four real requests to the vendor, one to look and one per page of fixtures. Verify, plan, and simulate call nothing. The optional armed [run](/concepts/runs-and-receipts) at the end spends what the vendor charges.
- **Sends:** nothing. The pipeline writes a local CSV.

Stacklens is an invented vendor that lists companies using a given technology. The outputs on this page came from a local stand-in for its API, at the address in `STACKLENS_URL`. Set that variable to your vendor's base URL; your paths and fields will differ too.

```sh
export STACKLENS_URL=http://127.0.0.1:8787
export STACKLENS_API_KEY=sk_test_standin
```

## Write the binding

1. Make one call to the endpoint and read what comes back:

    ```sh
    curl -fsS -H "Authorization: Bearer $STACKLENS_API_KEY" \
        "$STACKLENS_URL/v1/companies?technology=hubspot&page=1&per_page=2" | jq .
    ```

    The output is the following:

    ```json
    {
      "data": [
        {
          "id": "co_101",
          "name": "Acme Corp",
          "domain": "acme.com",
          "employees": 120,
          "industry": "Computer Software",
          "technologies": [
            {
              "name": "HubSpot",
              "category": "Marketing automation"
            },
            {
              "name": "Salesforce",
              "category": "CRM"
            },
            {
              "name": "Segment",
              "category": "Analytics"
            }
          ]
        },
    ...
      ],
      "meta": {
        "page": 1,
        "per_page": 2,
        "total_pages": 3
      }
    }
    ```

    Three things in this response shape the binding. The records sit under `data`, `meta.total_pages` says when to stop paging, and `technologies` is a list of objects.

1. Make the binding's folder inside your project, and put its parent on the adapter search path:

    ```sh
    mkdir -p adapters/stacklens-companies/fixtures
    export GTME_ADAPTER_PATH=$PWD/adapters
    ```

    gtme searches `GTME_ADAPTER_PATH` before `~/.gtme/adapters`, so the binding can live in version control next to the pipelines that use it. Set the variable in every shell you run gtme from.

1. Write `adapters/stacklens-companies/binding.yaml`:

    ```yaml
    # stacklens/companies: companies that use a given technology, from
    # Stacklens, an invented vendor. Served by a local stand-in for this guide.
    # Endpoint: GET /v1/companies?technology=...&page=...&per_page=...
    # Auth:     Authorization: Bearer <key>

    id: stacklens/companies
    version: 1
    role: source
    entity_type: company

    provides:
      type: object
      additionalProperties: false
      properties:
        company_domain: { type: string }
        company_name: { type: string }
        company_employees: { type: integer }
        company_industry: { type: string }
        stacklens.id: { type: string }
        stacklens.technologies: { type: array, items: { type: string } }

    config_schema:
      type: object
      additionalProperties: false
      required: [technology]
      properties:
        technology:
          type: string
          description: The technology to search for, such as hubspot
        per_page:
          type: integer
          minimum: 1
          maximum: 100
          default: 50
        tech_limit:
          type: integer
          minimum: 1
          default: 5
          description: The most technologies to keep per company
        base_url:
          type: string
          default: "https://api.stacklens.example"

    credentials: [STACKLENS_API_KEY]
    freshness_days: 30

    auth:
      type: bearer
      env: STACKLENS_API_KEY

    request:
      method: GET
      url: "{{config.base_url}}/v1/companies"
      query:
        technology: "{{config.technology}}"

    pagination:
      strategy: page
      param: page
      size_param: per_page
      page_size: "{{config.per_page}}"
      termination:
        total_pages_path: meta.total_pages

    extract:
      records: data
      fields:
        company_domain: { path: domain, transform: domain }
        company_name: name
        company_employees: { path: employees, absent: [0] }
        company_industry: industry
        stacklens.id: id
        stacklens.technologies:
          each: technologies
          template: "{{ item.name | strip }} ({{ item.category }})"
          limit: "{{ config.tech_limit }}"

    errors:
      "404": { verdict: skip, reason: no company uses this technology }

    retry:
      max_attempts: 3

    cost:
      per: record
      amount_usd: 0.02
    ```

    Here's what each block does:

    | Block | What it does here |
    |---|---|
    | `id` through `entity_type` | The manifest, the surface `gtme plan` reads. A [source](/concepts/steps-and-roles) reads nothing from earlier steps, so it declares no `needs`. |
    | `provides` | Names every field the step writes. Each is a [canonical field](/concepts/canonical-fields) for a company or starts with `stacklens.`, and `additionalProperties: false` means the step can't write a field you didn't list. |
    | `config_schema` | The settings a pipeline passes under `with:`. `technology` is required, and the rest have defaults. |
    | `credentials`, `auth` | Plan demands the key, and gtme sends it as a bearer token. |
    | `freshness_days` | How long a fetched value stays fresh. Plan prints it as `cache: 30d`. |
    | `pagination` | gtme adds `page` and `per_page`, and stops at `meta.total_pages` or at the step's `limit`, whichever comes first. |
    | `extract` | `records: data` is the list inside the response. `absent: [0]` reads a headcount of 0 as unknown. |
    | `each:` | Renders the template once per item in `technologies`, keeps at most `tech_limit`, and provides a list of strings (the decision record, [ADR-059](/decisions#adr-059)). |
    | `errors`, `retry` | A 404 ends the source with zero records and a warning. Statuses you don't list keep gtme's defaults: a 401 fails the run, and a 429 or 5xx is retried up to `retry.max_attempts` times. |
    | `cost` | Charges $0.02 per record emitted. Use `per: request` when the vendor bills per page. |

    The `each:` template is written in the same template language as a request ([ADR-057](/decisions#adr-057)). `{{ }}` inserts a value, `item` is one entry of the list, and `| strip` trims spaces. A render that comes out empty as a whole is dropped, but an empty piece inside it is kept, so an item with a blank category renders as `Segment ()`. To leave the parentheses out, test for the empty string, in single quotes because the test uses double ones:

    ```yaml
    template: '{{ item.name | strip }}{% if item.category != "" %} ({{ item.category }}){% endif %}'
    ```

    Each block is one of the binding primitives in [SPEC §10a](/spec#10a-the-binding-tier--universal-steps--decided-adr-022027). Run `gtme help --bindings` for every key each block accepts, such as the other pagination strategies and auth types ([ADR-041](/decisions#adr-041)).

1. Verify the binding before it has fixtures, to see what verify checks and where it stops:

    ```sh
    gtme adapters verify stacklens/companies
    ```

    The output is the following:

    ```
    stacklens/companies v1 — source (company)
      calls:       api.stacklens.example
      demands:     STACKLENS_API_KEY
      needs:       none
      provides:    company_domain, company_employees, company_industry, company_name, stacklens.id, stacklens.technologies
    gtme: adapters: stacklens/companies ships no conformance fixtures (fixtures/conformance.json) — fixtures are mandatory; record one real response per request (`gtme help --bindings`)
    ```

    The schema passed, and verify printed the surface you'd review in a stranger's binding: the host, the key, and the fields. Then it stopped, because nothing installs without fixtures ([ADR-042](/decisions#adr-042)). If a provided field is neither canonical nor prefixed, verify stops before printing anything:

    ```
    gtme: adapters: stacklens/companies: provides: "technologies" is not a canonical company field — use a canonical name (see spec/fields/company.json) or namespace it as <vendor>.technologies
    ```

1. Record every page as a fixture, a saved response that verify and simulate serve instead of calling the vendor:

    ```sh
    for page in 1 2 3; do
        curl -fsS -H "Authorization: Bearer $STACKLENS_API_KEY" \
            "$STACKLENS_URL/v1/companies?technology=hubspot&page=$page&per_page=2"
    done | jq -s '{config: {technology: "hubspot", per_page: 2},
        responses: [to_entries[] | {match: "?page=\(.key + 1)&", status: 200, body: .value}]}' \
        > adapters/stacklens-companies/fixtures/conformance.json
    ```

    The loop saves each page, and `jq` wraps them into one file with one entry per page, each matched on its page number.

    `config` is the step configuration verify runs with. It needs `technology` because the schema requires it, and `per_page: 2` makes verify walk all three pages. Each `match` is a piece of the request URL, and the host doesn't matter. A `match` made of query parameters compares each one exactly, so `page=2` never matches `per_page=2`, and verify can't pass with one page served twice.

1. Verify again:

    ```sh
    gtme adapters verify stacklens/companies
    ```

    The output is the following:

    ```
    stacklens/companies v1 — source (company)
      calls:       api.stacklens.example
      demands:     STACKLENS_API_KEY
      needs:       none
      provides:    company_domain, company_employees, company_industry, company_name, stacklens.id, stacklens.technologies
      fixtures:    ok — 3 response(s) on file, 5 record(s) extracted
    ```

    The stand-in holds five companies across three pages. If verify counts more than your vendor has, one `match` answered two pages.

## Run a pipeline through it

1. In the project folder, save `hubspot-users.yaml`:

    ```yaml
    name: hubspot-users
    version: 1

    source:
      use: stacklens/companies
      with:
        technology: hubspot
        limit: 4

    steps:
      - id: out
        use: csv/deliver
        with:
          path: hubspot-users.csv
          entity_type: company
        variables:
          company: company_name
          employees: company_employees
          stack: stacklens.technologies
        idempotency: company_domain
    ```

    `entity_type: company` tells `csv/deliver` it's writing companies. Without it, plan fails with `csv/deliver is a person adapter, but the records here are company`. `idempotency: company_domain` writes each company to this file once, however many times the pipeline runs.

1. Plan the pipeline:

    ```sh
    gtme plan hubspot-users.yaml
    ```

    The output is the following:

    ```
    pipeline hubspot-users (version 1)

    1. source [source] — stacklens/companies@1
         entity:    company
         provides:  company_domain, company_employees, company_industry, company_name, stacklens.id, stacklens.technologies
         cache:     30d
         creds:     STACKLENS_API_KEY (resolved)
         est/record: $0.0200
    ...
    plan ok — nothing has been spent
    ```

    Plan found the key in your shell. In a shell without it, plan stops and names the fix:

    ```
    gtme: step "source": missing credential STACKLENS_API_KEY (set it in the environment or run `gtme secret set STACKLENS_API_KEY`)
    ```

1. Run it under simulate, which serves your fixtures in place of the vendor:

    ```sh
    gtme run hubspot-users.yaml --simulate
    ```

    The output is similar to the following:

    ```
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    run 01M3MJW7WPBH8N0CZRK74DNRJY (hubspot-users)
    source [info]: stacklens/companies: 4 records
    source: sourced 4 records
    out: 4 in, 0 out, 0 cached, 0 filtered, 0 failed, 1 skipped, 3 held (dry run)

    run 01M3MJW7WPBH8N0CZRK74DNRJY — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
    step    adapter              in  out  empty  cached  filtered  failed  cost     avoided
    source  stacklens/companies  0   4    -      0       -         -       $0.0800  -
    out     csv/deliver          4   0    -      0       -         -       $0       -
    out: 1 record(s) held back by on_missing:
      umbrellahealth.com: missing company_employees
    out: resolved variables for 3 record(s) — review, then run again without --dry-run to arm:
      acme.com
        company: "Acme Corp"
        employees: "120"
        stack: "[\"HubSpot (Marketing automation)\",\"Salesforce (CRM)\",\"Segment (Analytics)\"]"
      globex.io
        company: "Globex"
        employees: "45"
        stack: "[\"HubSpot (CRM)\",\"Mixpanel (Analytics)\"]"
      initech.com
        company: "Initech"
        employees: "300"
        stack: "[\"HubSpot (Marketing automation)\",\"Zendesk (Support)\",\"Stripe (Payments)\",\"Looker (BI)\",\"Okta (Identity)\"]"
    total: $0.0800 (estimated) spent
    ```

    `limit: 4` stopped the source at 4 of the 5 companies. Umbrella Health reported 0 employees, which `absent: [0]` turned into no value.

    Every field under `variables:` needs a value before a record delivers. The default, `on_missing: skip`, skipped Umbrella Health at the `out` step, which is the `1 skipped` in the counts. Initech lists six technologies, and `tech_limit` kept five. The `$0.0800` is the binding's declared cost for 4 records, estimated; simulate called nothing. Simulate prints each variable as text, so the list shows as JSON there; the ledger keeps it a list.

1. Optional: run [armed](/concepts/gate-ladder). If your vendor's host differs from the binding's `base_url` default, set it under the source's `with:` block. The stand-in needed this line:

    ```yaml
    base_url: "http://127.0.0.1:8787"
    ```

    Then run with no flag, which is armed. Against a real vendor this run spends the declared $0.02 per record, `$0.0800` for these four. The stand-in charges nothing:

    ```sh
    gtme run hubspot-users.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3MJWM1GH52VEXRF4FRJJXDG (hubspot-users)
    source [info]: stacklens/companies: 4 records
    source: sourced 4 records
    out [info]: csv/deliver: wrote 1 row(s) to hubspot-users.csv
    out [info]: csv/deliver: wrote 1 row(s) to hubspot-users.csv
    out [info]: csv/deliver: wrote 1 row(s) to hubspot-users.csv
    out: 4 in, 3 out, 0 cached, 0 filtered, 0 failed, 1 skipped

    run 01M3MJWM1GH52VEXRF4FRJJXDG — done
    step    adapter              in  out  empty  cached  filtered  failed  cost     avoided
    source  stacklens/companies  0   4    -      0       -         -       $0.0800  -
    out     csv/deliver          4   3    -      0       -         -       $0       -
    out: 1 record(s) held back by on_missing:
      umbrellahealth.com: missing company_employees
    total: $0.0800 (estimated) spent
    ```

    The stand-in logged one request, because `per_page` defaulted to 50 and one page held all four. The receipt still shows `$0.0800`, the binding's declared cost. With a wrong key, the same run fails with `stacklens: credentials were rejected (HTTP 401)`; check the value in `STACKLENS_API_KEY`, or store it with `gtme secret set STACKLENS_API_KEY`. With `technology: nosuchtool`, the source warns `skip: no company uses this technology` and the run finishes with zero records, which is the `errors` block at work.

## What you have now

**You have a binding in your project that verifies offline, with a fixture for every page.** `gtme adapters verify stacklens/companies` is the check, and it runs without a key or the network.

If you ran armed, [the ledger](/concepts/ledger) holds the companies with the vendor's list kept as a list. Check one:

```sh
gtme show acme.com
```

The output is similar to the following:

```json
{
  "deliveries": [
    {
      "created_at": "2026-09-28T18:02:01.663Z",
      "run_id": "01M3MJWM1GH52VEXRF4FRJJXDG",
      "scope": "hubspot-users.csv",
      "status": "accepted",
      "target": "csv/deliver"
    }
  ],
  "entity_type": "company",
  "fields": {
    "company_domain": "acme.com",
    "company_employees": 120,
    "company_industry": "Computer Software",
    "company_name": "Acme Corp",
    "stacklens.id": "co_101",
    "stacklens.technologies": [
      "HubSpot (Marketing automation)",
      "Salesforce (CRM)",
      "Segment (Analytics)"
    ]
  },
  "identity_key": "acme.com",
  "identity_key_tier": "company_domain"
}
```

`identity_key` is how the ledger [recognizes the same company](/concepts/identity-keys) on every run.

Keep the binding in the project when it belongs with these pipelines, because it's versioned beside them. Copy it to `~/.gtme/adapters/stacklens-companies` when every folder on this machine should see it; verify passes there the same way.

## Hand it to Claude Code

**Paste this line into Claude Code for your own vendor:**

```text
Write a gtme binding for VENDOR's ENDPOINT from DOCS_URL in ./adapters, with its key in KEY_VAR, record a fixture for every page, and stop when gtme adapters verify passes.
```

Replace the following:

- `VENDOR`: the vendor's name
- `ENDPOINT`: the call you want, such as its company search
- `DOCS_URL`: the page that documents it
- `KEY_VAR`: the environment variable that holds the key

The agent writes the folder, records the fixtures with your key, and stops at a passing verify for you to read.

## Next

- [Types and traverse](/concepts/types-and-traverse) shows the binding shape that follows a relation, such as a company to its people.
- [A campaign is a folder](/concepts/campaign-is-a-folder) shows how a bundle carries this binding with its fixtures.
- [`gtme adapters`](/reference/cli/adapters) lists the verbs that search, add, verify, and update bindings.
