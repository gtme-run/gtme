---
name: "Door 1: show me"
description: Run a whole outbound pipeline offline with no keys, then run a keyless pipeline twice and watch the second run skip what the first one paid for
for: "Your first run. You have gtme and no API keys, and you want to see a whole pipeline work before connecting anything."
learn:
  - "run an outbound pipeline offline from recorded samples"
  - "read a receipt"
  - "why a second run costs less and delivers nobody twice"
order: 2
links:
  - to: /start/install
    type: depends-on
    description: Every door starts with the gtme binary on your PATH
  - to: /start/my-csv
    type: relates-to
    description: The next door runs the same kind of pipeline over your own rows with an Anthropic key
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate is the first rung of the ladder and armed is the last; this door shows both with nothing at stake
  - to: /concepts/ledger
    type: relates-to
    description: The second run is cheap because the first run's facts are in the ledger
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: How to read the step table every run prints, and what "never delivers twice" covers
  - to: /decisions#adr-028
    type: decided-by
    description: Why a simulated run writes no facts, so running it again gives the same receipt
  - to: /decisions#adr-056
    type: decided-by
    description: Why demo/enrich charges a pretend price, so the receipt's arithmetic is real with zero keys
---

# Door 1: show me

**This door needs gtme and nothing else.** No API keys and no accounts, and it spends $0 and sends nothing. If you don't have the binary yet, [install it](/start/install) first.

Make a folder, keep the ledger inside it so demo people stay out of your real one, and run the demo offline:

```sh
mkdir -p gtme-start && cd gtme-start
export GTME_LEDGER=./ledger.db
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/demo.yaml
gtme run demo.yaml --simulate
```

The output is similar to the following:

```
simulate: ignoring missing credentials (3 plan problems:
  - step "source": missing credential APOLLO_API_KEY (set it in the environment or run `gtme secret set APOLLO_API_KEY`)
  - step "reveal": missing credential APOLLO_API_KEY (set it in the environment or run `gtme secret set APOLLO_API_KEY`)
  - step "send": missing credential INSTANTLY_API_KEY (set it in the environment or run `gtme secret set INSTANTLY_API_KEY`))
simulate: fixtures only — no network, no spend, nothing sends, nothing persists
run 01M3CWQATKY6BTBKFTKE8RM81J (demo)
...
run 01M3CWQATKY6BTBKFTKE8RM81J — done (SIMULATED — fixtures only; nothing sent, nothing persisted)
step    adapter                    in  out  empty  cached  filtered  failed  cost     avoided
source  apollo/search              0   1    -      0       -         -       $0       -
fit     ai/filter                  1   1    -      0       -         -       $0       -
reveal  apollo/enrich              1   1    -      0       -         -       $0.0100  -
lines   ai/compose                 1   1    -      0       -         -       $0       -
send    instantly/add-to-campaign  1   0    -      0       -         -       $0       -
send: preflight skipped — the target is not read under --simulate; --dry-run checks it
send: resolved variables for 1 record(s) — review, then run again without --dry-run to arm:
  nh:ed48d7bfcbdb5e0ce36005a8cec0be0812e91c1f4a4f542bc86eed418173f417
    first_line: "Fixture first line for nh:ed48d7bfcbdb5e0ce36005a8cec0be0812e91c1f4a4f542bc86eed418173f417"
    first_name: "Jane"
total: $0.0100 (estimated) spent
```

**That's a [receipt](/concepts/runs-and-receipts): one row per step, with what went in, what came out, and what it cost.** The three "missing credential" lines aren't an error; they're the keys a real run would need. [Simulate](/concepts/gate-ladder) never touches the network: vendor steps answer from recorded samples, and AI steps return canned text that says so. The `$0.0100` on the reveal row is what a real run would have paid, and `1 held` on the send row means the email variables were printed and nothing was sent. A simulated run writes nothing down, so running it again gives the same receipt.

## The file you just ran

**`demo.yaml` is a whole outbound pipeline in 30 lines.** Here it is with the comments stripped:

