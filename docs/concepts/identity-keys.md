---
name: Identity keys
description: An identity key is the normalized email, LinkedIn URL, domain, or name hash that makes one person or company the same record across runs and vendors
for: "You've seen one person turn up as two records, or you're about to import a CSV and want to know what makes a row the same record"
learn:
  - "the key tiers and the order they're tried in"
  - "how an email, a domain, and a LinkedIn URL are normalized before they become a key"
  - "what happens when a weak key later gains a stronger one"
order: 6
roles: [builder]
links:
  - to: /concepts/ledger
    type: relates-to
    description: Identity keys are the primary key of the identities table the ledger is built on
  - to: /concepts/canonical-fields
    type: relates-to
    description: The registry defines the normalization rules that key derivation and every source share
  - to: /concepts/types-and-traverse
    type: relates-to
    description: Each entity type's file lists its key tiers, and the works_at relation a source writes is what traverse follows
  - to: /start/my-csv
    type: relates-to
    description: Maps your own CSV's columns, which is where a key column gets named
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Adapters map a vendor's fields to canonical names and never derive a key themselves
  - to: /guides/traverse
    type: relates-to
    description: Follows the works_at relation a source writes from people to their companies
  - to: /concepts/facts
    type: relates-to
    description: Decides which value wins when two rows for one record disagree about a field
  - to: /reference/ledger-schema
    type: relates-to
    description: The DDL for identities and field_values
  - to: /reference/cli/show
    type: relates-to
    description: Prints one record by any key it has held
  - to: /reference/cli/query
    type: relates-to
    description: Read-only SQL against the identities and relations tables
  - to: /spec#4-identity-keys--decided
    type: decided-by
    description: The normative tier lists, normalization rules, and the upgrade-in-place rule
  - to: /decisions#adr-020
    type: decided-by
    description: Why only the public LinkedIn URL is key material, and the order of the reserved handle tiers
  - to: /decisions#adr-017
    type: decided-by
    description: Why normalization rules live once, in the canonical field registry
---

# Identity keys

Here's a CSV written the way lists arrive: a mixed-case email, a LinkedIn URL with a tracking parameter, and one person with neither.

```
Full Name,Email,LinkedIn,Company Website
Jane Doe,Jane.Doe@Acme.com,https://www.linkedin.com/in/jane-doe/,https://www.acme.com/about
Raj Patel,,https://www.linkedin.com/in/Raj-Patel?trk=public,www.globex.co.uk
Mia Chen,,,initech.dev
```

Save it as `people.csv`, with this pipeline beside it as `people.yaml`. It has no steps, because keys are made when the source writes a row. It spends nothing and sends nothing:

```yaml
name: people
version: 1

source:
  use: csv/source
  with:
    path: people.csv
    columns: { full_name: Full Name, email: Email, linkedin_url: LinkedIn, company_domain: Company Website }

steps: []
```

Run it against a scratch ledger, then list the records it made:

```sh
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run people.yaml
gtme query "SELECT entity_type, identity_key FROM identities"
```

The output is the following:

```
{"entity_type":"company","identity_key":"acme.com"}
{"entity_type":"company","identity_key":"globex.co.uk"}
{"entity_type":"company","identity_key":"initech.dev"}
{"entity_type":"person","identity_key":"in/raj-patel"}
{"entity_type":"person","identity_key":"jane.doe@acme.com"}
{"entity_type":"person","identity_key":"nh:89bce7747a81c7c91c0c518e75f05d78aacde6d9366fdc77d00f103f226ddefc"}
6 rows
```

Three rows made six records, three people and their three companies, each named by the `identity_key` the runner derived.

## What you just saw

**An identity key is the one string that says which record a row is about.** Every fact in the [ledger](/concepts/ledger) hangs off an identity, and the `identities` table holds one row per type and key.

**A person is keyed by the first of these the record has:**

| Tier | From | Key looks like |
|---|---|---|
| 1 | `email` | `jane.doe@acme.com` |
| 2 | `linkedin_url`, public form only | `in/raj-patel` |
| 3–4 | `github_username`, `twitter_handle` | reserved; no adapter provides them yet |
| 5 | `full_name` and `company_domain`, hashed | `nh:89bce774...` |

Jane had both, and email comes first. Raj had no email, so his LinkedIn URL is his key.

Mia got a name hash, the weakest tier. A hash is a fixed-length fingerprint of text, the same every time for the same text. Hers is the SHA-256 hash of `mia chen|initech.dev`, prefixed `nh:`. The name is the one part it requires, so a name with no company domain is hashed alone. A row with no name and no stronger key is dropped, and the run's output says why.

