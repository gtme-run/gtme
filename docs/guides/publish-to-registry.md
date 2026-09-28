---
name: Publish to the registry
description: Contribute a verified binding to gtme-bindings, pinned in its index by commit and content hash, so anyone can install it by id with gtme adapters add
for: "You have a binding that passes gtme adapters verify in your project, and you want anyone to install it by its id."
learn:
  - "where a binding's folder goes in the registry and which checks its CI runs"
  - "write the index row that pins your commit and its content hash"
  - "preview your row with gtme adapters search before you push"
  - "what the pull request's checks and reviewers look at"
order: 19
roles: [extender]
links:
  - to: /guides/add-a-binding
    type: depends-on
    description: Writes and verifies the Stacklens binding this guide publishes
  - to: /concepts/adapter-tiers
    type: depends-on
    description: What a binding is, what a fixture is, and the registry's two trust tiers
  - to: /start/add-a-vendor
    type: relates-to
    description: Searches the registry before writing a binding, the check this guide starts with
  - to: /concepts/campaign-is-a-folder
    type: relates-to
    description: A bundle records each binding at its pin, the same pin the index row sets
  - to: /reference/cli/adapters
    type: relates-to
    description: The search, add, verify, and update verbs this guide runs
  - to: /spec#gtme-adapters--the-bindings-registry-adr-042
    type: decided-by
    description: URL-addressed bindings, the content hash, verify before install, and bare ids pinned at the index's sha
  - to: /spec#10a-the-binding-tier--universal-steps--decided-adr-022027
    type: decided-by
    description: Every binding ships fixtures, and those fixtures are its conformance test
  - to: /decisions#adr-042
    type: decided-by
    description: The registry is an index plus a verified set, and nothing installs unverified
  - to: /decisions#adr-059
    type: decided-by
    description: A bare registry id installs at the index row's pinned commit
---

# Publish to the registry

**You end with a pull request that adds your [binding](/concepts/adapter-tiers) to the registry, so anyone can install it by id.**

