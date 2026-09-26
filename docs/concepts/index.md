---
name: Concepts
description: The ideas behind gtme, ordered simplest to most complex, so that each page only needs the ones before it
for: "You've seen at least one receipt and want to know what happened underneath."
learn:
  - "the order to read the concepts in"
  - "which pages are the data model"
order: 2
links:
  - to: /concepts/ledger
    type: relates-to
    description: The one concept the rest depend on; facts live in an append-only table and steps read from it instead of passing records
  - to: /start
    type: relates-to
    description: Concepts assume you've seen at least one receipt; Start is where you get one
---

# Concepts

Each page explains one idea well enough that you can predict what gtme will do. They run along one spine: author a file, run it safely, understand what the run remembered, grow knowledge across runs, extend the tool.

The first four pages follow a file from authoring to a finished run: [a pipeline is a YAML file](/concepts/pipeline), [steps and roles](/concepts/steps-and-roles), [the gate ladder](/concepts/gate-ladder), and [runs and receipts](/concepts/runs-and-receipts). The page most readers need next is [the ledger](/concepts/ledger). It's where every fact goes, and it's why the second run of a pipeline is cheaper than the first. [Identity keys](/concepts/identity-keys), [canonical fields](/concepts/canonical-fields), and [provenance](/concepts/facts) are the three pages that follow it and that every guide depends on. [Groups and segments](/concepts/groups) is where a run's decisions become a set the next pipeline can read.

If you haven't run anything yet, [Start](/start) will get you a receipt in a few minutes, and the concepts read better with one in front of you.
