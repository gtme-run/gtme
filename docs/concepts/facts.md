---
name: Facts have provenance
description: One append-only row recording who wrote a value, how sure it was, and when. Ranking those rows picks the value a step reads
for: "Two runs or two vendors gave one person different values, and you want to know which one a step will read and why."
learn:
  - "what columns a fact carries and who sets confidence"
  - "which row wins when facts about one field conflict"
  - "how a cache window makes a winning value stale"
  - "why a raw vendor response isn't a fact, and what gtme vacuum deletes"
order: 8
roles: [builder, operator]
links:
  - to: /concepts/ledger
    type: depends-on
    description: Facts are the rows of the ledger's identity layer, and that page shows the current-value view built from them
  - to: /concepts/identity-keys
    type: relates-to
    description: Every fact hangs off one identity, and the identity key decides which one
  - to: /concepts/adapter-tiers
    type: relates-to
    description: The adapter that writes a fact is its source, and a process adapter is the kind that can send a confidence below 1
  - to: /guides/interrogate
    type: relates-to
    description: Asking what gtme knows about one record, and where each value came from
  - to: /guides/top-up
    type: relates-to
    description: Re-running a pipeline so the cache window decides what gets paid for again
  - to: /reference/cli/show
    type: relates-to
    description: Every flag of gtme show, including --provenance
  - to: /reference/cli/vacuum
    type: relates-to
    description: The command that deletes expired payloads and nothing else
  - to: /reference/ledger-schema
    type: relates-to
    description: The DDL for field_values, field_value_ranks, and payloads
  - to: /spec#3-ledger-schema--decided
    type: decided-by
    description: The ranking rule is one SQL view, and no second implementation may exist
  - to: /decisions#adr-003
    type: decided-by
    description: The current value is the highest-confidence row in the freshness window, newest first on a tie
  - to: /decisions#adr-030
    type: decided-by
    description: Extracted values are facts and kept forever, while raw responses are purgeable cache
  - to: /decisions#adr-039
    type: decided-by
    description: An AI step's output is reused while its prompt, model, and inputs are unchanged
---

# Facts have provenance

Save this next to `contacts.csv` from [See it run](/start/show-me) as `facts.yaml`. It runs [armed](/concepts/gate-ladder), spends no real money, and sends nothing. `demo/enrich` records a pretend $0.01 per record.

```yaml
name: facts
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
```

Run it against a fresh ledger:

```sh
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run facts.yaml
```

Jane got promoted, and the CSV still says `VP Marketing`. Add a second source for her title, a `sql/transform` step that writes `CMO`, to the end of `facts.yaml`:

```yaml
  - id: promote
    use: sql/transform
    with:
      uses: [email]
      provides: [title]
      query: >
        SELECT identity_id, 'CMO' AS title FROM current_values
        WHERE field = 'email' AND value = 'jane.doe@acme.com'
```

Run the file again:

```sh
gtme run facts.yaml
```

The output is similar to the following:

```
...
step     adapter        in  out  empty  cached  filtered  failed  cost  avoided
source   csv/source     0   3    -      0       -         -       $0    -
score    demo/enrich    3   0    -      3       -         -       $0    $0.0300
promote  sql/transform  3   1    2      0       -         -       $0    -
total: $0 spent, $0.0300 avoided via cache (3 records skipped)
```

Now ask the ledger for every `title` row it holds for Jane, ranked:

```sh
gtme query "SELECT value, source, confidence, created_at, rank
            FROM field_value_ranks
            WHERE field = 'title'
              AND identity_id = (SELECT id FROM identities WHERE identity_key = 'jane.doe@acme.com')"
```

```
{"confidence":1,"created_at":"2026-09-26T17:14:39.416Z","rank":1,"source":"sql/transform @ 87a73c20aceb","value":"\"CMO\""}
{"confidence":1,"created_at":"2026-09-26T17:14:39.413Z","rank":2,"source":"csv/source@1","value":"\"VP Marketing\""}
{"confidence":1,"created_at":"2026-09-26T17:14:39.384Z","rank":3,"source":"csv/source@1","value":"\"VP Marketing\""}
3 rows
```

