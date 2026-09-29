---
name: "Connect your stack: secrets, plan, dry-run, arm"
description: Take a pipeline file that names your vendors, install what it needs, store each key, and climb from plan to an armed run without a surprise
for: "You have a pipeline file that names your vendors, and you want it running on your own keys with nothing spent or sent until you've read what it will do."
learn:
  - "how plan's errors become the checklist of adapters to install and keys to store"
  - "where a stored key lives and which copy wins when there are two"
  - "what to read at simulate and dry-run before you arm"
order: 2
roles: [operator, builder]
links:
  - to: /concepts/gate-ladder
    type: depends-on
    description: Each step after the keys are stored is one rung of the ladder, taken in order
  - to: /concepts/adapter-tiers
    type: depends-on
    description: A vendor the binary doesn't ship is a binding installed from the registry, pinned and verified
  - to: /start/my-stack
    type: relates-to
    description: The same climb on a fixed Apollo-to-Instantly pipeline, with a real dry-run and armed receipt
  - to: /start/for-agents
    type: relates-to
    description: The rules an agent follows on this procedure, including that you type every key
  - to: /reference/cli/secret
    type: relates-to
    description: How gtme secret set stores a key without echoing it
  - to: /reference/cli/adapters
    type: relates-to
    description: The search, add, verify, and update verbs
  - to: /concepts/pipeline
    type: relates-to
    description: The file this guide connects; nothing in it changes between rungs except limit and campaign
  - to: /concepts/ledger
    type: relates-to
    description: What the dry-run paid for lands here, so the armed run reads it instead of paying again
  - to: /start/add-a-vendor
    type: relates-to
    description: Writes a binding when the registry has no entry for your vendor
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The dry-run receipt is what you read before arming
  - to: /concepts/groups
    type: relates-to
    description: Qualifying into a group pins the list, so the armed run sends only to people the dry-run showed
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A connected pipeline freezes into a bundle that carries each binding at its pin
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH
  - to: /spec#6-adapter-manifest--decided
    type: decided-by
    description: Credentials resolve from the environment first, then the secrets file, and a missing one is a plan error
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The secret and adapters verbs, and exit code 3 for a credential problem
  - to: /decisions#adr-042
    type: decided-by
    description: Bindings install from the registry pinned to a commit, verified before they install
  - to: /decisions#adr-040
    type: decided-by
    description: Preflight reads the delivery target before any record moves; the dry-run reports it and the armed run stops on a blocked check
---

# Connect your stack: secrets, plan, dry-run, arm

**Goal: your pipeline runs armed on your own vendor keys, after every cheaper rung of the [gate ladder](/concepts/gate-ladder) comes back clean.** The rungs are plan, simulate, dry-run, and the armed run, the one with no flag that spends and sends.

## Before you start

**You need `gtme` ([Install](/start/install)) and one key per vendor the file names.** Plan names each key except the model's. For the file in the next section, that's a HubSpot private-app token that can read contacts, an Instantly API key, and an Anthropic API key. `gtme secret set` keeps them in `~/.gtme/secrets`, whatever `GTME_LEDGER` points at.

**You need an Instantly campaign, active, whose emails use the two lines this file writes.** Put `{{first_line}}` and `{{ps_line}}` in every email of its sequence, A/B variants included. An active campaign emails anyone added to it on its schedule, and pausing it in Instantly stops its emails.

**Steps 1 through 7 spend nothing and send nothing.** Step 8, the dry-run, spends model tokens on five records and sends nothing. Step 9, the armed run, spends only on what the [ledger](/concepts/ledger) doesn't already have, and it sends. This page shows real output for steps 1 through 7. Steps 8 and 9 call HubSpot and Instantly for real, so for their output it points to [Your stack](/start/my-stack), which ran the same `send` step.

## The file

This [pipeline](/concepts/pipeline) sources contacts from HubSpot, writes two lines for each with a model, and adds them to an Instantly campaign. Save it as `hubspot-to-instantly.yaml`:

```yaml
name: hubspot-to-instantly
version: 1

source:
  use: hubspot/contact-search
  with:
    known_property: email
    limit: 5

steps:
  - id: lines
    use: ai/compose
    uses: [first_name, title, company_name]
    with:
      template: >
        Write first_line and ps_line using title and company_name.
        first_line is one specific sentence; ps_line is one short,
        low-pressure sentence. No flattery, no exclamation marks.

  - id: send
    use: instantly/add-to-campaign
    with:
      campaign: 0198a0b1-7e6d-4c5b-9a8f-1e2d3c4b5a69   # the campaign id, from its URL in Instantly
    variables:
      first_line: first_line
      ps_line: ps_line
    idempotency: email
```

`hubspot/contact-search` takes contacts that have `known_property` set, oldest HubSpot record first, and `limit: 5` stops at five. Those can be customers. To send to prospects only, add a [filter](/concepts/steps-and-roles) step on `hubspot.lifecycle_stage`, which the source provides, before you arm.

## Steps

