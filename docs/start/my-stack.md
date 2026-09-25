---
name: "Door 3: my stack"
description: Run a live Apollo-to-Instantly pipeline on your own keys, climbing simulate, plan, and dry-run before one armed run that sends
order: 4
links:
  - to: /concepts/gate-ladder
    type: depends-on
    description: Each command on this page is one rung of the ladder, taken in order
  - to: /concepts/ledger
    type: relates-to
    description: The dry run's paid results land in the ledger, so the armed run reads them instead of paying again
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The dry-run receipt is the artifact a human reads before arming
  - to: /start/show-me
    type: relates-to
    description: Door 1 runs the same pipeline shape offline, with no keys
  - to: /start/my-csv
    type: relates-to
    description: The previous door, with one model key and a local CSV as the target
  - to: /start/add-a-vendor
    type: relates-to
    description: The next door, an adapter for a vendor gtme doesn't ship
  - to: /guides/connect-your-stack
    type: relates-to
    description: The full walkthrough of secrets, plan, dry-run, and arming for any pipeline
  - to: /decisions#adr-040
    type: decided-by
    description: Why the dry run reads the Instantly campaign and checks its sequence before any record moves
  - to: /decisions#adr-043
    type: decided-by
    description: Why Apollo search is free and only the reveal past the filter spends credits
---

# Door 3: my stack

**This door needs Apollo, Harvest, Anthropic, and Instantly keys, plus an Instantly campaign that exists and is active.**

**Nothing spends until the dry run, and only the armed run sends.** The dry run spends Apollo reveal credits, Harvest calls, and model tokens. Sending means adding people to your live Instantly campaign, which emails them on its own schedule.

This is Door 1 on live vendors. You'll climb the [gate ladder](/concepts/gate-ladder) in order: simulate, plan, dry-run, armed.

## The file you'll run

**Fetch the pipeline:**

```sh
curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/apollo-to-instantly.yaml
```

Here it is without comments:

```yaml
name: apollo-to-instantly
version: 1

source:
  use: apollo/search
  with:
    query: "vp marketing, saas, 50-200 employees"
    titles: ["vp marketing", "head of marketing"]
    employee_ranges: ["50,200"]
    limit: 500

steps:
  - id: icp-filter
    use: ai/filter
    uses: [first_name, title, company_name]
    with:
      template: >
        Keep only contacts likely to own outbound tooling decisions.
      batch_size: 25

  - id: reveal
    use: apollo/enrich
    when: icp-filter.passed
    cache: 30d

  - id: linkedin
    use: harvest/profile
    when: icp-filter.passed
    cache: 30d
    with:
      posts_limit: 3

  - id: personalize
    use: ai/compose
    when: icp-filter.passed
    uses: [recent_posts, role_history]
    with:
      template: >
        Write first_line and ps_line using recent_posts and role_history.
        first_line references something specific and recent; ps_line is one
        short, low-pressure sentence. No flattery, no exclamation marks.
      batch_size: 25

  - id: send
    use: instantly/add-to-campaign
    with:
      campaign: "Q3 VP Marketing"
    variables:
      first_line: first_line
      ps_line: ps_line
    idempotency: email
```

The lines you'll change:

