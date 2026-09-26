---
name: gtme docs
description: "How gtme works and how to use it, with one reading path for each kind of reader and four collections behind them"
roles: [operator, builder, extender, agent]
for: "Anyone who has heard of gtme and wants to know where to start reading."
learn:
  - "which reading path is yours, and what it skips"
  - "which of the four collections answers the question you have"
  - "where the canon lives and how the docs relate to it"
links:
  - to: /start
    type: relates-to
    description: From nothing to a receipt, four ways, sorted by what you have on hand; the place to begin
  - to: /concepts
    type: relates-to
    description: The ideas behind the tool in two parts, how a run works and how the model works
  - to: /start/show-me
    type: relates-to
    description: Where the person who runs campaigns begins
  - to: /start/my-csv
    type: relates-to
    description: Where the person who builds pipelines begins
  - to: /start/add-a-vendor
    type: relates-to
    description: Where the person adding a vendor begins
  - to: /start/for-agents
    type: relates-to
    description: Where an agent driving gtme begins
---

# gtme docs

gtme is a CLI for GTM data pipelines. A small YAML file describes a campaign, and the runner executes it against vendor [adapters](/concepts/adapter-tiers). An append-only [ledger](/concepts/ledger) remembers every fact, so re-runs cost less and nothing is delivered twice.

## Pick your path

Each path lists the pages one kind of reader needs, in order, and nothing else. Every page names its readers in its frontmatter, so the graph can rebuild these paths.

| You | Begin with | Then read |
|---|---|---|
| **I run campaigns.** You read plans and receipts, answer review steps, top up, and report. You may never write a pipeline from scratch. | [See it run](/start/show-me) | [The gate ladder](/concepts/gate-ladder), [Runs and receipts](/concepts/runs-and-receipts), The ledger, [Groups and segments](/concepts/groups), [Participants](/concepts/participants) |
| **I build pipelines.** You write pipeline files, often with an agent beside you. | [Your CSV](/start/my-csv) | [A pipeline is a YAML file](/concepts/pipeline), [Steps and roles](/concepts/steps-and-roles), [Identity keys](/concepts/identity-keys), [Canonical fields](/concepts/canonical-fields), [Facts have provenance](/concepts/facts), [Types and traverse](/concepts/types-and-traverse), [A campaign is a folder](/concepts/campaign-is-a-folder) |
| **I add a vendor.** You write a binding or a process adapter for a vendor gtme doesn't ship. | [Add a vendor](/start/add-a-vendor) | Two adapter tiers, then the Reference pages on the binding manifest and the wire protocol |
| **I am an agent.** Claude or another model driving gtme for one of the other three readers. | [For agents](/start/for-agents) | The Reference collection, and `gtme help --agent` |

## The four collections

Each has one job.

- [Start](/start) gets you from nothing to a receipt. Four ways to begin, sorted by what you have on hand, from no keys and $0 up to your whole stack.
- [Concepts](/concepts) explain what you saw, in two parts: how a run works, for anyone who runs one, and how the model works, for anyone who writes or extends one.
- Guides do one task at a time, with every command and its output.
- Reference is for looking things up: every CLI verb, every pipeline.yaml key, every adapter, every field.

SPEC.md and DECISIONS.md at the repo root are canon. These pages explain and link to them; they never redefine them.
