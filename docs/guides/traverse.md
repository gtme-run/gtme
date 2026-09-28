---
name: Traverse a relation
description: Write a traverse step that turns a run's people into their companies, plan it, run it keyless, and check the relations and identities the ledger holds
for: "You have people and want their companies, or companies and want the people at them, and you're about to write the step that crosses from one type to the other."
learn:
  - "how to pick the relation and which end of it is the parent"
  - "how to read a traverse in the plan, and which fields a step after it can read"
  - "what the traverse line on the receipt counts"
  - "where to check the relation rows and the identities in the ledger"
order: 12
roles: [builder]
links:
  - to: /concepts/pipeline
    type: relates-to
    description: What accounts.yaml and people-at.yaml are made of, a source and a list of steps
  - to: /concepts/ledger
    type: relates-to
    description: The relations the traverse follows and the identities it reaches are rows in the one ledger file
  - to: /reference/cli/show
    type: relates-to
    description: Prints the company the traverse reached, with its fields
  - to: /concepts/types-and-traverse
    type: depends-on
    description: What a type, a relation, a leg, a traverse, minting, and coalescing are; this guide is the procedure that uses them
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan is the rung that checks every leg before anything runs
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: What the receipt columns count, including empty
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The filter and compose roles the company leg uses
  - to: /concepts/adapter-tiers
    type: relates-to
    description: A vendor traverse is a binding shaped like a source, with a from type and a relation
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: The shipped posts-to-engagers bundle is the vendor version of this guide
  - to: /concepts/groups
    type: relates-to
    description: The group a traverse run ends in takes the last leg's type
  - to: /reference/ledger-schema/relations
    type: relates-to
    description: The table the relation rows in this guide come from
  - to: /reference/ledger-schema/step_events
    type: relates-to
    description: Where the traversed and coalesced events for each child are logged
  - to: /reference/fields/company
    type: relates-to
    description: The fields a step on the company leg can read
  - to: /reference/cli/groups
    type: relates-to
    description: Every verb for the typed groups the two runs end in
  - to: /reference/cli/query
    type: relates-to
    description: Read-only SQL against the ledger, used to check the relations
  - to: /spec#7-contract-validation--the-planner--decided
    type: decided-by
    description: How plan walks typed legs and refuses a field or a when from the wrong leg
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: What each count on the traverse receipt line means
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: sql/traverse, its two result columns, and why it mints nothing
  - to: /decisions#adr-054
    type: decided-by
    description: The traverse role, typed legs, and relations as facts
  - to: /decisions#adr-058
    type: decided-by
    description: The words leg and already in this run
---

# Traverse a relation

**Goal: a [pipeline](/concepts/pipeline) that turns a CSV of people into their companies.** The companies you have two contacts at end in a [group](/concepts/groups). Then you cross the same relation back, from a company to the people who work there.

## Before you start

**You need `gtme` ([Install](/start/install)), and [Types and traverse](/concepts/types-and-traverse) explains the words this guide uses.** That page defines a type, a relation, a leg, and what it means to mint or coalesce.

**This guide needs no API keys, spends nothing, and sends nothing.** Its traverse is `sql/traverse`, which follows a relation the [ledger](/concepts/ledger) already holds. The relation comes from your CSV: a person row with a `company_domain` makes the source write `works_at` to that company, and mint the company if it's new. `sql/traverse` mints nothing.

## Steps

Build the pipeline, then check what it left behind:

1. Make a folder with its own ledger, and save four people as `people.csv`:

    ```sh
    mkdir -p traverse && cd traverse
    export GTME_LEDGER=./ledger.db
    cat > people.csv <<'EOF'
    Full Name,Email,Title,Company,Company Website
    Jane Doe,jane.doe@acme.com,VP Marketing,Acme Inc,acme.com
    Sam Lee,sam@acme.com,Head of Growth,Acme Inc,acme.com
    Bob Stone,bob@globex.io,RevOps Lead,Globex,globex.io
    Priya Rao,priya@gmail.com,Founder,,
    EOF
    ```

    Jane and Sam both work at Acme. Priya has no company.