- **Who it searches for** is `query:`, `titles:`, and `employee_ranges:`. Apollo search is free ([ADR-043](/decisions#adr-043)).
- **How many people it pulls** is `limit:`.
- **Who passes the filter** is the `template:` under `icp-filter`. Only they cost a reveal.
- **What the lines say** is the `template:` under `personalize`.

We recommend `limit: 5` for the first dry run, raised after the [receipt](/concepts/runs-and-receipts) reads right:

```yaml
    limit: 5
```

Without Harvest, delete the `linkedin` step and point `personalize` at Apollo's fields, or plan fails:

```yaml
    uses: [first_name, title, company_name]
    with:
      template: >
        Write first_line and ps_line using title and company_name.
```

With Harvest, someone it finds nothing for gets lines written without posts.

## Simulate it

Simulate spends nothing and sends nothing:

```sh
gtme run apollo-to-instantly.yaml --simulate
```

The output is similar to the following:

```
simulate: ignoring missing credentials (4 plan problems:
...
simulate: fixtures only — no network, no spend, nothing sends, nothing persists
run 01M3CX161QEV1CKXPXSS382TEC (apollo-to-instantly)
...
send: 2 in, 0 out, 0 cached, 0 filtered, 0 failed, 2 held (dry run)

run 01M3CX161QEV1CKXPXSS382TEC — done (SIMULATED — fixtures only; nothing sent, nothing persisted)
step         adapter                    in  out  empty  cached  filtered  failed  cost     avoided
source       apollo/search              0   2    -      0       -         -       $0       -
icp-filter   ai/filter                  2   2    -      0       -         -       $0       -
reveal       apollo/enrich              2   2    -      0       -         -       $0.0200  -
linkedin     harvest/profile            2   0    -      0       -         -       $0       -
personalize  ai/compose                 2   2    -      0       -         -       $0       -
send         instantly/add-to-campaign  2   0    -      0       -         -       $0       -
personalize: 2 missing recent_posts, role_history — dispatched anyway (on_missing: run); set on_missing: skip or fail to hold them
simulation gap: linkedin (harvest/profile) — 2 record(s) passed through untouched (no fixtures to serve)
send: preflight skipped — the target is not read under --simulate; --dry-run checks it
send: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
  nh:ed48d7bfcbdb5e0ce36005a8cec0be0812e91c1f4a4f542bc86eed418173f417
    first_line: "Fixture first line for nh:ed48d7bfcbdb5e0ce36005a8cec0be0812e91c1f4a4f542bc86eed418173f417"
    ps_line: "Fixture ps line for nh:ed48d7bfcbdb5e0ce36005a8cec0be0812e91c1f4a4f542bc86eed418173f417"
...
total: $0.0200 (estimated) spent
```

Apollo answered from fixtures, which are recorded responses, and the model from canned text; `$0.0200` is what the reveals would cost. Harvest has no fixtures, so `linkedin` passed records through untouched, and `on_missing: run` let `personalize` write anyway. "held (dry run)" and "without --dry-run to arm" are the CLI's words under simulate too, not a third mode.

**The `send: resolved variables` block, each person's values for the email, is the part to read.** The dry run prints the same block with real people.

## Plan it

Plan spends nothing and sends nothing:

```sh
gtme plan apollo-to-instantly.yaml
```

Without keys, the output is the following:

```
gtme: 4 plan problems:
  - step "source": missing credential APOLLO_API_KEY (set it in the environment or run `gtme secret set APOLLO_API_KEY`)
  - step "reveal": missing credential APOLLO_API_KEY (set it in the environment or run `gtme secret set APOLLO_API_KEY`)
  - step "linkedin": missing credential HARVEST_API_KEY (set it in the environment or run `gtme secret set HARVEST_API_KEY`)
  - step "send": missing credential INSTANTLY_API_KEY (set it in the environment or run `gtme secret set INSTANTLY_API_KEY`)
```

Plan exits with code `3`, which means every problem is a missing credential, and each line names its fix. Plan only warns about the Anthropic key; without it, every record fails at `icp-filter` and the receipt names the key.

Each command prompts for a key and saves it to `~/.gtme/secrets`, a home-directory file only you can read:

```sh
gtme secret set APOLLO_API_KEY
gtme secret set HARVEST_API_KEY
gtme secret set ANTHROPIC_API_KEY
gtme secret set INSTANTLY_API_KEY
```

Run plan again to see each step's estimated cost.

## Dry-run it

**Set `campaign:` to an Instantly campaign that exists and is active.** Before any record moves, the runner reads the campaign and stops if its sequence emails, A/B variants included, lack `{{first_line}}` and `{{ps_line}}` ([ADR-040](/decisions#adr-040)). Pausing the campaign in Instantly stops its emails.

The dry run spends Apollo reveal credits, Harvest calls, and model tokens. It sends nothing:

```sh
gtme run apollo-to-instantly.yaml --dry-run
```

<!-- receipt: capture with live accounts before publish -->

In the receipt, check that the campaign checks passed, the count matches `limit`, every `first_line` is filled and sane, and the `cost` column is what you expected.

## Arm it

**The armed run searches Apollo again, so it can include people the dry run didn't.** People already judged reuse their verdicts, reveals, and lines from the [ledger](/concepts/ledger); anyone new is judged, revealed, and sent unread. Arm right after reading the dry run. To pin the list exactly, qualify into a [group](/concepts/groups) and send from it.

The armed run spends only on people the ledger lacks, and it sends:

```sh
gtme run apollo-to-instantly.yaml
```

<!-- receipt: capture with live accounts before publish -->

Delivery is keyed on email, so a second run adds nobody twice:

```sh
gtme run apollo-to-instantly.yaml
```

<!-- receipt: capture with live accounts before publish -->

That receipt shows `cached` on the paid steps, the money saved in `avoided`, and `0` out on `send`.

## Next

**[Connect your stack](/guides/connect-your-stack) covers the same climb for any pipeline.** [Door 4](/start/add-a-vendor) adds a vendor gtme doesn't ship. For Claude Code, paste this line; the agent stops at the dry-run receipt and never arms:

```
Follow gtme.run/start.md, door 3, with apollo-to-instantly.yaml at limit: 5
```
