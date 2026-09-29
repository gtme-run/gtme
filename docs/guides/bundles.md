---
name: Package and share a campaign
description: Freeze a finished run into a bundle folder, hand it to a teammate or an agent, run it on their ledger, and freeze a new version after a change
for: "You have a campaign that ran, and you want a teammate, an agent, or you next quarter to run exactly that campaign again."
learn:
  - "how to freeze a run into a bundle and what to put beside it"
  - "what the receiving side runs, and what it pays for again"
  - "what happens when someone edits a frozen file"
  - "how to ship a change as a new bundle version"
order: 15
roles: [builder]
links:
  - to: /concepts/campaign-is-a-folder
    type: depends-on
    description: What a bundle contains and what its manifest records; this guide is the procedure
  - to: /start/install
    type: depends-on
    description: Puts the gtme binary on your PATH, on both machines
  - to: /concepts/gate-ladder
    type: relates-to
    description: The receiving side checks a bundle with simulate before it arms it
  - to: /concepts/ledger
    type: relates-to
    description: Cache, deliveries, and groups live in the ledger, so they stay behind when the bundle travels
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Bindings travel in a bundle with their fixtures; process adapters don't
  - to: /concepts/groups
    type: relates-to
    description: A group a bundled pipeline reads resolves against the receiving ledger
  - to: /reference/cli/secret
    type: relates-to
    description: Stores a key on the receiving machine, since keys never travel in a bundle
  - to: /concepts/pipeline
    type: relates-to
    description: A bundle's pipeline.yaml is the frozen form of the file this guide starts from
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: Idempotency is checked against the receiving ledger's deliveries, so a bundle run elsewhere can write to the same people again
  - to: /decisions#adr-042
    type: decided-by
    description: A registry-installed binding records its pinned commit, and the bundle carries that record
  - to: /start/my-stack
    type: relates-to
    description: Where the receiving side stores its own keys with gtme secret set
  - to: /reference/cli/freeze
    type: relates-to
    description: Every form and flag of the command that writes a bundle
  - to: /reference/cli/run
    type: relates-to
    description: The command that verifies a bundle's hashes and runs it
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: The normative bundle contents and the rule that gtme run accepts a bundle path anywhere it accepts a pipeline file
  - to: /decisions#adr-029
    type: decided-by
    description: Freeze output is a self-contained, diffable, portable folder that simulates offline
  - to: /decisions#adr-057
    type: decided-by
    description: A template kept in a file travels in the bundle under templates/
---

# Package and share a campaign

**Goal: a campaign frozen into a folder that anyone with `gtme` can run, plus a second version after you change it.** A [bundle](/concepts/campaign-is-a-folder) is that folder: the campaign's files, and a manifest that records a fingerprint, a hash, of each one, which changes if a single byte does.

## Before you start

**You need `gtme` on both sides.** [Install](/start/install) covers it. Built-in adapters ship inside the binary, not the bundle, so both sides should run the same release; `gtme version` prints it.

**This guide needs no keys and sends nothing past your own disk.** The pipeline's only priced step is `demo/enrich`, a pretend $0.01 per record that calls no vendor, and its output goes to a local `out.csv`. The first [armed](/concepts/gate-ladder) run of three people reports $0.03.

## Steps

1. Make a campaign folder with its own [ledger](/concepts/ledger), and fetch a practice CSV:

    ```sh
    mkdir -p renewals && cd renewals
    export GTME_LEDGER=$PWD/ledger.db
    curl -fsSLO https://raw.githubusercontent.com/gtme-run/gtme/main/examples/contacts.csv
    ```

    Save the opening line as `opener.txt`:

    ```text
    {{ record.full_name }}, your renewal with {{ record.company_domain }} is coming up.
    ```

    Save the [pipeline](/concepts/pipeline) as `renewal-openers.yaml`:

    ```yaml
    name: renewal-openers
    version: 1

    source:
      use: csv/source
      with:
        path: contacts.csv
        columns: { full_name: Full Name, email: Email, title: Title, company_domain: Company Website }

    steps:
      - id: score
        use: demo/enrich
        with:
          cost_per_record_usd: 0.01

      - id: keep
        use: sql/filter
        with:
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'demo.score' AND CAST(value AS INTEGER) >= 70

      - id: opener
        use: text/compose
        uses: [full_name, company_domain]
        provides: [opener]
        with:
          template: { file: opener.txt }

      - id: out
        use: csv/deliver
        with:
          path: out.csv
        variables:
          opener: renewal-openers.opener
          score: demo.score
        idempotency: email
    ```

    `score` rates each person, `keep` passes scores of 70 or more, `opener` fills the template, and `out` writes a row per person.

