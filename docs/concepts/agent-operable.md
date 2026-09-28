---
name: Agent-operable by design
description: gtme describes its whole surface in one JSON document, keeps data on stdout and prose on stderr, and exits with one code per kind of failure
for: "You're handing gtme to an agent or a script and want to know what it reads, what it can parse, and how it tells a bad file from a missing key."
learn:
  - "what `gtme help --agent` contains and where it comes from"
  - "which output is data for a program and which is text for you"
  - "what each exit code means and what an agent does next"
  - "why plan is the check an agent runs after every edit"
order: 14
roles: [agent, builder]
defines:
  - term: "exit code"
    definition: "The number a gtme command ends with, one per kind of failure: 0 ok, 2 a refused file or value, 3 a missing credential, 4 rate-limited, 5 network, 1 anything else."
  - term: "stdout"
    definition: "The output stream gtme reserves for data, such as query rows, record JSON, and frozen YAML, so a program can read it without cleanup."
  - term: "stderr"
    definition: "The output stream gtme writes everything meant for a person to, such as plans, progress, receipts, and errors."
links:
  - to: /start/for-agents
    type: relates-to
    description: The agent's way in, and the rules it follows before anything spends or sends
  - to: /concepts/adapter-tiers
    type: relates-to
    description: Every installed adapter's manifest is in the document, and help --bindings is the contract for writing a new one
  - to: /concepts/pipeline
    type: relates-to
    description: What an agent writes from the document, and what plan checks
  - to: /concepts/ledger
    type: relates-to
    description: The document lists the ledger's tables and views, which is how an agent reads a run's numbers as JSON
  - to: /concepts/runs-and-receipts
    type: relates-to
    description: The receipt is text on stderr, and every number on it is a ledger row an agent can query
  - to: /concepts/gate-ladder
    type: relates-to
    description: Plan is the rung that checks and prices a file with no network and no spend
  - to: /concepts/participants
    type: relates-to
    description: An agent answers agent steps with gtme answer, reading pending records from stdout first
  - to: /start/add-a-vendor
    type: relates-to
    description: Uses gtme help --bindings, the second document, to write a binding
  - to: /reference/cli
    type: relates-to
    description: Every verb in one table, with the exit codes they share
  - to: /reference/cli/help
    type: relates-to
    description: The forms of gtme help, including --agent and --bindings
  - to: /reference/cli/query
    type: relates-to
    description: Read-only SQL against the ledger, with rows on stdout
  - to: /reference/cli/freeze
    type: relates-to
    description: Prints a run's exact pipeline as YAML on stdout
  - to: /reference/cli/secret
    type: relates-to
    description: The prompt where you, never the agent, enter a key after exit code 3
  - to: /reference/cli/answer
    type: relates-to
    description: How an agent records its judgment on a pending record
  - to: /reference/adapters/demo-enrich
    type: relates-to
    description: The keyless adapter whose provides block the page reads from the document
  - to: /decisions#adr-007
    type: decided-by
    description: gtme help --agent prints the full surface, generated from the binary and never kept by hand
  - to: /decisions#adr-041
    type: decided-by
    description: The binding contract lives in a second document so the pipeline document stays short
  - to: /decisions#adr-049
    type: decided-by
    description: Agents get agent steps and gtme answer, so an agent drives gtme instead of being launched by it
  - to: /spec#8-cli-surface--decided
    type: decided-by
    description: Human output on stderr, data on stdout, the exit codes, and the round-trip test for help --agent
---

# Agent-operable by design

Ask the binary to describe itself. `jq`, a small command-line JSON reader (`brew install jq`), lists the top-level keys of what it prints:

```sh
gtme help --agent | jq keys
```

It prints:

```json
[
  "adapters",
  "bindings",
  "examples",
  "exit_codes",
  "ledger",
  "participants",
  "sql_steps",
  "verbs"
]
```

That's one JSON document, printed by the binary you have installed. An agent reads it when it needs to know what gtme can do.

## What you just saw

**Each key is one part of gtme an agent can use.**

| Key | What an agent gets |
|---|---|
| `verbs` | Every command and flag, with a line on what each does |
| `adapters` | What every installed [adapter](/concepts/adapter-tiers) needs and provides, read from its manifest, the file that declares both |
| `examples` | Whole [pipeline](/concepts/pipeline) files to start from |
| `ledger` | The tables and views of the [ledger](/concepts/ledger), for writing SQL |
| `sql_steps` | How the `sql/*` steps, the ones that run SQL against the ledger, read it |
| `participants` | The four commands for answering an `agent/*` step, a step whose judgment the agent itself supplies |
| `exit_codes` | What each exit code means |
| `bindings` | A pointer to `gtme help --bindings`, the contract for writing a new adapter |

Here's what the document says [`demo/enrich`](/reference/adapters/demo-enrich), the pretend adapter from [See it run](/start/show-me), provides:

```sh
gtme help --agent | jq '.adapters[] | select(.id == "demo/enrich") | .provides'
```

