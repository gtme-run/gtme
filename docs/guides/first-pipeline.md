---
name: Build your first pipeline from a CSV
description: Write a pipeline file from an empty folder one step at a time, check each edit with plan, then dry-run and run it with no keys for $0
for: "You have a CSV of people and want to write your own pipeline file from nothing, checking each piece with plan before you add the next."
learn:
  - "write a pipeline file one step at a time and check each edit with plan"
  - "map a CSV header to a canonical field, filter in SQL, and compose a line from a template"
  - "review a dry-run's resolved variables, then run armed"
  - "confirm that a second run delivers nothing twice"
order: 1
roles: [builder]
links:
  - to: /concepts/pipeline
    type: depends-on
    description: What each key in the file does; this guide writes those keys one at a time
  - to: /concepts/steps-and-roles
    type: depends-on
    description: The filter, compose, and deliver roles of the three steps this guide adds
  - to: /concepts/gate-ladder
    type: depends-on
    description: Plan checks every edit, and the dry-run is the review before the armed run
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH, which every step here runs
  - to: /start/show-me
    type: relates-to
    description: Uses the same sample contacts.csv every output on this page comes from
  - to: /start/my-csv
    type: relates-to
    description: The same kind of CSV run through a finished file with AI steps and an Anthropic key
  - to: /concepts/canonical-fields
    type: relates-to
    description: What a CSV header becomes, and why company_domain is the field to map a website to
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: What each column of the receipt counts, and how idempotency skips a second delivery
  - to: /concepts/ledger
    type: relates-to
    description: Where the verdicts, lines, and deliveries from these runs are kept
  - to: /start/my-stack
    type: relates-to
    description: The next file adds vendor keys and a delivery target that sends
  - to: /concepts/identity-keys
    type: relates-to
    description: Why out.csv's first column is an email for most rows and a name hash for a row with no email
  - to: /reference/adapters/text-compose
    type: relates-to
    description: The template dialect the opener step uses, and what it provides
  - to: /reference/cli/plan
    type: relates-to
    description: Every line plan prints, which this guide reads after each edit
  - to: /spec#9-pipelineyaml--decided
    type: decided-by
    description: The grammar of the file this guide writes
  - to: /decisions#adr-018
    type: decided-by
    description: Why columns on the source and variables on the deliver step are the only mappings
  - to: /decisions#adr-027
    type: decided-by
    description: Why a SQL filter is an ordinary step that costs nothing and runs every time
  - to: /decisions#adr-057
    type: decided-by
    description: The template key and text/compose, which renders a field with no model
---

# Build your first pipeline from a CSV

**Goal: `contacts.yaml`, a pipeline file you wrote yourself.** It keeps the VPs and heads of a function in your CSV and writes each one an opening line.

## Before you start

**You need `gtme` and a CSV of people with a header row.** [Install](/start/install) covers the first. [A pipeline is a YAML file](/concepts/pipeline) explains each key you'll write, and [Steps and roles](/concepts/steps-and-roles) explains the three steps. Without a CSV of your own, fetch the sample every output on this page comes from:

```sh
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
```

With your own CSV, your headers and counts will differ.

**This guide needs no keys, spends $0 on every rung, and sends nothing past your own disk.** The one deliver step appends rows to `out.csv` in your folder.

## Write the file

1. Make a folder with its own [ledger](/concepts/ledger), and copy in the first 10 rows of your CSV:

    ```sh
    mkdir contacts
    head -n 11 CSV_FILE > contacts/contacts.csv
    cd contacts
    export GTME_LEDGER=./ledger.db
    ```

    Replace `CSV_FILE` with the path to your CSV, or with `contacts.csv` for the sample. `GTME_LEDGER` keeps these runs out of your main ledger. Set it again in any new terminal.

1. Create `contacts.yaml` with a name and a source, and plan it:

    ```yaml
    name: contacts
    version: 1

    source:
      use: csv/source
      with:
        path: contacts.csv
    ```

    ```sh
    gtme plan contacts.yaml
    ```

    Plan prints:

    ```
    pipeline contacts (version 1)

    1. source [source] — csv/source@1
         entity:    person
         provides:  company_website, email, full_name, title
         est/record: ?

    available fields after the last step: company_website, email, full_name, title
    plan ok — nothing has been spent
    ```

    That's already a valid file, and `provides:` lists what each header became.

    gtme lowercases a header and turns its spaces into underscores. If the result is a [canonical field](/concepts/canonical-fields), like `Full Name` becoming `full_name`, it maps itself. Any other header lands as `csv.` plus its name, so a `Job Title` column shows up as `csv.job_title`. Look for a `csv.` field you need; the next step maps it.

    We name a pipeline after what it holds. The name prefixes every field the file declares for itself, so pick one you'll recognize in the ledger.

