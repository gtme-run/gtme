---
name: Concepts
description: The ideas behind gtme in two parts, how a run works and how the model works, ordered so each page only needs the ones before it
for: "You've seen at least one receipt and want to know what happened underneath."
learn:
  - "which part is yours, the run or the model"
  - "the order to read the concepts in"
  - "which pages are the data model"
order: 2
roles: [operator, builder, extender, agent]
links:
  - to: /concepts/gate-ladder
    type: relates-to
    description: How a run works, first page; what each command spends and sends
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: How a run works; the table a run ends with, column by column
  - to: /concepts/ledger
    type: relates-to
    description: The one concept the rest depend on; facts live in an append-only table and steps read from it instead of passing records
  - to: /concepts/groups
    type: relates-to
    description: How a run works; where one run's decisions become a set the next pipeline reads
  - to: /concepts/participants
    type: relates-to
    description: How a run works; how a person or an agent answers a step and the run collects it
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: How a run works; a frozen run travels as a folder of text
  - to: /concepts/pipeline
    type: relates-to
    description: How the model works, first page; every key in a pipeline file
  - to: /concepts/steps-and-roles
    type: relates-to
    description: How the model works; what each role reads, writes, and costs
  - to: /concepts/identity-keys
    type: relates-to
    description: How the model works; what makes a row the same record across runs and vendors
  - to: /concepts/canonical-fields
    type: relates-to
    description: How the model works; one name with one meaning across adapters
  - to: /concepts/facts
    type: relates-to
    description: How the model works; which value wins when two rows disagree
  - to: /concepts/types-and-traverse
    type: relates-to
    description: How the model works; a run past one type of record
  - to: /concepts/adapter-tiers
    type: relates-to
    description: How the model works; where an adapter comes from and what each tier can express
  - to: /start
    type: relates-to
    description: Concepts assume you've seen at least one receipt; Start is where you get one
---

# Concepts

Each page explains one idea well enough that you can predict what gtme will do. They come in two parts, and most readers need only one.

**How a run works** is for anyone who runs a pipeline, whether or not they wrote it. [The gate ladder](/concepts/gate-ladder) says what each command spends and sends. [Runs and receipts](/concepts/runs-and-receipts) reads the table a run ends with.

[The ledger](/concepts/ledger) is where every fact goes, and it's why the second run of a pipeline is cheaper than the first. [Groups and segments](/concepts/groups) is where a run's decisions become a set the next pipeline can read. [Participants](/concepts/participants) is how a person or an agent answers a step. [A campaign is a folder](/concepts/campaign-is-a-folder) is how a finished run travels to a teammate or another machine.

**How the model works** is for anyone writing or extending a pipeline. [A pipeline is a YAML file](/concepts/pipeline) and [steps and roles](/concepts/steps-and-roles) follow a file from authoring to a plan. [Identity keys](/concepts/identity-keys), [canonical fields](/concepts/canonical-fields), and [provenance](/concepts/facts) are the three pages every guide depends on. [Types and traverse](/concepts/types-and-traverse) is how a pipeline grows past one type of record, and [two adapter tiers](/concepts/adapter-tiers) is how it grows past the adapters gtme ships.

The pages are also numbered for a linear read, simplest first, where each page only needs the ones before it. If you haven't run anything yet, [Start](/start) will get you a receipt in a few minutes, and the concepts read better with one in front of you.
