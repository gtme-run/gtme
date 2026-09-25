---
name: Start
description: Install gtme and walk one of four doors to a receipt, spending nothing until a page says so in plain words right before the command
for: "You have gtme installed, or are about to, and want a first result before reading anything else."
learn:
  - "how to pick a door by what you have on hand"
  - "what each door needs, spends, and ends with"
  - "what a receipt is"
order: 1
links:
  - to: /start/install
    type: relates-to
    description: One binary, then gtme version proves it's there
  - to: /start/show-me
    type: relates-to
    description: Door 1, no keys, $0; a whole pipeline offline from recorded samples, then a persisted run whose second pass shows dollars avoided
  - to: /start/my-csv
    type: relates-to
    description: Door 2, a CSV and an Anthropic key, cents on the model; your rows judged and written to a file
  - to: /start/my-stack
    type: relates-to
    description: Door 3, four vendor keys and a live Instantly campaign; simulate, plan, dry-run, then one armed run
  - to: /start/add-a-vendor
    type: relates-to
    description: Door 4, no keys, $0; a binding that verifies and simulates for a vendor gtme doesn't ship
  - to: /start/for-agents
    type: relates-to
    description: The paste lines that hand gtme to Claude Code, and the rules the agent follows
  - to: /concepts
    type: relates-to
    description: Once a door has produced a receipt, Concepts explains what the run did and remembered
---

# Start

Every page here ends in a [receipt](/concepts/runs-and-receipts): a table of what a pipeline did, what it cost, and what it would have sent. [Install](/start/install) gets the binary onto your machine. Then pick a door by what you have on hand.

| Door | Needs | Spends | Ends with |
|---|---|---|---|
| [1. Show me](/start/show-me) | nothing | $0 | a receipt from recorded samples, then a second run that skips what the first one did |
| [2. My CSV](/start/my-csv) | a CSV and an Anthropic key | cents, on the model | your rows judged and written to a file |
| [3. My stack](/start/my-stack) | Apollo, Harvest, Anthropic, and Instantly keys, plus a live Instantly campaign | vendor credits and model tokens, [gated](/concepts/gate-ladder) | a dry-run receipt, one armed run, then a re-run that sends nobody twice |
| [4. Add a vendor](/start/add-a-vendor) | `jq` and one request to a free public API | $0 | a new [adapter](/concepts/adapter-tiers) that verifies and simulates |

The doors are a menu, not a sequence. Each is one pipeline file you fetch, and every command is safe to re-run. Nothing sends and nothing spends until a page says so, in plain words, right before the command. We recommend Door 1 first, whatever you have on hand: it costs nothing and proves the install.

If an agent is going to do the typing, [For agents](/start/for-agents) has the two paste lines and the rules it follows. When you have a receipt and want to know what happened underneath, go to [Concepts](/concepts).
