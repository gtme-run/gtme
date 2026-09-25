---
name: Install
description: Install gtme with Homebrew, check it with gtme version and gtme init, and update or remove it later
for: "You have a Mac or Linux terminal and haven't installed gtme yet."
learn:
  - "install gtme with Homebrew or from a release"
  - "prove the install with `gtme version` and `gtme init`"
  - "update and uninstall it"
order: 1
links:
  - to: /start/show-me
    type: relates-to
    description: The next page, which runs a whole pipeline offline with no keys and ends in a receipt
  - to: /start/for-agents
    type: relates-to
    description: The same install handed to Claude Code as one paste line, plus the Claude Code plugin
  - to: /concepts/ledger
    type: relates-to
    description: gtme init creates the ledger, the file every run reads from and writes to
  - to: /reference/cli/init
    type: relates-to
    description: The command that creates ~/.gtme and the ledger
  - to: /reference/cli/version
    type: relates-to
    description: The command that prints the installed version
---

# Install

gtme runs go-to-market work, such as finding people, scoring them, and adding them to a campaign, from a YAML file you can read, change, and run again.

**This page needs a Mac or Linux computer and a terminal.** No API keys, no accounts.

**It spends $0.** The only things it adds to your machine are the `gtme` program and, after you run `gtme init`, a folder at `~/.gtme`.

## Install with Homebrew

**We recommend Homebrew.** Paste this into your terminal:

```sh
brew install gtme-run/tap/gtme
```

If you don't have Homebrew, [its home page](https://brew.sh) has the install line. Homebrew downloads the ready-built `gtme` program and checks that it's the exact file we published. Nothing on this page runs a script it downloaded, and gtme never phones home.

Three other ways give you the same program:

- Download the file for your computer from the [releases page](https://github.com/gtme-run/gtme/releases/latest), unpack it, and move `gtme` into a folder your terminal looks in for programs, such as `~/.local/bin`.
- If you have Go 1.25 or newer, run `go install github.com/gtme-run/gtme/cmd/gtme@latest`.
- From a copy of the source code, run `./install.sh`, which builds gtme, puts it in `~/.local/bin`, and runs `gtme init` for you.

## Check that it worked

**Two commands prove the install.** The first prints the version you have:

```sh
gtme version
```

The output is similar to the following:

```
gtme v0.6.1
```

The second creates the `~/.gtme` folder and the [ledger](/concepts/ledger) inside it, the one file where gtme remembers the people, facts, and deliveries from every run:

```sh
gtme init
```

The output is similar to the following:

```
ledger created: /var/folders/cm/pyqn674s1xbbrfv0yfbx5xwr0000gn/T/tmp.8JJJKBqb9D/.gtme/ledger.db
gtme home: /var/folders/cm/pyqn674s1xbbrfv0yfbx5xwr0000gn/T/tmp.8JJJKBqb9D/.gtme
```

On your machine, both paths start with your home folder. Running `gtme init` again is safe: it prints `ledger up to date` and changes nothing.

## Update or remove gtme

- To update, run `brew upgrade gtme`.
- To uninstall, run `brew uninstall gtme`, then `rm -r ~/.gtme`. The second command deletes the ledger and everything gtme remembered.

## Next

**[Door 1: show me](/start/show-me) runs a whole outbound pipeline offline with no keys.** It ends in a [receipt](/concepts/runs-and-receipts), one row per step with what it cost. If you'd rather have Claude Code do the typing, [For agents](/start/for-agents) has the line to paste and the Claude Code plugin.