Hand that query to your agent with a different field name when someone asks why a value is what it is.

## What you just saw

**A fact is one row saying who wrote what about whom, how sure it was, and when.** Each row in `field_values` is about one identity and one [canonical field](/concepts/canonical-fields), and carries its provenance:

- `value`: stored as JSON, which is why the strings print with escaped quotes.
- `source`: the [adapter](/concepts/adapter-tiers) or step that wrote it, with its version or a hash of its query.
- `confidence`: how sure the source was, from 0 to 1.
- `created_at`: when the runner wrote the row, not when the vendor learned it.

[The ledger schema](/reference/ledger-schema) lists the rest. The query added `rank`, the row's place among every row for that field.

**The ledger keeps every row.** Each run of the source wrote `VP Marketing` again, and `promote` added `CMO` beside them. All three have confidence 1, so the newest wins. The rule is highest confidence first, then newest, and it lives in one view, `field_value_ranks`, a saved query the database answers on demand, per the decision record [ADR-003](/decisions#adr-003).

To check a winner, ask `gtme show` for its provenance:

```sh
gtme show jane.doe@acme.com --provenance --fields title
```

```json
{
  "entity_type": "person",
  "fields": {
    "title": {
      "confidence": 1,
      "created_at": "2026-09-26T17:14:39.416Z",
      "run_id": "01M3FBCEDNG29WPGMFDNQ2N9FD",
      "source": "sql/transform @ 87a73c20aceb",
      "value": "CMO"
    }
  },
  "identity_key": "jane.doe@acme.com",
  "identity_key_tier": "email"
}
```

**To make a source win, run it last.** With equal confidence the newest row wins, and `promote` runs after the source on every run, so `CMO` keeps winning. Take the step out and the next run's CSV row is newest, so `VP Marketing` wins again. There's no per-source priority setting. The only way to outrank by confidence is a process adapter, one you write as your own program, that sends a lower confidence for the source you trust less.

**Confidence is set by the source, per field, on each record it sends.** It's optional and defaults to 1. Every shipped adapter leaves it unset, Apollo included, so today a conflict goes to the newest row. A process adapter can send `0.6` for a guessed email, and that guess loses to a verified email at 1 however much newer it is. The [record message](/reference/wire-protocol), the record an adapter sends back over the wire protocol, has a `confidence` map for it.

**A cache window decides when a step's own output is too old to reuse.** `cache: 30d` on `score` means a `demo.score` written in the last 30 days counts as fresh, which is why the second [receipt](/concepts/runs-and-receipts) shows 3 cached and no spend. The check looks only at the step's own output, not at whether its inputs changed. An `ai/*` step reuses its answer while the prompt, model, and inputs are unchanged, with no window unless `cache:` is set ([ADR-039](/decisions#adr-039)). After 30 days, `score` pays again and writes a new row, and the old one stays.

The window belongs to the writing step and only decides whether it runs. A step reading its inputs applies no window, so it sees the same winner as `gtme show`. `gtme show` and `gtme query` apply no window either, so all three agree.

## Why it's this way

**Append-then-rank keeps the history and one answer.** Steps read from the ledger instead of passing records along, for the reasons on [the ledger](/concepts/ledger) page. The cost is growth: a source re-run writes every field again, changed or not.

**A raw vendor response is a payload, kept apart from facts.** When an adapter attaches the response a record came from, the runner keeps it in `payloads` for 90 days unless the adapter declares otherwise, and no step reads it. `gtme vacuum` deletes expired payloads, and so does the start of every run that isn't simulated, dry-run included, without touching a fact ([ADR-030](/decisions#adr-030)).

**Confidence rarely decides anything today.** Because every source writes 1 unless it says otherwise, the ranking is newest-wins in practice. When a lower-confidence source does arrive, an old row at 1 keeps beating it, so when a value looks stale, check `--provenance`.

## Where it shows up

- [Interrogate](/guides/interrogate) is this page as a task: what gtme knows about one record, and from where.
- [Top up](/guides/top-up) re-runs a pipeline and lets the cache window decide what gets paid for again.
- [`gtme show`](/reference/cli/show) and [`gtme vacuum`](/reference/cli/vacuum) are the lookup pages.