1. Plan the file before you install anything. Plan reads the file and the adapters you have installed, with no network:

   ```sh
   gtme plan hubspot-to-instantly.yaml
   ```

   The output is similar to the following:

   ```
   gtme: 5 plan problems:
     - step "source": adapters: unknown adapter "hubspot/contact-search" — if it is a registry entry, install it: gtme adapters add hubspot/contact-search
     built-in: agent/compose, agent/filter, agent/review, ai/compose, ai/filter, ai/review, csv/deliver, csv/source, demo/enrich, http/deliver, http/enrich, human/compose, human/filter, human/review, text/compose
   ...
     - step "lines": needs first_name, title, company_name, which no earlier step provides (available: nothing)
     - step "send": adapters: unknown adapter "instantly/add-to-campaign" — if it is a registry entry, install it: gtme adapters add instantly/add-to-campaign
     - step "send": needs email, which no earlier step provides (available: first_line, ps_line)
   ```

   The problem is in the file itself, not a missing key. The `built-in` line is every [adapter](/concepts/adapter-tiers) this binary ships, and no vendor is on it. Neither `hubspot/contact-search` nor `instantly/add-to-campaign` is one, so plan prints the command that installs each from the registry. The two `needs` lines are fallout from it: with no source, no step downstream gets a field. Fix unknown adapters first.

