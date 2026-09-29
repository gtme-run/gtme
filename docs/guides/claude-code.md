---
name: Use gtme from Claude Code
description: Install the gtme plugin in Claude Code, describe a goal in plain words, and take the pipeline it writes from plan to an armed run
for: "You use Claude Code and want it to build and run a gtme pipeline for you, and you want to know which parts stay yours."
learn:
  - "how to add the gtme plugin and which skill fires when"
  - "what a session looks like from your goal to an armed run"
  - "what you say to Claude Code, and where you step in"
order: 16
roles: [operator, builder]
links:
  - to: /concepts/identity-keys
    type: relates-to
    description: Why the first column of for-sdr.csv is each person's email
  - to: /start/for-agents
    type: depends-on
    description: The paste line, the plugin's four skills, and the rules the agent follows before anything spends or sends
  - to: /start/install
    type: depends-on
    description: Installs the gtme binary the plugin's skills call
  - to: /concepts/pipeline
    type: relates-to
    description: The YAML file the create-pipeline skill writes
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan, simulate, dry-run, and armed, the rungs the run-pipeline skill climbs in order
  - to: /concepts/steps-and-roles
    type: relates-to
    description: The filter, compose, and deliver roles of the three steps in this guide's pipeline
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: Every rung ends in a receipt, which is what the agent reads back to you
  - to: /concepts/ledger
    type: relates-to
    description: Why the armed run reused the lines the dry-run wrote, and why a re-run delivers nothing twice
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: The example bundles the create-pipeline skill copies a starting shape from
  - to: /reference/cli/secret
    type: relates-to
    description: The prompt where you enter a key after plan names it
  - to: /reference/adapters/text-compose
    type: relates-to
    description: The keyless compose adapter this guide's pipeline uses for its opener line
  - to: /start/my-csv
    type: relates-to
    description: The same session with a model key, where a model does the judging and the writing
  - to: /start/my-stack
    type: relates-to
    description: A pipeline that delivers to a vendor, where the arm gate is yours
  - to: /decisions#adr-007
    type: decided-by
    description: gtme help --agent prints the whole surface the skills read their facts from
  - to: /decisions#adr-050
    type: decided-by
    description: Claude Code drives gtme and gtme never launches Claude Code
  - to: /spec#dry-run-and-the-armed-gate-adr-019
    type: decided-by
    description: A dry-run resolves every delivery and sends none, and it is the artifact you read before arming
---

# Use gtme from Claude Code

**Goal: you give Claude Code this request, and you end with `for-sdr.csv`, one opening line per senior signup.** Claude Code turns it into a [pipeline](/concepts/pipeline) file and runs it:

```text
Every Monday I export trial signups to signups.csv. Keep the VPs, heads,
and directors, write each one a line our SDR can open with, and put
them in a CSV for the SDR.
```

## Before you start

**You need Claude Code, a terminal, and gtme installed.** [For agents](/start/for-agents) has the rules Claude Code follows. This guide is what the session looks like from your side of the chat.

This guide needs no keys and spends $0. The pipeline reads a CSV and writes a CSV, with no vendor and no model. The only thing it sends is a file on your machine, `for-sdr.csv`.

You do steps 1 to 3 yourself. From step 4 on, Claude Code runs every command shown and reads the output back to you; the blocks show what it ran. We describe what Claude Code says in plain words, because its wording changes from run to run. Every `gtme` output on the page is real.

**Where you step in:**

- Say yes to the plan. The skill reads its plan back before it builds anything.
- Read the dry-run. It shows exactly what would be written or sent. On a pipeline with a vendor or a model step, the dry-run is also the first command that costs money, and the agent tells you what it will spend before it runs it.
- Say send. On a vendor target, such as an Instantly campaign, nothing arms until you reply to "Send it?" after reading the dry-run.
- Enter keys yourself. When plan names a missing key, run `gtme secret set KEY_NAME` in your own terminal and paste the key there, never in the chat.

