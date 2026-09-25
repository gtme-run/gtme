---
name: gtme docs
description: "How gtme works and how to use it, in four collections: Start, Concepts, Guides, and Reference"
links:
  - to: /start
    type: relates-to
    description: From nothing to a receipt through one of four doors; the place to begin
  - to: /concepts
    type: relates-to
    description: The ideas behind the tool, simplest first; the ledger is the one everything else stands on
---

# gtme docs

gtme is a CLI for GTM data pipelines. A small YAML file describes a campaign, and the runner executes it against vendor [adapters](/concepts/adapter-tiers). An append-only [ledger](/concepts/ledger) remembers every fact, so re-runs cost less and nothing is delivered twice.

The docs are four collections, each with one job.

- [Start](/start) gets you from nothing to a [receipt](/concepts/runs-and-receipts). Four [doors](/start/show-me), sorted by what you have on hand, from no keys and $0 up to your whole stack.
- [Concepts](/concepts) explain what you saw, one idea per page, ordered so each page only needs the ones before it. The ledger is the first one that matters.
- Guides do one task at a time, with every command and its output.
- Reference is for looking things up: every CLI verb, every pipeline.yaml key, every adapter, every field.

SPEC.md and DECISIONS.md at the repo root are canon. These pages explain and link to them; they never redefine them.