```yaml
name: demo
version: 1

source:
  use: apollo/search
  with:
    query: vp marketing, saas
    limit: 1

steps:
  - id: fit
    use: ai/filter
    uses: [title, company_name]
    with:
      template: >
        Keep only people who plausibly own outbound tooling decisions.

  - id: reveal
    use: apollo/enrich
    when: fit.passed
    cache: 30d

  - id: lines
    use: ai/compose
    when: fit.passed
    uses: [full_name, title, company_name]
    with:
      template: >
        Write first_line and ps_line for a short, honest intro email.

  - id: send
    use: instantly/add-to-campaign
    with: { campaign: "Q3 VP Marketing" }
    variables:
      first_name: first_name
      first_line: first_line
    idempotency: email
```

Search Apollo, keep the people an AI judge says own outbound tooling, and pay for contact details only after the judge says yes. Then write a first line and add them to an Instantly campaign. Each `use:` names an [adapter](/concepts/adapter-tiers) as `vendor/verb`. The next section is about two of these lines: `cache: 30d` on `reveal` and `idempotency: email` on `send`.

## Run something twice

**The second file runs armed, which means for real: it writes to the ledger file you set.** It still spends $0 and sends nothing. `cache.yaml` reads three fictional people from a CSV, scores them with `demo/enrich`, keeps anyone scoring 70 or more, and writes the keepers to `out.csv` in this folder. `demo/enrich` is gtme's built-in scorer: no vendor, a pretend $0.01 per record, so the receipt has real arithmetic.

1. Download the pipeline and its CSV:

    ```sh
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/cache.yaml
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
    ```

1. Run it. This is the armed run; the $0.03 on the receipt is the pretend price:

    ```sh
    gtme run cache.yaml
    ```

    ```
    run 01M3CWQDPB13EJGJVM8FSN38HK (cache)
    ...
    step    adapter      in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source   0   3    -      0       -         -       $0       -
    score   demo/enrich  3   3    -      0       -         -       $0.0300  -
    keep    sql/filter   3   1    -      0       2         -       $0       -
    out     csv/deliver  1   1    -      0       -         -       $0       -
    total: $0.0300 (estimated) spent
    ```

1. Run it again:

    ```sh
    gtme run cache.yaml
    ```

    ```
    run 01M3CWQDQ9N9W3XTC922W5S569 (cache)
    ...
    step    adapter      in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source   0   3    -      0       -         -       $0    -
    score   demo/enrich  3   0    -      3       -         -       $0    $0.0300
    keep    sql/filter   3   1    -      0       2         -       $0    -
    out     csv/deliver  1   0    -      1       -         -       $0    $0.0000
    total: $0 spent, $0.0300 avoided via cache (4 records skipped)
    ```

**Read the `score` row: `cached 3`, `avoided $0.0300`.** Each person already had a score in the [ledger](/concepts/ledger), inside its 30-day window, so the scorer didn't run. Then read `out`: `cached 1`. Jane went to `out.csv` last time under the same email, so she wasn't delivered again; that's `idempotency: email`. It holds per delivery target, so the same person can still be added to a different campaign on purpose.

The counter is real and the dollars are pretend. With your own rows and a real price behind a step, the `avoided` column is money you didn't spend.

1. See what the ledger kept about Jane:

    ```sh
    gtme show jane.doe@acme.com
    ```

    ```json
    {
      "deliveries": [
        {
          "created_at": "2026-09-25T18:20:01.620Z",
          "run_id": "01M3CWQDPB13EJGJVM8FSN38HK",
          "scope": "out.csv",
          "status": "accepted",
          "target": "csv/deliver"
        }
      ],
      "entity_type": "person",
      "fields": {
        "company_domain": "acme.com",
        "demo.note": "synthetic — demo/enrich called no vendor",
        "demo.score": 100,
        "email": "jane.doe@acme.com",
        "full_name": "Jane Doe",
        "title": "VP Marketing"
      },
      "identity_key": "jane.doe@acme.com"
    }
    ```

    The `deliveries` entry is the row that stopped her second delivery, and `demo.note` is the scorer admitting it called no one. `gtme show` only reads.

When you're done, `rm ledger.db out.csv` and the demo is gone. Your real ledger at `~/.gtme/ledger.db` was never touched.

## Next

**[Door 2: my CSV](/start/my-csv) runs this shape over your own contacts with one Anthropic API key.** It costs a few cents on the model. If Claude Code is doing the typing, [the agent page](/start/for-agents) has the line to paste.