These are the agent's rules plus your read; gtme itself can't tell who ran a command.

## Steps

1. **Install gtme.** Follow [Install](/start/install), then check it:

    ```sh
    gtme version
    ```

    The output is similar to the following:

    ```text
    gtme v0.6.1
    ```

    If `gtme` isn't found, finish Install before going on, because every skill calls it.

2. **Add the plugin.** In Claude Code, enter these two lines:

    ```text
    /plugin marketplace add gtme-run/gtme
    /plugin install gtme@gtme-run
    ```

    Claude Code now has four skills, and it picks one from what you say, so you rarely call one by name:

    | You say something like | Skill that fires |
    |---|---|
    | "Find the senior people in this signup list" | `/gtme:create-pipeline` |
    | "Run it," "does this work," "send it" | `/gtme:run-pipeline` |
    | "gtme doesn't have Hunter" | `/gtme:create-adapter` |
    | "What did last week's run cost?" | `/gtme:analyze` |

    The skills read what gtme can do from the gtme you installed, so they match your version ([ADR-007](/decisions#adr-007), the decision record behind that).

3. **Say what you want, in your words.** Save this as `signups.csv` in an empty folder, and start Claude Code in that folder:

    ```csv
    First Name,Email,Job Title,Company Website
    Jane,jane.doe@acme.com,VP Marketing,acme.com
    Bob,bob@globex.io,Head of Growth,globex.io
    Carol,carol@initech.dev,Software Engineer,initech.dev
    Dev,dev@umbrella.co,Director of Sales,umbrella.co
    Erin,erin.wu@hooli.com,Marketing Intern,hooli.com
    ```

    Then enter the request from the goal. The create-pipeline skill asks what it can't infer, such as which tools you use and how many people a run should cover. Then it reads its plan back in one paragraph and waits for your yes. It doesn't ask you to read YAML.

4. **Let it build and plan, one step at a time.** The skill writes a file and runs `gtme plan` after each step it adds. Plan spends nothing and calls no network. Here's the file it ends with, `trial-signups.yaml`:

    ```yaml
    name: trial-signups
    version: 1
    source:
      use: csv/source
      with:
        path: signups.csv
        columns: { title: Job Title }
    steps:
      - id: senior
        use: sql/filter
        with:
          query: >
            SELECT identity_id FROM current_values
            WHERE field = 'title'
              AND (value LIKE 'VP%' OR value LIKE 'Head of%' OR value LIKE 'Director%')
      - id: opener
        use: text/compose
        uses: [first_name, title]
        provides: [opener]
        with:
          template: >
            {{ record.first_name | default: "Hi there" }}, thanks for trying us.
            Most teams with a {{ record.title }} start with the weekly report, so that's where I'd look first.
      - id: out
        use: csv/deliver
        with:
          path: for-sdr.csv
        variables:
          email: email
          first_name: first_name
          opener: trial-signups.opener
        idempotency: email
    ```

    You don't need to read it; it's here so you recognize it later. `senior` keeps rows by title, `opener` fills in a sentence per person with [`text/compose`](/reference/adapters/text-compose), and `out` writes the file. Those are the [filter, compose, and deliver roles](/concepts/steps-and-roles).

    If the draft leaves out the `columns:` line, plan stops on it:

    ```sh
    gtme plan trial-signups.yaml
    ```

    ```text
    gtme: step "opener": needs title, which no earlier step provides (available: company_website, csv.job_title, email, first_name)
    ```

    The error names the fix: your header `Job Title` came in as `csv.job_title`, so the agent maps it to `title` and plans again. The output ends with the following:

    ```text
    ...
    send surface: 1 deliver step(s)
      out → csv/deliver (touch scope: trial-signups)

    available fields after the last step: company_website, email, first_name, title, trial-signups.opener
    plan ok — nothing has been spent
    ```

    `send surface` is the list of places this pipeline can write to. Here there's one, and it's a local file.

5. **Rehearse it with `--simulate`.** When plan is clean, Claude Code says it's built and moves on by itself, without waiting for you. The run-pipeline skill runs the whole file offline, the lowest rung of the [gate ladder](/concepts/gate-ladder):

    ```sh
    gtme run trial-signups.yaml --simulate
    ```

    The output is similar to the following:

    ```text
    simulate: recorded responses only — no network, no spend, nothing sends, nothing persists
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   5    -      0       -         -       $0    -
    senior  sql/filter    5   3    -      0       2         -       $0    -
    opener  text/compose  3   3    -      0       -         -       $0    -
    out     csv/deliver   3   0    -      0       -         -       $0    -
    ...
    total: $0 spent
    ```

    Five came in, `senior` filtered Carol and Erin, and three were held at `out`. Nothing persists, so `for-sdr.csv` doesn't exist yet.

6. **Read the dry-run.** A dry-run runs every step for real except delivery, and prints what each delivery would carry:

    ```sh
    gtme run trial-signups.yaml --dry-run
    ```

    The output is similar to the following:

    ```text
    dry run: deliver steps will resolve and receipt their variables, but nothing sends
    ...
    out: resolved variables for 3 record(s) — review, then run again without --dry-run to arm:
      jane.doe@acme.com
        email: "jane.doe@acme.com"
        first_name: "Jane"
        opener: "Jane, thanks for trying us. Most teams with a VP Marketing start with the weekly report, so that's where I'd look first."
    ...
    total: $0 spent
    ```

    The agent reads these lines back to you. This is the moment to say "make the opener shorter" or "drop the directors," and Claude Code edits the file, plans again, and brings you a new dry-run.

7. **Arm it.** Arming, the run that writes and sends for real, is the same command with no flag:

    ```sh
    gtme run trial-signups.yaml
    ```

    The output is similar to the following:

    ```text
    ...
    step    adapter       in  out  empty  cached  filtered  failed  cost  avoided
    source  csv/source    0   5    -      0       -         -       $0    -
    senior  sql/filter    5   3    -      0       2         -       $0    -
    opener  text/compose  3   0    -      3       -         -       $0    ?
    out     csv/deliver   3   3    -      0       -         -       $0    -
    total: $0 spent, $0.0000+? avoided via cache (3 records skipped)
    ```

    `out` wrote 3 rows. `opener` shows `cached 3` because the dry-run already wrote those lines to the [ledger](/concepts/ledger), and the armed run read them back. The `?` means `text/compose` has no price to report, and because it calls nothing, the skip saved $0.

    The target here is a file on your machine, so the agent can arm it after you've seen the dry-run's total line, here $0. On a vendor target, it stops and asks "Send it?" first, and an earlier "go ahead, don't ask again" doesn't count as that reply.

## What you have now

**You have `trial-signups.yaml`, and a `for-sdr.csv` your SDR can work from.** It holds Jane, Bob, and Dev, each with an opener. The first column, `identity_key`, is how gtme recognizes each person across runs, here their email ([Identity keys](/concepts/identity-keys)).

```text
identity_key,email,first_name,opener
jane.doe@acme.com,jane.doe@acme.com,Jane,"Jane, thanks for trying us. Most teams with a VP Marketing start with the weekly report, so that's where I'd look first."
...
```

To check it, ask Claude Code to run it again. The [receipt](/concepts/runs-and-receipts) shows `out` with `3` cached and `0` out, because `idempotency: email` means nobody gets written twice:

```text
...
out     csv/deliver   3   0    -      3       -         -       $0    $0.0000
...
```

Next Monday, replace `signups.csv` with the new export, start Claude Code in the same folder, and say "run the trial signups." New rows get openers, and last week's people are skipped.

## Next

- [Your CSV](/start/my-csv) swaps the SQL rule and the fill-in sentence for a model that judges and writes, with one key you enter.
- [Your stack](/start/my-stack) delivers to a vendor, where the arm gate is yours.
- [`gtme secret`](/reference/cli/secret) stores the keys a vendor pipeline asks for, from your own terminal.