1. Find the missing adapter in the registry. Searching reads GitHub and spends nothing:

   ```sh
   gtme adapters search hubspot
   ```

   The output is similar to the following:

   ```
   ID                      ROLE    TIER      INSTALL                                                                          DESCRIPTION
   hubspot/contact-search  source  verified  gtme adapters add github.com/gtme-run/gtme-bindings/hubspot-contact-search@main  Source HubSpot contacts via the CRM v3 Search API, filtered…
   ```

   Install both by ID, the commands plan printed:

   ```sh
   gtme adapters add hubspot/contact-search instantly/add-to-campaign
   ```

   The output is similar to the following:

   ```
   fetched github.com/gtme-run/gtme-bindings/hubspot-contact-search@bcf671b9e1df177ed24e98b58e0c132176261c78 at bcf671b9e1df
   hubspot/contact-search v1 — source (person)
     calls:       api.hubapi.com
     demands:     HUBSPOT_ACCESS_TOKEN
     needs:       none
     provides:    company_name, email, first_name, hubspot.contact_id, hubspot.lifecycle_stage, last_name, title
     fixtures:    ok — 1 response(s) on file, 2 record(s) extracted
   ...
   ```

   `hubspot/contact-search` is a binding, a vendor adapter written as YAML. `instantly/add-to-campaign` prints the same surface as a process adapter, built for your platform and pinned to a gtme release (the decision record, [ADR-063](/decisions#adr-063)). `calls:` lists every host it reaches, and `demands:` is the key it needs. The install is pinned to commit `bcf671b9e1df`, the one the registry's index names (the decision record, [ADR-042](/decisions#adr-042)). The `INSTALL` column's longer reference also works, and pins whatever commit `main` points to. If search finds nothing, [Add a vendor](/start/add-a-vendor) shows how to write the binding.

1. Plan again until only keys are left:

   ```sh
   gtme plan hubspot-to-instantly.yaml
   ```

   It prints:

   ```
   gtme: 2 plan problems:
     - step "source": missing credential HUBSPOT_ACCESS_TOKEN (set it in the environment or run `gtme secret set HUBSPOT_ACCESS_TOKEN`)
     - step "send": missing credential INSTANTLY_API_KEY (set it in the environment or run `gtme secret set INSTANTLY_API_KEY`)
   ```

   Exit code `3` means every remaining problem is a missing credential. This list is your key checklist.

1. Store each key. Each command prompts for the value, doesn't echo it, and saves it to `~/.gtme/secrets`, a file only your user can read:

   ```sh
   gtme secret set HUBSPOT_ACCESS_TOKEN
   gtme secret set INSTANTLY_API_KEY
   gtme secret set ANTHROPIC_API_KEY
   ```

   Plan didn't list `ANTHROPIC_API_KEY`, because the AI adapters mark it optional and plan only warns. Without it, `lines` fails at run time, so store it now.

   The runner looks in your environment first and the secrets file second ([SPEC §6](/spec#6-adapter-manifest--decided)). A key exported in your shell wins over the stored one. To rotate a key, run `gtme secret set` again, which replaces the line, and unset any exported copy.

1. Check what's stored. This prints names, never values:

   ```sh
   gtme secret list
   ```

   It prints:

   ```
   ANTHROPIC_API_KEY
   HUBSPOT_ACCESS_TOKEN
   INSTANTLY_API_KEY
   ```

1. Plan until it's clean:

   ```sh
   gtme plan hubspot-to-instantly.yaml
   ```

   The output is similar to the following:

   ```
   pipeline hubspot-to-instantly (version 1)

   1. source [source] — hubspot/contact-search@1
   ...
        creds:     HUBSPOT_ACCESS_TOKEN (resolved)
        est/record: $0.0000

   2. lines [compose] — ai/compose@1
   ...
        warning:   optional credential ANTHROPIC_WORKSPACE_ID is not set; this step will fail at run time if it needs it
        est/record: ?

   3. send [deliver] — instantly/add-to-campaign@2
   ...
        creds:     INSTANTLY_API_KEY (resolved)
        est/record: $0.0000

   send surface: 1 deliver step(s)
     send → instantly/add-to-campaign (touch scope: hubspot-to-instantly)
   ...
   plan ok — nothing has been spent
   ```

   Every `creds:` line says `resolved`. Ignore the `ANTHROPIC_WORKSPACE_ID` warning unless a run fails on it. `est/record: ?` means the model is metered, so the dry-run receipt is where you see its cost. `send surface` lists every step that sends when you arm.

1. Simulate it. Simulate runs offline from each adapter's recorded responses:

   ```sh
   gtme run hubspot-to-instantly.yaml --simulate
   ```

   The output is similar to the following:

   ```
   simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
   ...
   run 01M3K16V5G360T18EQBN9FDYPW — done (SIMULATED — recorded responses only; nothing sent, nothing persisted)
   step    adapter                    in  out  empty  cached  filtered  failed  cost  avoided
   source  hubspot/contact-search     0   2    -      0       -         -       $0    -
   lines   ai/compose                 2   2    -      0       -         -       $0    -
   send    instantly/add-to-campaign  2   0    -      0       -         -       $0    -
   send: preflight skipped — the target is not read under --simulate; --dry-run checks it
   send: resolved variables for 2 record(s) — review, then run again without --dry-run to arm:
     ada.quill@example.test
       first_line: "Fixture first line for ada.quill@example.test"
       ps_line: "Fixture ps line for ada.quill@example.test"
   ...
   total: $0 (estimated) spent
   ```

   Read two things. There's no `simulation gap:` line, so every adapter had recorded responses to serve; a gap line names an adapter simulate couldn't vouch for. And `send: resolved variables` lists the fields the `send` step hands the campaign, here `first_line` and `ps_line`. The last line's "run again without --dry-run to arm" is dry-run's wording. After simulate, the dry-run is next, not the armed run.

1. Dry-run it. In `hubspot-to-instantly.yaml`, keep `limit: 5` and set `campaign:` to your campaign's id, from its URL in Instantly. The dry-run spends model tokens and sends nothing:

   ```sh
   gtme run hubspot-to-instantly.yaml --dry-run
   ```

   Before any record moves, preflight reads the campaign, prints its name, and reports whether it's active and every email uses both variables (the decision record, [ADR-040](/decisions#adr-040)). For a campaign named Q4 CRM follow-up, the receipt's line reads:

   ```
   send: preflight ok — campaign "Q4 CRM follow-up" (0198a0b1-7e6d-4c5b-9a8f-1e2d3c4b5a69) — 3 check(s) (✓ campaign active, ✓ variable first_line referenced, ✓ variable ps_line referenced)
   ```

   A failed check names the fix: activate the campaign, or add the missing variable to every email. Then read every `first_line` and `ps_line` in the resolved variables, the exact text the armed run sends.

1. Arm it. With `limit: 5` still set, this adds up to five people, and Instantly emails them on the campaign's schedule. A blocked preflight stops the step here, before anyone is added:

   ```sh
   gtme run hubspot-to-instantly.yaml
   ```

   On the receipt, `lines` shows the dry-run's lines as `cached`, so they aren't paid for twice, and `send` shows how many people it added. The source searches HubSpot again, so a contact added after the dry-run is composed and sent without your read. [Groups](/concepts/groups) pin the exact list you dry-ran. Running the file again adds no one twice, because `idempotency: email` skips anyone already added.

## What you have now

**A pipeline on your own keys that has sent once, with its adapter pinned.** `gtme runs` lists the armed run as `done`, not `done (dry)`, and the people appear in the campaign's leads in Instantly. Check the install and its pin:

```sh
gtme adapters
```

The output is the following:

```
ID                      VERSION  ROLE    KIND     SOURCE
hubspot/contact-search  1        source  binding  github.com/gtme-run/gtme-bindings/hubspot-contact-search@bcf671b9e1df177ed24e98b58e0c132176261c78 (bcf671b9e1df)
```

The hash in parentheses is the pin, the short form of the commit in `SOURCE`. `gtme secret list` and `gtme plan` recheck the keys. To have Claude Code do steps 1 through 7 and stop there, paste this line. The agent leaves each `gtme secret set` for you to run ([For agents](/start/for-agents)):

```text
Connect hubspot-to-instantly.yaml: plan it, install what plan names, list the keys for me to set, then simulate and stop before the dry-run
```

## Next

- [Runs and receipts](/concepts/runs-and-receipts) reads every column of the dry-run receipt.
- [`gtme adapters`](/reference/cli/adapters) verifies an installed binding and moves its pin with `update`.
- [A campaign is a folder](/concepts/campaign-is-a-folder) packages the connected pipeline with its pins to share or rerun.