**A company is keyed by its domain, reduced to eTLD+1.** That's the part of a domain you'd buy from a registrar. `https://www.acme.com/about` became `acme.com`, and `www.globex.co.uk` became `globex.co.uk`. A company with no domain gets an `nh:` hash of its name.

**Every key is normalized before it's compared.** An email is trimmed and lowercased. A LinkedIn URL loses its protocol, host, trailing slash, and query string, then is lowercased, so Raj keys as `in/raj-patel`. Two rows with the same email are one record, a shared inbox like `info@acme.com` included.

**Only the public LinkedIn URL is a key.** LinkedIn also hands out internal URLs with an opaque token in the path, and a source drops that shape with a warning:

```
source [warn]: row 3: dropped field "linkedin_url": "https://www.linkedin.com/in/ACwAAAbQ2xKB9xyz" is not a valid value (rule linkedin_url)
```

That person falls to a weaker tier until an enrichment writes the public URL.

The source also writes a `works_at` relation from each person to their company, which [traverse](/concepts/types-and-traverse) follows.

## When a later run brings a different key

**A matching row lands on its record, and a stronger key replaces a weaker one in place.** Here's `more.csv`. Mia now has an email, and Jane arrives with only a differently cased LinkedIn URL:

```
Full Name,Email,LinkedIn,Company Website
Mia Chen,Mia@Initech.dev,,initech.dev
Jane Doe,,https://linkedin.com/in/Jane-Doe,acme.com
```

Change `path:` in `people.yaml` to `more.csv` and run it again in the same ledger. The `identities` table still has 6 rows. The runner logged Mia's change in `step_events`, the ledger's log of what each step did to each record:

```sh
gtme query "SELECT step_id, detail FROM step_events
            WHERE event = 'identity_upgraded'"
```

```
{"detail":"{\"from\":\"nh:89bce7747a81c7c91c0c518e75f05d78aacde6d9366fdc77d00f103f226ddefc\",\"to\":\"mia@initech.dev\"}","step_id":"source"}
1 rows
```

Mia's row matched her name hash and carried an email, so her record kept its id and facts and took the email as its key. `identity_key_tier` names the tier the key came from, so a record still on a bare `nh:` hash says so where the key prints. The old key still finds her:

```sh
gtme show nh:89bce7747a81c7c91c0c518e75f05d78aacde6d9366fdc77d00f103f226ddefc
```

```json
{
  "entity_type": "person",
  "fields": {
    "company_domain": "initech.dev",
    "email": "mia@initech.dev",
    "full_name": "Mia Chen"
  },
  "identity_key": "mia@initech.dev",
  "identity_key_tier": "email"
}
```

Jane shows the other half: a record answers to every key it has ever held. The first run derived `in/jane-doe` from her LinkedIn column and stored it beside her email key. So her LinkedIn-only row landed on her record, and her email key stayed:

```sh
gtme show in/jane-doe --fields email,full_name
```

```json
{
  "entity_type": "person",
  "fields": {
    "email": "jane.doe@acme.com",
    "full_name": "Jane Doe"
  },
  "identity_key": "jane.doe@acme.com",
  "identity_key_tier": "email"
}
```

The upgrade holds for any step whose row carries a known key and a stronger one, an enrich step included. Which of two values wins for a field is [Facts have provenance](/concepts/facts).

So what? Every source names Jane the same way, so the ledger's cache works across vendors. When your agent writes a source, map every email and public LinkedIn column the file has in `columns:`. A row carrying both is what ties them together.

That's it. That's how a row becomes a record.

## Why it's this way

**Keys come from what a platform shows publicly, never from a vendor's record id.** A vendor id splits a person in two the moment a second vendor arrives, and an email or a domain reads the same whoever reports it ([SPEC §4](/spec#4-identity-keys--decided)).

Internal LinkedIn URLs aren't keys because one person under both shapes would get two keys no upgrade could join (the decision record, [ADR-020](/decisions#adr-020)). The normalization rules sit in the [canonical field](/concepts/canonical-fields) registry, so the key and the stored value are made by the same rule ([ADR-017](/decisions#adr-017)).

**What it costs.** gtme never merges two records. Had Mia's second row carried only her email, nothing would tie it to the hash, and she'd be two people. Two Mia Chens at one company share a hash and become one. That's why the hash is the last tier.

This query lists every record still keyed by a name hash, and it's the block you'd hand your agent after every import:

```sh
gtme query "SELECT identity_key FROM identities WHERE identity_key LIKE 'nh:%'"
```

After the second run, none are left:

```
0 rows
```

## Where it shows up

- [Your CSV](/start/my-csv) is where you map the columns a key comes from.
- [Traverse a relation](/guides/traverse) follows `works_at` from people to companies.
- [`gtme show`](/reference/cli/show), [`gtme query`](/reference/cli/query), and [the schema](/reference/ledger-schema) are the lookup pages.
