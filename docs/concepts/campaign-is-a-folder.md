---
name: A campaign is a folder
description: gtme freeze --bundle packs a run's exact configuration into a folder of text files with a manifest of hashes, and gtme run accepts that folder anywhere
for: "You've run a pipeline and want to hand the campaign to a teammate, an agent, or another machine, and know what arrives and what doesn't."
learn:
  - "what a bundle contains and what its manifest records"
  - "what gtme run checks before it runs a bundle"
  - "what stays with the machine: credentials, inputs, and ledger state"
  - "why an edited bundle refuses to run"
order: 12
roles: [builder, operator]
links:
  - to: /concepts/pipeline
    type: depends-on
    description: A bundle is a frozen pipeline file plus everything that file references
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Bindings are data and travel in a bundle; process adapters are executables and don't
  - to: /concepts/gate-ladder
    type: relates-to
    description: Simulate on a bundle serves its bindings from the fixtures packed inside it
  - to: /concepts/groups
    type: relates-to
    description: Group names in a bundled pipeline resolve against the ledger the bundle runs on, never against the bundle
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: A bundle is frozen from one run, and the manifest's source_run_id names it
  - to: /guides/bundles
    type: relates-to
    description: Freezes, shares, and adapts a campaign bundle end to end
  - to: /guides/connect-your-stack
    type: relates-to
    description: Where the machine that receives a bundle sets its own keys and climbs from simulate to armed
  - to: /concepts/ledger
    type: relates-to
    description: Group membership, cache, and run history live in the ledger and never travel in a bundle
  - to: /reference/bundles
    type: relates-to
    description: The bundle layout and every manifest key
  - to: /reference/cli/freeze
    type: relates-to
    description: The command that writes a bundle, or prints a pipeline's YAML without one
  - to: /decisions#adr-029
    type: decided-by
    description: Why freeze output is a portable folder, and the three guarantees it keeps
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The normative bundle contents, and the rule that gtme run accepts a bundle path wherever it accepts a pipeline file
---

# A campaign is a folder

Take the three-person example from [A pipeline is a YAML file](/concepts/pipeline). Run it once so there's a run to freeze, then freeze it. `last` names your most recent run:

```sh
gtme run hello.yaml
gtme freeze last --bundle ./hello-bundle
```

The last line of output is similar to the following:

```
froze run 01M3FBDQ11PD7C2NFS42DN24G7 into bundle ./hello-bundle (4 steps) — self-contained except credentials and input files
```

Here's what landed:

```sh
find hello-bundle -type f | sort
```

```
hello-bundle/manifest.json
hello-bundle/pipeline.yaml
hello-bundle/registry/company.json
hello-bundle/registry/person.json
hello-bundle/registry/post.json
```

```sh
cat hello-bundle/manifest.json
```

```json
{
  "bundle_format_version": 1,
  "name": "hello",
  "source_run_id": "01M3FBDQ11PD7C2NFS42DN24G7",
  "created_at": "2026-09-26T17:15:21.032Z",
  "gtm_version": "v0.6.2-0.20260926171205-74078a28d320",
  "contents": {
    "pipeline.yaml": "c3dd12fe876050d84d4332a8a34073f04c1218af26be31fc52309c0b6f3289f5",
    "registry/company.json": "56a4a6434fbb77bd24e2fc7f013abb28129a23f2d88d367ae591b9f7230814b5",
    "registry/person.json": "2666a0850834b1542364a8c5deb7ce27e6033f931fb0fbe74397cb858f3457bc",
    "registry/post.json": "53962867ce1614f937a16b50dddf736045271d7d6f44d4253d989ac571d63508"
  }
}
```

That folder is a campaign bundle: everything the run was configured with, as text.

## What you just saw

**A bundle is a run's configuration written out as files.** `pipeline.yaml` is the config that run stored, rebuilt as YAML. `registry/` is the whole [canonical field](/concepts/canonical-fields) vocabulary, one file per kind of record, packed for review. Every bundle gets all three files, so `post.json` is there though this pipeline has no posts. Nothing in the folder executes.

This pipeline uses only adapters built into the binary, so there's nothing else to pack. An installed [binding](/concepts/adapter-tiers), a vendor adapter written as YAML, lands in `adapters/` with its fixtures (recorded responses) and the version it was installed at. While the bundle runs, its copies win over whatever the machine has installed. A template kept in a file (`template: {file: ...}`) travels too. A process adapter runs as its own program, so it doesn't travel, and the machine needs the same one.