1. Save `accounts.yaml`:

    ```yaml
    name: accounts
    version: 1

    source:
      use: csv/source
      with:
        path: people.csv
        columns: { full_name: Full Name, email: Email, title: Title, company_name: Company, company_domain: Company Website }

    steps:
      - id: to-company
        use: sql/traverse
        with:
          entity_type: company
          query: >
            SELECT r.to_id AS identity_id, r.from_id AS parent_id
            FROM relations r WHERE r.relation = 'works_at'

      - id: two-contacts
        use: sql/filter
        with:
          query: >
            SELECT r.to_id AS identity_id FROM relations r
            WHERE r.relation = 'works_at'
            GROUP BY r.to_id HAVING COUNT(*) >= 2

      - id: account-line
        use: text/compose
        uses: [company_name, company_domain]
        provides: [account_line]
        with:
          template: "{{ record.company_name }} ({{ record.company_domain }})"

    group: target-accounts
    ```

    `to-company` is the traverse. Here the person is the parent and the company is the child. `entity_type: company` is the type it emits, and its query returns the child as `identity_id` and the parent, the record in the run it came from, as `parent_id`.

    A relation runs one way. `works_at` goes from a person (`from_id`) to a company (`to_id`). Companies are matched by domain, so two people at `acme.com` reach one company, and a person with no domain gets no `works_at` row. `two-contacts` is a [filter](/concepts/steps-and-roles) that counts people per company across the whole ledger and keeps companies with 2 or more; to keep every company, delete it. `account-line` is a compose step that fills a template and calls no model.

1. [Plan](/concepts/gate-ladder) it:

    ```sh
    gtme plan accounts.yaml
    ```

    The output is the following, trimmed:

    ```
    pipeline accounts (version 1)

    1. source [source] — csv/source@1
         entity:    person
         writes:    works_at → company (from company_domain)
    ...
    2. to-company [traverse] — sql/traverse
         traverse:  person → company via sql/traverse (follows an existing relation; mints nothing)
    ...
    3. two-contacts [filter] — sql/filter
         entity:    company
    ...
    4. account-line [compose] — text/compose@1
         entity:    company
         reads:     company_name, company_domain
         requires:  company_name, company_domain
         provides:  accounts.account_line
         est/record: ?

    ends in group "target-accounts" as company (records that complete the run are added)

    available fields after the last step: accounts.account_line
    note: a step provides open-ended fields, so per-record needs are re-checked at run time
    plan ok — nothing has been spent
    ```

    Read the `entity:` lines top to bottom. The source works on people, and every step after `to-company` works on companies, so the run has two legs. `writes:` is where the relation comes from. The group takes the last leg's type.

    After a traverse, a step reads the new type's fields. `account-line` reads `company_name` and `company_domain`, which the source wrote to each company when it minted it. The available-fields line lists only what this pipeline's steps provide. The note means plan checked the field names against the company type, and the run checks each company has the values.

1. Add `title` to `account-line`, and plan again:

    ```yaml
        uses: [company_name, company_domain, title]
    ```

    ```sh
    gtme plan accounts.yaml
    ```

    It prints:

    ```
    gtme: step "account-line": uses: "title" is not a canonical company field — use a canonical name (see spec/fields/company.json) or namespace it as <vendor>.title
    ```

    `title` belongs to a person, and after `to-company` the records are companies, so plan stops before anything runs. The company fields page, linked at the end, lists what the company leg can read. A step that reads person fields goes before the traverse. Take `title` back out.

1. Gate on a person filter, and plan again. Add a `senior` filter before `to-company`, and put `when: senior.passed` on `account-line`:

    ```yaml
      - id: senior
        use: sql/filter
        with:
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'title' AND (value LIKE 'VP%' OR value LIKE 'Head of%')
    ```

    ```sh
    gtme plan accounts.yaml
    ```

    It prints:

    ```
    gtme: step "account-line": when: senior.passed names a step before the traverse "to-company" — a verdict is a fact about the parent, not the child; gate at the traverse instead (when: senior.passed on "to-company" mints only children of passing parents)
    ```

    Move `when: senior.passed` to `to-company`, as the message says, or take both edits back out. The rest of this guide uses the file without them.

1. Run it:

    ```sh
    gtme run accounts.yaml
    ```

    The [receipt](/concepts/runs-and-receipts) is similar to the following:

    ```
    ...
    run 01M3ME25E2Z1JG6XXYZ0Y72ZEN — done
    step          adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source        csv/source    0   4    -      0       -         -       $0    -
    to-company    sql/traverse  4   3    1      0       -         -       $0    -
    two-contacts  sql/filter    2   1    -      0       1         -       $0    -
    account-line  text/compose  1   1    -      0       -         -       $0    -
    group "target-accounts": 1 record(s) added
    to-company: 4 parent(s) in, 3 out, 1 empty — 2 traversed (company), 1 already in this run
    total: $0 spent
    ```

    `to-company` took 4 people in, and 3 of them led to a company. Priya is the 1 `empty`: she has no `works_at` row. `2 traversed` counts the children, Acme and Globex.

    Sam's Acme reached a company Jane had already brought into the run, so it coalesced into that record and counts as `already in this run`. So 3 rows came out, and 2 companies move forward, which is why `two-contacts` shows 2 in. It kept Acme, and Acme is the group's one member.