1. Run it. A bundle is frozen from a run, so there has to be one:

    ```sh
    gtme run renewal-openers.yaml
    ```

    The output is similar to the following:

    ```
    run 01M3MGMBM85JE40V4K9S9DNDFH (renewal-openers)
    ...
    run 01M3MGMBM85JE40V4K9S9DNDFH — done
    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   3    -      0       -         -       $0       -
    score   demo/enrich   3   3    -      0       -         -       $0.0300  -
    keep    sql/filter    3   1    -      0       2         -       $0       -
    opener  text/compose  1   1    -      0       -         -       $0       -
    out     csv/deliver   1   1    -      0       -         -       $0       -
    total: $0.0300 (estimated) spent
    ```

1. Freeze that run into a new folder. `last` names your most recent run:

    ```sh
    gtme freeze last --bundle renewal-openers-v1
    ```

    The output is similar to the following:

    ```
    froze run 01M3MGMBM85JE40V4K9S9DNDFH into bundle renewal-openers-v1 (5 steps) — self-contained except credentials and input files
    ```

1. Read what landed:

    ```sh
    find renewal-openers-v1 -type f | sort
    ```

    ```
    renewal-openers-v1/manifest.json
    renewal-openers-v1/pipeline.yaml
    renewal-openers-v1/registry/company.json
    renewal-openers-v1/registry/person.json
    renewal-openers-v1/registry/post.json
    renewal-openers-v1/templates/opener.txt
    ```

    `manifest.json` lists every other file with its hash, the run it was frozen from, and the release that froze it:

    ```sh
    grep -E "source_run_id|gtm_version" renewal-openers-v1/manifest.json
    ```

    ```
      "source_run_id": "01M3MGMBM85JE40V4K9S9DNDFH",
      "gtm_version": "0.0.0-dev",
    ```

    `0.0.0-dev` is a local build; a released binary records its version number. Your template traveled as `templates/opener.txt`, and the frozen `opener` step now points there:

    ```sh
    grep -A4 "id: opener" renewal-openers-v1/pipeline.yaml
    ```

    ```
      - id: opener
        use: text/compose
        with:
          template:
            file: templates/opener.txt
    ```

    `source_run_id` is the run from step 2. A pipeline that calls a vendor through a [binding](/concepts/adapter-tiers) also gets an `adapters/` folder, one per binding, with its fixtures, the recorded vendor responses simulate replays. A binding you installed with `gtme adapters add` also carries `.source.json`, the commit it's pinned at ([ADR-042](/decisions#adr-042)).

1. Save the run's receipt beside the frozen files, so the person you hand it to knows what a good run looks like. `gtme` prints the receipt to a separate output stream, so `2>` saves it to a file:

    ```sh
    gtme runs last 2> renewal-openers-v1/receipt.txt
    ```

    ```sh
    head -3 renewal-openers-v1/receipt.txt
    ```

    ```
    run 01M3MGMBM85JE40V4K9S9DNDFH
    pipeline: renewal-openers
    status:   done
    ```

    The manifest lists only frozen files, so `receipt.txt` doesn't change any hash. The shipped bundles keep a README there too.

