---
name: For agents
description: Hand gtme to Claude Code or any agent with a shell in one paste line, and the rules it follows before anything spends or sends
order: 6
links:
  - to: /start/show-me
    type: relates-to
    description: The first door, which needs no keys and spends nothing, and the one we recommend the agent start with
  - to: /start/add-a-vendor
    type: relates-to
    description: The last of the four doors; an agent walks the same doors this collection walks by hand
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: Every door ends in a receipt, which is what the agent reports back to you
  - to: /concepts/gate-ladder
    type: relates-to
    description: The rungs from simulate to armed; the agent climbs through dry-run and you arm a live target
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: The run-pipeline skill runs bundles as well as single pipeline files
  - to: /concepts/adapter-tiers
    type: relates-to
    description: The create-adapter skill writes adapters for vendors gtme doesn't ship
  - to: /reference/plugin-skills
    type: relates-to
    description: Each plugin skill's trigger, procedure, and the commands it runs
  - to: /concepts/agent-operable
    type: relates-to
    description: Why the binary describes its whole surface in one machine-readable document an agent reads when it needs to
  - to: /reference/cli/help
    type: relates-to
    description: The help verb, including the --agent output the agent reads
  - to: /guides/claude-code
    type: relates-to
    description: The next page for this reader, a full session driving gtme from Claude Code
---

# For agents

gtme is a single binary that runs a go-to-market campaign written as a YAML file and prints a receipt of what each run did and what it cost.

**This page needs an agent.** Use Claude Code for the plugin, or any agent that can run a shell for the paste line.

**This page spends nothing.** The paste line and the plugin install cost $0. Each door states what its run spends right before the command, and the agent reads that line too.

Paste this line into your agent:

```text
Follow gtme.run/start.md
```

The line points the agent at [START.md](https://gtme.run/start.md), the whole instruction set in one file, and you can open it before you paste. The agent installs gtme, then walks one door to a [receipt](/concepts/runs-and-receipts). A door is one path from nothing to a receipt. There are four, from Door 1 to [Door 4: add a vendor](/start/add-a-vendor), and they're the same doors this collection walks by hand.

We recommend Door 1 first, because it costs nothing and proves the install. After that, pick the door that matches what you have: a CSV, vendor keys, or a vendor gtme doesn't ship.

## The plugin

**In Claude Code, the plugin is optional and adds four slash commands.** Paste the line first, then add the plugin if you want the commands. You can do both.

```text
/plugin marketplace add gtme-run/gtme
/plugin install gtme@gtme-run
```

Claude Code picks a skill from what you ask for, or you can call one by name:

| Skill | What it does |
|---|---|
| `/gtme:create-pipeline` | Turns an outreach goal, in your words, into a pipeline, the YAML file that lists a campaign's steps |
| `/gtme:run-pipeline` | Rehearses, runs, or ships a pipeline or [bundle](/concepts/campaign-is-a-folder). Shipping is the armed run, and on a live target that command is yours |
| `/gtme:create-adapter` | Writes an [adapter](/concepts/adapter-tiers) for a vendor or data source gtme doesn't ship, or a new entity type, meaning a kind of record such as posts or jobs |
| `/gtme:analyze` | Answers what a run did, what it cost, why a record dropped, and where each fact came from |

The skills hold procedure only. They take their facts from the installed binary, from START.md, and from the pattern bundles in the gtme repository, so they track the version you have.

## The rules the agent follows

**Nothing sends until you run it, and nothing spends until you've read what it costs.** START.md ends with five rules the agent reads before its first command. The run-pipeline skill restates the arm gate and the key rule. The create skills stop at plan or simulate and hand off to it, and analyze only reads, so it can't spend or send.

- **Never arm a live target.** Arming means running `gtme run` without `--simulate` or `--dry-run`, so it runs for real. A live target is a vendor the pipeline sends to, such as an Instantly campaign, and that run is yours after you've read the dry-run receipt. A local target, such as a CSV on your machine, is the agent's to run after you've read the line that says what it spends.
- **Never handle a key.** `gtme secret set KEY` prompts you, and you type the key. The agent never puts one on a command line, in a file, or in the chat.
- **Announce spend first.** The agent climbs [the gate ladder](/concepts/gate-ladder) in order. `gtme plan` and `--simulate` spend $0. A dry-run spends on vendor credits and model tokens and holds only the send. So the agent tells you the cost before any command that spends, and waits for you when the target is live.
- **Follow the error.** Each error message names its fix. The agent does the named thing and runs the same command again.
- **Stop at "done when."** Each door ends with a condition. The agent reports the receipt and waits, because the next door is yours to open.

**The binary doesn't know who typed a command.** The arm gate is the agent's rule plus your read of the dry-run receipt. What the binary enforces is that plan and simulate spend nothing and a dry-run sends nothing.

## Where the agent looks things up

**The agent reads the whole CLI and adapter surface from `gtme help --agent` when it needs to, and you never have to.** [Agent-operable by design](/concepts/agent-operable) explains why, and the [help reference](/reference/cli/help) documents the command.

## Next

**[Use gtme from Claude Code](/guides/claude-code) walks a full session from goal to receipt.** If you'd rather see a receipt with your own eyes first, run [Door 1: show me](/start/show-me) by hand.