The registry is [gtme-bindings](https://github.com/gtme-run/gtme-bindings) on GitHub: a folder per binding, plus an index that `gtme adapters search` and `gtme adapters add` read.

## Before you start

- **Write and verify the binding first**, as in [Add a vendor with a binding](/guides/add-a-binding). This guide publishes that binding, `stacklens/companies`, from the project's `adapters/` folder.
- **You need** gtme, `git`, `python3`, a GitHub account, and the GitHub CLI, `gh`, signed in.
- **Spend:** nothing. Nothing in this guide calls a vendor, so no vendor key is needed: verify replays fixtures offline.
- **Sends:** the fork, the push, and the pull request are writes to GitHub under your account. Nothing else leaves your machine.

Stacklens is an invented vendor, so it will never be in the registry. The outputs on this page came from a scratch copy of gtme-bindings. Your ids, hashes, and commits will differ.

The registry lists a binding in one of two ways (the decision record, [ADR-042](/decisions#adr-042)). This guide takes the *verified* way: the folder lives in gtme-bindings, and the registry's CI, the checks GitHub makes on every pull request, verifies your fixtures on every change. To keep the folder in your own repository instead, add only the index row, with `tier: community`, `source.url` pointing at your repository, and `sha` and `sha256` taken from it.

## Add the binding to your fork

1. Search the registry, so you don't publish a second binding for the same endpoint:

    ```sh
    gtme adapters search stacklens
    ```

    The output is the following:

    ```
    no registry entries match "stacklens" (index: https://raw.githubusercontent.com/gtme-run/gtme-bindings/main/index.json)
    an agent that finds nothing writes a binding — `gtme help --bindings`
    ```

    If a row matches your endpoint, improve that binding with a pull request instead of adding a second one.

1. From outside your project folder, fork the registry, clone your fork, and start a branch. A fork is your own copy of the repository on GitHub, and the clone is its copy on your machine:

    ```sh
    gh repo fork gtme-run/gtme-bindings --clone
    cd gtme-bindings
    git switch -c stacklens-companies
    ```

    `gh` names your fork `origin` and the registry `upstream`.

1. Copy the binding's folder to the top of the clone:

    ```sh
    cp -R PROJECT_DIR/adapters/stacklens-companies .
    ```

    Replace `PROJECT_DIR` with the project folder that holds your binding. Keep the folder's name: the index row's `path` names it, and `gtme adapters add` installs to a folder of the same name.

1. Replace real people in the fixtures. A fixture recorded from a live account can hold real names, emails, and companies. The registry's README asks for made-up values in the vendor's real field names and structure, so edit the values in `fixtures/conformance.json` and leave its keys and nesting alone.

1. Verify every binding in the clone, the same check the registry's CI makes:

    ```sh
    for f in */binding.yaml; do
        id=$(awk '/^id: /{print $2; exit}' "$f")
        echo "== $id"
        GTME_ADAPTER_PATH=$PWD gtme adapters verify "$id"
    done
    ```

    The output is the following:

    ```
    == apollo/enrich
    apollo/enrich v1 — enrich (person)
    ...
    == stacklens/companies
    stacklens/companies v1 — source (company)
      calls:       api.stacklens.example
      demands:     STACKLENS_API_KEY
      needs:       none
      provides:    company_domain, company_employees, company_industry, company_name, stacklens.id, stacklens.technologies
      fixtures:    ok — 3 response(s) on file, 5 record(s) extracted
    ```

    `GTME_ADAPTER_PATH=$PWD` makes gtme find each binding in the clone instead of in `~/.gtme/adapters`. CI builds gtme from its main branch, so an older gtme can pass here and fail in CI. Upgrade first if yours is behind.

1. Commit the folder on its own:

    ```sh
    git add stacklens-companies
    git commit -m "stacklens/companies: companies that use a given technology"
    ```

    The index row records this commit's sha, and a commit can't contain its own sha, so the row goes in a second commit.

## List it in the index

1. Print the commit's sha, and the folder's content hash with the registry's `scripts/hash.sh`:

    ```sh
    git rev-parse HEAD
    ```

    The output is similar to the following:

    ```
    e65b97a684bb879d4bd4ee91edf183a1f9be5252
    ```

    ```sh
    scripts/hash.sh stacklens-companies
    ```

    The output is similar to the following:

    ```
    d3725babb4ca18da95b84e4cc372f95c81b74d7695303d83dccab5abf0a1f776
    ```

    The content hash is a sha256 over every file in the folder. `gtme adapters add` computes the same hash after it fetches, and refuses to install on a mismatch.

1. In `index.json`, add your entry at the end of the `bindings` list, and set `generated_at` at the top of the file to the output of `date -u +%Y-%m-%dT%H:%M:%SZ`:

    ```json
    {
      "id": "stacklens/companies",
      "description": "Source companies that use a given technology from Stacklens, with each company's technology list; page pagination, $0.02 per record.",
      "vendor": "stacklens",
      "role": "source",
      "entity_type": "company",
      "provides": [
        "company_domain",
        "company_name",
        "company_employees",
        "company_industry",
        "stacklens.id",
        "stacklens.technologies"
      ],
      "credentials": [
        "STACKLENS_API_KEY"
      ],
      "source": {
        "url": "github.com/gtme-run/gtme-bindings",
        "path": "stacklens-companies",
        "ref": "main",
        "sha": "COMMIT_SHA"
      },
      "sha256": "CONTENT_SHA256",
      "tier": "verified",
      "since": "2026-09-28"
    }
    ```

    Replace the following:

    - `COMMIT_SHA`: the output of `git rev-parse HEAD`
    - `CONTENT_SHA256`: the output of `scripts/hash.sh`

    `role`, `entity_type`, `provides`, and `credentials` repeat what verify printed. `since` is the date you publish. Leave `ref` as `main`, because `sha` is what pins. Search shows the first 60 characters of `description`, so lead with the role and what the records are.

1. In a second terminal, from the clone, serve it over HTTP, because gtme reads an index only from a URL:

    ```sh
    python3 -m http.server 8000 --bind 127.0.0.1
    ```

1. Back in the first terminal, search your copy of the index. `GTME_REGISTRY` points gtme at an index other than the registry's:

    ```sh
    GTME_REGISTRY=http://localhost:8000/index.json gtme adapters search stacklens
    ```

    The output is the following:

    ```
    ID                   ROLE    TIER      INSTALL                                                                       DESCRIPTION
    stacklens/companies  source  verified  gtme adapters add github.com/gtme-run/gtme-bindings/stacklens-companies@main  Source companies that use a given technology from Stacklens…
    ```

    gtme checked the whole file against the index schema before it searched. A row that breaks the schema fails the search and names the key, such as a hash pasted short:

    ```
    gtme: adapters: registry index http://localhost:8000/index.json does not conform to registry-index.schema.json: jsonschema: '/bindings/6/sha256' does not validate with https://gtme.spec/schemas/registry-index.schema.json#/properties/bindings/items/properties/sha256/pattern: does not match pattern '^[0-9a-f]{64}$'
    ```

    Stop the server when the row reads the way you want.

1. Commit the index row as a second commit:

    ```sh
    git commit -am "index: stacklens/companies, verified"
    ```

## Open the pull request

1. Push the branch to your fork and open the pull request against the registry:

    ```sh
    git push -u origin stacklens-companies
    gh pr create --fill-first
    ```

    `gh` opens the pull request on the registry and prints its URL. `--fill-first` takes the title and body from the folder's commit. `gh pr checks` shows the registry's checks as they finish.

**The registry's CI and a maintainer check the pull request.** The registry's `verify` workflow checks the pull request. It builds gtme, repeats the verify loop you ran, and recomputes the hash of every verified row. If a file in the folder changed after you hashed it, the hash check fails with a line like this:

```
stacklens/companies: index d3725babb4ca != content 092c2fcc3f94
```

Rehash with `scripts/hash.sh` and update `sha256`. If you amended or rebased the folder's commit, its sha changed too, so update `sha` to match. CI doesn't check `sha`, so before you push, compare it with the folder's last commit:

```sh
git log -1 --format=%H -- stacklens-companies
```

A maintainer then reads the pull request. What verify printed is the surface they read ([ADR-042](/decisions#adr-042)): the host the binding calls, the key it demands, and the fields it provides.

## What you have now

**After the merge, `index.json` on the registry's main branch lists your binding, and anyone can install it by id:**

```sh
gtme adapters add stacklens/companies
```

The output is similar to the following:

```
fetched github.com/gtme-run/gtme-bindings/stacklens-companies@e65b97a684bb879d4bd4ee91edf183a1f9be5252 at e65b97a684bb
stacklens/companies v1 — source (company)
  calls:       api.stacklens.example
  demands:     STACKLENS_API_KEY
  needs:       none
  provides:    company_domain, company_employees, company_industry, company_name, stacklens.id, stacklens.technologies
  fixtures:    ok — 3 response(s) on file, 5 record(s) extracted
installed stacklens/companies at /tmp/gtme-pub/.gtme/adapters/stacklens-companies — pinned to e65b97a684bb
```

The bare id resolved to the row's `sha`, the commit that added your folder, so it installs the same commit as `github.com/gtme-run/gtme-bindings/stacklens-companies@SHA` would ([ADR-059](/decisions#adr-059)). `add` verified the binding and matched its hash before it installed anything, and it recorded the pin in `.source.json` beside the binding ([SPEC §8](/spec#gtme-adapters--the-bindings-registry-adr-042)). Nothing moves that pin until whoever installed it updates to a new reference, such as `gtme adapters update stacklens/companies @main`.

A fix works the same way: a pull request that changes the folder, then updates the row's `sha` and `sha256`.

## Hand it to Claude Code

**Paste this line into Claude Code for your own binding:**

```text
Publish my gtme binding in PROJECT_DIR/adapters/FOLDER to gtme-run/gtme-bindings: fork and clone it, copy the folder in, replace real people in the fixtures with made-up values, run the registry's verify loop, commit the folder, add its index.json row with the commit sha and scripts/hash.sh output and set generated_at, preview it with GTME_REGISTRY, and stop before you push.
```

Replace the following:

- `PROJECT_DIR`: the project folder that holds your binding
- `FOLDER`: the binding's folder, its id with the slash as a dash

The agent forks the registry under your account, then stops with both commits on your branch for you to read and push.

## Next

- [A campaign is a folder](/concepts/campaign-is-a-folder) shows how a bundle carries your binding at its pin.
- [`gtme adapters`](/reference/cli/adapters) lists every form of search, add, verify, and update.
- [Setting up an agent](/start/for-agents) installs the Claude Code plugin and the rules that keep an agent from spending or sending until you say so.