It prints:

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "demo.score": {
      "type": "integer",
      "minimum": 0,
      "maximum": 100
    },
    "demo.note": {
      "type": "string"
    }
  }
}
```

That's how an agent knows `demo.score` exists, and that it's a whole number from 0 to 100, before it writes a step that reads it. The binary builds the document from the adapters it finds each time you call it, so an adapter you install shows up in the next call.

## Data on stdout, prose on stderr

**Every command writes what you read to stderr and what a program reads to stdout.** stdout and stderr are a program's two output streams. A terminal shows both, so you never see the difference. A pipe or a file gets stdout only.

After a run of `hello.yaml` from See it run, ask for the run's records and discard stderr with `2> /dev/null`:

```sh
gtme show --run last 2> /dev/null
```

It prints one JSON object per line. Trimmed to Jane with `...`:

```
{"entity_type":"person","fields":{"company_domain":"acme.com","demo.note":"synthetic — demo/enrich called no vendor","demo.score":100,"email":"jane.doe@acme.com","full_name":"Jane Doe","title":"VP Marketing"},"identity_key":"jane.doe@acme.com","identity_key_tier":"email","state":"out"}
...
```

The line that went to stderr was `3 record(s) from run 01M3FMC2CE1H6B17NEJSYG4VJ3`, a count for you. What's left, an agent can pipe into `jq` or save to a file as is. [`gtme freeze`](/reference/cli/freeze), which prints the exact pipeline a run used, puts its YAML on stdout the same way. The pending listing a [participant](/concepts/participants) reads before it answers puts one JSON line per record there.

**The receipt is for you, and every number on it is a row an agent can query.** The [receipt](/concepts/runs-and-receipts) goes to stderr as a table. In See it run, its `score` row says `$0.0300` and its total says `(estimated)`. The same dollars come back from the ledger as JSON:

```sh
gtme query "SELECT step_id, SUM(amount_usd) AS usd, basis
            FROM costs GROUP BY step_id, basis"
```

It prints:

```
{"basis":"estimated","step_id":"score","usd":0.03}
1 rows
```

The JSON line is on stdout and `1 rows` is on stderr. `basis` says whether the dollars were measured from a vendor's response or estimated from a rate.

## One exit code per kind of failure

**When a command fails, the exit code says what kind of failure it was, and the message says the fix.** An exit code is the number a command ends with, which a script or an agent reads before any text. Every error naming its fix is one of gtme's [design principles](/spec#0-design-principles-context-non-normative). In `hello.yaml`, the `out` step ends like this:

```yaml
    variables:
      score: demo.score
      note: demo.note
```

Change `demo.score` to `demo.scor` on the `score:` line, then plan the file and print the exit code:

```sh
gtme plan hello.yaml; echo $?
```

It prints:

```
gtme: step "out": needs demo.scor, which no earlier step provides (available: company_domain, demo.note, demo.score, email, full_name, title)
2
```

The message names the step, the field, and what's available, and nothing went to stdout. Here are the codes, from the same document:

```sh
gtme help --agent | jq -c '.exit_codes[]'
```

It prints:

```
{"code":0,"means":"ok"}
{"code":1,"means":"other error"}
{"code":2,"means":"validation or contract error: the plan, the file, or a value was refused"}
{"code":3,"means":"auth or credential error"}
{"code":4,"means":"rate-limited by a vendor"}
{"code":5,"means":"network error"}
```

## So what?

An agent picks its next move from the number:

| Code | What the agent does next |
|---|---|
| 2 | Edits the file and plans again. |
| 3 | Asks you for the key the message asks for. You enter it at the [`gtme secret set`](/reference/cli/secret) prompt, never the agent, per For agents. |
| 4 or 5 | Runs the same command again later. That's safe: finished work is cached, and a delivery with the same key isn't made twice, for the reasons on the receipt page. |
| 1 | Reads the message. Code 1 carries no other hint. |

**Exit 0 means the command ran, not that it did anything useful.** A run that spends money and sends nothing still exits 0, with the spend on its receipt, so an agent checks the numbers with `gtme query` before it reports. A run that stops at an `agent/*` step isn't finished either; Participants covers what the agent does next.

**Plan is the check an agent can afford after every edit.** [Plan](/concepts/gate-ladder) resolves every field and credential with no network and no spend. Exit code 0 means every field and credential resolved, and plan's last line says nothing has been spent.

That's it. An agent reads one document, parses stdout, branches on the exit code, and plans before anything spends. If your agent needs telling, this is the block to give it:

```
Run `gtme help --agent` before writing a pipeline; it is the whole surface.
Read stdout as data and stderr as notes for a person.
Branch on the exit code: 2 fix the file, 3 ask me for the key, 4 or 5 retry later.
Run `gtme plan` after every edit, and show me the cost before any run that spends.
```

## Why it's this way

**Most of the document is adapter manifests, so it grows with every adapter you install.** With the 20 built-in adapters it's about 48 kilobytes. We'd have an agent pull the one key it needs with `jq`, as this page does.

**An agent works from whatever document is in front of it, so gtme generates that document.** A page kept by hand would drift from the binary. The decision record [ADR-007](/decisions#adr-007) made `gtme help --agent` print the whole surface from the binary itself. SPEC.md, the file that fixes what gtme does, sets [the test](/spec#gtme-help---agent-adr-007): an agent given only this document must be able to write a pipeline that passes `gtme plan`.

**Writing a binding takes a second document.** The binding contract is large, and most agents never need it, so [ADR-041](/decisions#adr-041) moved it to `gtme help --bindings`. [Add a vendor](/start/add-a-vendor) uses it.

**The streams and the codes are a contract, so a script can rely on them.** [SPEC §8](/spec#8-cli-surface--decided) fixes both. The code says whether a command broke. Whether it did anything useful is a question for the ledger.

## Where it shows up

- [For agents](/start/for-agents) is the agent's way in, with the rules it follows before anything spends or sends.
- [`gtme help`](/reference/cli/help) and [the CLI reference](/reference/cli) list every form and the shared exit codes.
- [`gtme query`](/reference/cli/query) is the lookup page for reading the ledger as JSON.