1. Check the relation rows with [`gtme query`](/reference/cli/query):

    ```sh
    gtme query "SELECT p.identity_key AS person, r.relation,
                c.identity_key AS company FROM relations r
                JOIN identities p ON p.id = r.from_id
                JOIN identities c ON c.id = r.to_id"
    ```

    It prints:

    ```
    {"company":"acme.com","person":"jane.doe@acme.com","relation":"works_at"}
    {"company":"acme.com","person":"sam@acme.com","relation":"works_at"}
    {"company":"globex.io","person":"bob@globex.io","relation":"works_at"}
    3 rows
    ```

    The source wrote these rows, and `to-company` followed them without writing any. Priya has none, which is why she came out `empty`.

1. Check what the traverse reached:

    ```sh
    gtme query "SELECT e.event, i.identity_key FROM step_events e
                JOIN identities i ON i.id = e.identity_id
                WHERE e.step_id = 'to-company' AND i.entity_type = 'company'"
    ```

    It prints:

    ```
    {"event":"traversed","identity_key":"acme.com"}
    {"event":"coalesced","identity_key":"acme.com"}
    {"event":"traversed","identity_key":"globex.io"}
    3 rows
    ```

    Acme is one identity with two events. The ledger's word for `already in this run` is `coalesced`.

1. Go the other way. Save `companies.csv` and `people-at.yaml`:

    ```sh
    printf 'Company,Company Website\nAcme Inc,acme.com\n' > companies.csv
    ```

    ```yaml
    name: people-at
    version: 1

    source:
      use: csv/source
      with:
        path: companies.csv
        entity_type: company
        columns: { company_name: Company, company_domain: Company Website }

    steps:
      - id: to-people
        use: sql/traverse
        with:
          entity_type: person
          query: >
            SELECT r.from_id AS identity_id, r.to_id AS parent_id
            FROM relations r WHERE r.relation = 'works_at'

    group: acme-people
    ```

    The relation is the same, and the columns swap: the company is now the parent, so `to_id` is `parent_id`.

1. Run it:

    ```sh
    gtme run people-at.yaml
    ```

    The output includes:

    ```
    to-people: 1 result row(s) ignored (parent outside the run)
    ...
    group "acme-people": 2 record(s) added
    to-people: 1 parent(s) in, 1 out, 0 empty — 2 traversed (person), 0 already in this run
    ```

    The query read every `works_at` row in the ledger, and gtme kept the ones whose parent is in this run. Bob's row points at Globex, which isn't, so it's ignored.

## What you have now

**You have two pipelines that cross the same relation in opposite directions, and a typed group from each.**

```sh
gtme groups
```

```
group            type     members  added  removed  touched  created
acme-people      person   2        2      0        0        2026-09-28
target-accounts  company  1        1      0        0        2026-09-28
```

`target-accounts` holds companies and `acme-people` holds people, because each group took its run's last leg. Run `gtme show acme.com` ([`gtme show`](/reference/cli/show)) and it prints the company with its two CSV fields and `accounts.account_line`.

**To get the companies out as a file, add a `csv/deliver` step with `entity_type: company` under its `with:`, or query the group.** Without that key, `csv/deliver` expects people and plan refuses it on a company leg. The query needs no change to the pipeline:

```sh
gtme query --format csv \
  "SELECT i.identity_key AS domain, v.value AS account_line
            FROM group_members m
            JOIN groups g ON g.id = m.group_id
            JOIN identities i ON i.id = m.identity_id
            JOIN current_values v ON v.identity_id = m.identity_id
              AND v.field = 'accounts.account_line'
            WHERE g.name = 'target-accounts'" > target-accounts.csv
```

`target-accounts.csv` then holds one row per company:

```
domain,account_line
acme.com,Acme Inc (acme.com)
```

**To make it yours, start from the relations your ledger holds:**

```sh
gtme query --format table \
  "SELECT relation, count(*) AS n FROM relations GROUP BY relation"
```

```
relation  n
works_at  3
1 rows
```

Change the relation name and the `entity_type`, and put each column where the relation's direction says it goes. If the relation you need isn't in the ledger yet, a vendor traverse mints it: the shipped `posts-to-engagers` [bundle](/concepts/campaign-is-a-folder) has two, `harvest/profile-posts` and `harvest/post-reactions`. Both are [bindings](/concepts/adapter-tiers) that need `HARVEST_API_KEY` and spend per call, and `gtme run . --simulate` runs them offline from recorded responses. To run this with your agent, paste this into Claude Code:

```text
Map the CSV at PATH with company_domain from its website column, write a pipeline that crosses works_at from each person to their company with sql/traverse, and end it in a group. Run gtme plan and read me each traverse line and the entity of every step after it. If plan is ok, run it, explain which people were empty and which companies were already in this run, and export the group as one CSV row per company.
```

Replace `PATH` with your CSV file.

## Next

- [Company fields](/reference/fields/company) lists every field a step on the company leg can read.
- [The `relations` table](/reference/ledger-schema/relations) shows the columns your traverse query selects from.
- [`gtme groups`](/reference/cli/groups) covers every verb for the typed group a run ends in.