**The manifest is a list of hashes.** `bundle_format_version` versions the folder layout, and `name` is the pipeline's. `source_run_id` is the run you froze, the id its [receipt](/concepts/runs-and-receipts) carries. `created_at` and `gtm_version` record when and with which `gtme` binary. `contents` lists every other file with its SHA-256 hash, a fingerprint that changes if one byte does.

**Your CSV isn't in it.** The freeze line said so, because the input is yours to replace. Copy it in, point `GTME_LEDGER` at a fresh, empty [ledger](/concepts/ledger), and run the folder:

```sh
cp contacts.csv hello-bundle/
cd hello-bundle
export GTME_LEDGER=$(mktemp -d)/ledger.db
gtme run . --simulate
```

The output is similar to the following:

```
bundle hello (frozen from run 01M3FBDQ11PD7C2NFS42DN24G7) — hashes verified
simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
...
run 01M3FBDX5990D9H3RK0RKJB5WW — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
...
```

`gtme run` takes the folder wherever it takes a pipeline file, and checks every hash first: the `hashes verified` line. Under [simulate](/concepts/gate-ladder), each binding answers from the fixtures packed inside the bundle, so the run needs no network and no key. The rest is the receipt `hello.yaml` prints on its own.

That folder and that last block are what you'd hand your agent. It can check the whole campaign offline before a person reads it.

## So what?

**Edit a bundle and it won't run.** Raise the threshold in the frozen file:

```sh
perl -pi -e 's/>= 70/>= 80/' pipeline.yaml
```

```sh
gtme run . --simulate
```

The output is the following:

```
gtme: bundle: pipeline.yaml does not match its manifest hash — the bundle has been modified since it was frozen
```

The run stops before it reads a record. To change a bundle, copy `pipeline.yaml` out, edit and run the copy, then freeze that run into a new folder. Your own CSV under the same file name still verifies, because the manifest lists only frozen files.

`gtme plan` doesn't accept a bundle folder yet, and only `gtme run` does. Check a bundle with `gtme run . --simulate` from inside its folder, then `--dry-run` on the next rung.

A [group](/concepts/groups) named in a bundled pipeline resolves against the ledger the bundle runs on. A bundle that reads its people from another campaign's group stops on a fresh ledger until that campaign has run there.

That's it. A campaign is a folder you can diff, commit, and run anywhere the binary is.

## Why it's this way

**Bindings are data, so everything a campaign needs except keys and inputs is text.** The decision record, [ADR-029](/decisions#adr-029), made freeze output the unit you share. It's text in a stable order, so it diffs, and the same bundle runs on any machine or ledger. SPEC.md [§8](/spec#8-cli-surface--decided) adds that simulate works from the bundle's own fixtures.

**The configuration travels, and state stays with the machine.** Your teammate adds their own keys with `gtme secret set`. Then they climb to armed, the rung that spends and sends, as [Connect your stack](/guides/connect-your-stack) shows.

**What it costs.** Freeze rebuilds the YAML from the run's stored config, so the comments in `hello.yaml` are gone from the frozen copy. The shipped bundles keep theirs in a README the manifest doesn't list, so editing it doesn't break a hash. A bundle can't carry the people it selected either, because a list of people is ledger state and belongs to whoever ran the campaign.

## Where it shows up

The gtme repository ships five pattern bundles in its `bundles/` folder, and each one simulates offline with no key:

| Pattern | What it does |
|---|---|
| `qualify-group-send` | Judges people into a group, then a second bundle writes to the group and sends |
| `email-waterfall` | Tries a second email finder where the first found nothing, then verifies |
| `account-shape` | Four bundles: companies, people gated by company, a rollup per account, then outreach |
| `events-cron` | Delivers from a CSV that something else appends to, on a schedule |
| `posts-to-engagers` | Goes from people to their posts to who reacted, then judges the reactors |

- [Package and share a campaign](/guides/bundles) takes a campaign of your own from a run to a shared folder.
- [Bundles](/reference/bundles) and [`gtme freeze`](/reference/cli/freeze) are the lookup pages.