1. Add the input and zip the folder:

    ```sh
    cp contacts.csv renewal-openers-v1/
    zip -r renewal-openers-v1.zip renewal-openers-v1
    ```

    Leave the CSV out when the teammate brings their own list. Tell them what their side brings:

    - The same gtme release, which the manifest's `gtm_version` records.
    - Their list, saved in the bundle folder as `contacts.csv` with the headers the pipeline maps: `Full Name`, `Email`, `Title`, and `Company Website`. Renaming their columns is fine; editing `pipeline.yaml` breaks its hash.
    - Any keys the bundle's vendors need, which simulate lists.
    - Any [group](/concepts/groups) the bundle reads, filled by running the campaign that writes it on their ledger first.

    We recommend committing bundles to a git repository, one folder per version, so every change to a campaign is a diff a teammate can read. A zip is fine for a one-off handoff.

1. On the receiving side, unzip it next to a fresh ledger and simulate it from inside the folder. To play the teammate on one machine, use a sibling folder:

    ```sh
    mkdir -p ../teammate && cd ../teammate
    export GTME_LEDGER=$PWD/ledger.db
    unzip -q ../renewals/renewal-openers-v1.zip && cd renewal-openers-v1
    gtme run . --simulate
    ```

    The output is similar to the following:

    ```
    bundle renewal-openers (frozen from run 01M3MGMBM85JE40V4K9S9DNDFH) — hashes verified
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    ...
    out: resolved variables for 1 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        opener: "Jane Doe, your renewal with acme.com is coming up."
        score: "100"
    total: $0.0300 (estimated) spent
    ```

    On a bundle that needs keys, simulate lists each missing one, and the receiving side stores them with `gtme secret set`, as [Your stack](/start/my-stack) shows. A bundle that reads a group stops here with `group "NAME" does not exist` until the campaign that fills it has run on this ledger. `gtme run` checked every hash in the manifest before it read a record ([SPEC §8](/spec#8-cli-surface--decided)), and simulate is the first check because `gtme plan` doesn't take a folder yet. Its total is what an armed run would cost, and nothing was charged; the `--dry-run` in its hint is dry-run's wording.

1. Arm it. On the teammate's fresh ledger, this spends the pretend $0.03 again and writes Jane to `out.csv` again. With a vendor or a live target, run `gtme run . --dry-run` first and read its resolved variables; this bundle only writes a local file:

    ```sh
    gtme run .
    ```

    The output is similar to the following:

    ```
    bundle renewal-openers (frozen from run 01M3MGMBM85JE40V4K9S9DNDFH) — hashes verified
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost     avoided
    source  csv/source    0   3    -      0       -         -       $0       -
    score   demo/enrich   3   3    -      0       -         -       $0.0300  -
    keep    sql/filter    3   1    -      0       2         -       $0       -
    opener  text/compose  1   1    -      0       -         -       $0       -
    out     csv/deliver   1   1    -      0       -         -       $0       -
    total: $0.0300 (estimated) spent
    ```

    The rows match step 2's receipt exactly.

    `score` shows `cached 0` because the cache lives in each ledger, and the teammate's is new. `out` delivered Jane again, because [idempotency](/concepts/runs-and-receipts) is checked against the teammate's deliveries, and there are none. Run on your own ledger in `renewals/`, the same bundle skips all three scores and writes nothing. So a bundle carries the campaign, and each side's contact history stays with that side. Hand over a new list, or agree on who sends.

1. Edit a frozen file and run it again:

    ```sh
    perl -pi -e 's/is coming up/renews next month/' templates/opener.txt
    gtme run . --simulate
    ```

    The output is the following:

    ```
    gtme: bundle: templates/opener.txt does not match its manifest hash — the bundle has been modified since it was frozen
    ```

    The run stops before it reads a record and exits `2`. Unzip the original again to undo the edit. To change a campaign, change its source files and freeze again, as the next step does.

1. Make the change on purpose, back in your own folder. Edit the source files, not the bundle, and run them:

    ```sh
    cd ../../renewals
    export GTME_LEDGER=$PWD/ledger.db
    perl -pi -e 's/is coming up/renews next month/' opener.txt
    gtme run renewal-openers.yaml
    ```

    The output is similar to the following:

    ```
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   3    -      0       -         -       $0    -
    score   demo/enrich   3   0    -      3       -         -       $0    $0.0300
    keep    sql/filter    3   1    -      0       2         -       $0    -
    opener  text/compose  1   1    -      0       -         -       $0    -
    out     csv/deliver   1   0    -      0       -         -       $0    -
    out: 1 already delivered
    total: $0 spent, $0.0300 avoided via cache (3 records skipped)
    ```

    `opener` wrote Jane a new line, `score` came from the cache, and `out` wrote nothing: `out: 1 already delivered` is Jane, who already has a delivery on this ledger. That's the safety working: v2 reaches people v1 hasn't, which is the normal case for a new list.

1. Freeze the new run into its own folder, and compare the two versions:

    ```sh
    gtme freeze last --bundle renewal-openers-v2
    diff -r renewal-openers-v1 renewal-openers-v2
    ```

    The output is similar to the following:

    ```
    froze run 01M3MGQ75E6V3X11F5KZSWHTAE into bundle renewal-openers-v2 (5 steps) — self-contained except credentials and input files
    Only in renewal-openers-v1: contacts.csv
    diff -r renewal-openers-v1/manifest.json renewal-openers-v2/manifest.json
    4,5c4,5
    <   "source_run_id": "01M3MGMBM85JE40V4K9S9DNDFH",
    <   "created_at": "2026-09-28T17:22:37.243Z",
    ---
    >   "source_run_id": "01M3MGQ75E6V3X11F5KZSWHTAE",
    >   "created_at": "2026-09-28T17:24:11.471Z",
    12c12
    <     "templates/opener.txt": "b3e9401b17ad3e1624b26237ae221cf8fa6e9591d51c82552f59848e3eed5680"
    ---
    >     "templates/opener.txt": "648e7dfe176402cb9a1128f8b362c894b1ce28bbcbf33c3e10f7eee8b22a915d"
    Only in renewal-openers-v1: receipt.txt
    diff -r renewal-openers-v1/templates/opener.txt renewal-openers-v2/templates/opener.txt
    1c1
    < {{ record.full_name }}, your renewal with {{ record.company_domain }} is coming up.
    ---
    > {{ record.full_name }}, your renewal with {{ record.company_domain }} renews next month.
    ```

    The change is one template line and its hash. `gtme freeze --bundle` wants an empty folder and refuses `renewal-openers-v1` as `is not empty`, so every version gets its own.

## What you have now

**Two bundle versions of one campaign, each tied to the run it came from.** To check one, put `contacts.csv` in its folder and run `gtme run . --simulate` there. The first line should end in `hashes verified`.

Your ledger stays with you, and so do the cache, the delivery history, and group membership. Keys and any process adapter, a vendor adapter that runs as its own program, stay with each machine. The hashes catch an accidental edit. Anyone can recompute one after a deliberate edit, so read a bundle's diff before you arm it, as you'd read a change to code.

**To run exactly this campaign next quarter, run the bundle on the same ledger.** People already delivered are skipped, and scores inside their [cache window](/concepts/facts) are reused, so only new people cost anything. On a fresh ledger, it pays for everyone again and sends to everyone again.

To have Claude Code check a bundle someone handed you, paste this line:

```text
Check the bundle in BUNDLE_DIR: run gtme run . --simulate from inside it on a fresh GTME_LEDGER, show me receipt.txt if there is one, and list any missing keys or groups. Don't arm it.
```

Replace `BUNDLE_DIR` with the path to the bundle folder.

## Next

- [`gtme freeze`](/reference/cli/freeze) lists every form and flag of the command that writes a bundle.
- [`gtme run`](/reference/cli/run) lists `--simulate` and `--dry-run`, which both work on a bundle folder.
- [`gtme secret`](/reference/cli/secret) stores the keys a vendor bundle needs on the receiving machine.