1. Map the header that holds each person's company website or domain to `company_domain`. Under the source's `with:`, add `columns:` and plan again:

    ```yaml
        columns:
          company_domain: Company Website
    ```

    Plan's entry for the source now reads:

    ```
    ...
    1. source [source] — csv/source@1
         entity:    person
         writes:    works_at → company (from company_domain)
         provides:  company_domain, email, full_name, title
         est/record: ?
    ...
    ```

    `company_website` is a canonical field too, but gtme matches companies on `company_domain`, and it stores `https://www.acme.com` as `acme.com`. The new `writes:` line means the source also records which company each person works at. `columns:` puts gtme's name first and your header second (the decision record, [ADR-018](/decisions#adr-018)). Map any other header the same way; with a `Job Title` column, add `title: Job Title`.

1. Add a `steps:` list with a filter that keeps VPs and heads of a function, and plan again:

    ```yaml

    steps:
      - id: leaders
        use: sql/filter
        with:
          uses: [title]
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head of%')
    ```

    Plan adds:

    ```
    ...
    2. leaders [filter] — sql/filter
         entity:    person
         reads:     title
         requires:  title
         provides:  (none)
         est/record: ?
    ...
    ```

    `reads: title` is the field the query uses. Plan doesn't check that the source provides it, so compare it with the source's `provides:` yourself. On a SQL step, `uses:` goes inside `with:`, unlike the next step.

    The query asks the ledger's `current_values` view, which holds one row per person per field, for the people to keep. `sql/filter` runs no model and costs nothing. A person the query doesn't return fails, and the ledger keeps that verdict and its reason ([ADR-027](/decisions#adr-027)). `LIKE` ignores case, so `VP%` matches `vp sales`, but it misses `SVP Growth`. Write patterns for the titles in your file, such as `value LIKE '%VP%'`.

    Plan does stop on a field name gtme doesn't know. With `uses: [titel]`, plan prints the following and exits with status 2:

    ```
    gtme: step "leaders": "titel" is not a canonical person field (did you mean "title"?) — use a canonical name or namespace it as <vendor>.titel
    ```

1. Append a compose step that writes an opening line from a template, and plan again:

    ```yaml

      - id: opener
        use: text/compose
        uses: [full_name, company_domain]
        provides: [opener]
        with:
          template: "{{ record.full_name }}, a quick question about growth at {{ record.company_domain }}"
    ```

    Plan adds:

    ```
    ...
    3. opener [compose] — text/compose@1
         entity:    person
         reads:     full_name, company_domain
         requires:  full_name, company_domain
         provides:  contacts.opener
         est/record: ?
    ...
    ```

    `provides: [opener]` lands as `contacts.opener`, prefixed with the file's name. [`text/compose`](/reference/adapters/text-compose) renders the template once per record with no model, and `record.*` can name only the fields in `uses:` ([ADR-057](/decisions#adr-057)). A record that `leaders` failed never reaches this step.

1. Append a deliver step that writes each kept person to `out.csv`, and plan again:

    ```yaml

      - id: out
        use: csv/deliver
        with:
          path: out.csv
        variables:
          full_name: full_name
          opener: contacts.opener
        idempotency: email
    ```

    Plan adds the step and the send surface:

    ```
    ...
    4. out [deliver] — csv/deliver@1
         record:    touched → contacts
         entity:    person
         reads:     contacts.opener, full_name
         requires:  contacts.opener, full_name
         provides:  (none)
         idempotency: email
         variables: full_name ← full_name, opener ← contacts.opener
    ...
    send surface: 1 deliver step(s)
      out → csv/deliver (touch scope: contacts)

    available fields after the last step: company_domain, contacts.opener, email, full_name, title
    plan ok — nothing has been spent
    ```

    `variables:` maps each column of `out.csv` to a ledger field. `idempotency: email` keys each delivery on the email address, so a later run skips anyone already written to this file. `send surface` lists every step that sends, and here it's the one that writes a file.

1. Dry-run it. This spends $0 and sends nothing:

    ```sh
    gtme run contacts.yaml --dry-run
    ```

    It prints output similar to the following:

    ```
    dry run: deliver steps will resolve and receipt their variables, but nothing sends
    ...
    run 01M3K0CMAM4J6WCN7QFKD144DC — done (dry run — nothing sent)
    step     adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source   csv/source    0   3    -      0       -         -       $0    -
    leaders  sql/filter    3   2    -      0       1         -       $0    -
    opener   text/compose  2   2    -      0       -         -       $0    -
    out      csv/deliver   2   0    -      0       -         -       $0    -
    out: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        full_name: "Jane Doe"
        opener: "Jane Doe, a quick question about growth at acme.com"
      bob@globex.io
        full_name: "Bob Stone"
        opener: "Bob Stone, a quick question about growth at globex.io"
    total: $0 spent
    ```

    A [dry-run](/concepts/gate-ladder) runs every step and holds `out`. The resolved variables are exactly what `out` would write, so read them now. `leaders` filtered one record, Carol, whose title is Marketing Operations Manager. The gate ladder has a lower rung, simulate, which fakes vendor and model calls. This file makes none, so we skip it.

1. Run it armed, the run with no flag. This spends $0 and sends the two kept rows to `out.csv`:

    ```sh
    gtme run contacts.yaml
    ```

    It prints output similar to the following:

    ```
    ...
    run 01M3K0CMBDTCFVRS73JGF1H644 — done
    step     adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source   csv/source    0   3    -      0       -         -       $0    -
    leaders  sql/filter    3   2    -      0       1         -       $0    -
    opener   text/compose  2   0    -      2       -         -       $0    ?
    out      csv/deliver   2   2    -      0       -         -       $0    -
    total: $0 spent, $0.0000+? avoided via cache (2 records skipped)
    ```

    The `opener` row shows `cached 2` because the dry-run already composed both lines and the ledger kept them. The `?` means a template has no price to count as saved. Open the file:

    ```sh
    cat out.csv
    ```

    ```
    identity_key,full_name,opener
    jane.doe@acme.com,Jane Doe,"Jane Doe, a quick question about growth at acme.com"
    bob@globex.io,Bob Stone,"Bob Stone, a quick question about growth at globex.io"
    ```

    `identity_key` is the key gtme files each person under: the email, or a hash of the name when the email is blank ([Identity keys](/concepts/identity-keys)).

1. Run it again:

    ```sh
    gtme run contacts.yaml
    ```

    It prints output similar to the following:

    ```
    ...
    run 01M3K0CMC9B1RGP1N8EMVRXM51 — done
    step     adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source   csv/source    0   3    -      0       -         -       $0    -
    leaders  sql/filter    3   2    -      0       1         -       $0    -
    opener   text/compose  2   0    -      2       -         -       $0    ?
    out      csv/deliver   2   0    -      2       -         -       $0    $0.0000
    total: $0 spent, $0.0000+? avoided via cache (4 records skipped)
    ```

    The `out` row shows `cached 2` and `out 0`: [idempotency](/concepts/runs-and-receipts) found both emails already delivered to this file, and `out.csv` still has two rows. `leaders` judged all three again, because a SQL step recomputes on every run.

## What you have now

**`contacts.yaml` is a four-step pipeline you wrote and can read line by line:**

```yaml
name: contacts
version: 1

source:
  use: csv/source
  with:
    path: contacts.csv
    columns:
      company_domain: Company Website

steps:
  - id: leaders
    use: sql/filter
    with:
      uses: [title]
      query: >
        SELECT identity_id FROM current_values
        WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head of%')

  - id: opener
    use: text/compose
    uses: [full_name, company_domain]
    provides: [opener]
    with:
      template: "{{ record.full_name }}, a quick question about growth at {{ record.company_domain }}"

  - id: out
    use: csv/deliver
    with:
      path: out.csv
    variables:
      full_name: full_name
      opener: contacts.opener
    idempotency: email
```

To check what happened, list the runs this folder's ledger holds. You should see one dry-run and two armed runs:

```sh
gtme runs
```

```
run                         pipeline  status      started                   records  in flight
01M3K0CMC9B1RGP1N8EMVRXM51  contacts  done        2026-09-28T03:19:28.905Z  3        -
01M3K0CMBDTCFVRS73JGF1H644  contacts  done        2026-09-28T03:19:28.877Z  3        -
01M3K0CMAM4J6WCN7QFKD144DC  contacts  done (dry)  2026-09-28T03:19:28.852Z  3        -
```

When the 10 rows read right, point `path:` at the full CSV and run again; it still costs $0, and anyone already in `out.csv` isn't written twice. The file is also what you hand your agent next. To judge titles with a model instead of `LIKE`, ask it to swap `leaders` for an `ai/filter` step. That needs an Anthropic key and spends a few cents per record, as Your CSV shows.

## Next

- [Your stack](/start/my-stack) adds vendor keys and a delivery target that sends.
- [Your CSV](/start/my-csv) runs a file with an `ai/filter` and an `ai/compose` step.
- [`gtme plan`](/reference/cli/plan) lists every form of the check you ran after each edit.
