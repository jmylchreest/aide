---
sidebar_position: 5
---

# From Source

For contributors to aide itself, and for anyone who wants to build and run a
local checkout.

## Prerequisites

- Go 1.25+ (see `go.mod` for the exact pinned version)
- [Bun](https://bun.sh) 1.0+ — the runtime for all plugin and web UI
  TypeScript, per the `plugin-runtime` decision
- Git

## Build

```bash
git clone https://github.com/jmylchreest/aide && cd aide

# Install JS dependencies — this also installs the git hooks (see below)
bun install

# Build the Go binary
make build

# Build the TypeScript
bun run build
```

`make test` runs both suites; `make test-go` and `make test-ts` run them
individually.

## Git hooks

The repo uses [lefthook](https://github.com/evilmartians/lefthook) to manage
git hooks. Pre-commit runs three jobs, ~70ms in total:

- **export-decisions** — exports the local decision store to `.aide/shared/`
  and stages it, so architectural decisions travel with the repo instead of
  living only in one developer's database.
- **check-shared-context** — refuses to publish a decision containing a
  developer-specific absolute path.
- **check-resolver-guards** — fails on new inline project-root resolution
  logic outside the allowlist (also run in CI).

**`bun install` installs the hooks for you** via the root `prepare` script. If
you work only on the Go side and never run `bun install`, install them
explicitly:

```bash
make hooks
```

Both paths need bun, so the hook jobs are bun scripts rather than shell — they
run natively on Windows, no Git Bash required. Note that the wider dev workflow
is not Windows-native: the Makefile and several `scripts/*.sh` helpers assume a
POSIX shell, so Windows contributors want WSL or Git Bash regardless.

Both are idempotent, and lefthook re-syncs `.git/hooks` by itself whenever
`lefthook.yml` changes. `make build` and `make test` print a warning if hooks
are missing — they never install or fail on their own, so a build never
depends on network access or silently rewrites your `.git/hooks`.

To check what would run, or to run a hook by hand:

```bash
bunx lefthook run pre-commit
```

`git commit --no-verify` bypasses the hooks for one commit.

### Why there is no automatic install on clone

Git has no client-side clone hook, by design — a repository that could execute
code on `git clone` would be a supply-chain vulnerability, so hooks are never
transferred with a clone and no committed file can install itself. Every hook
manager, lefthook and husky alike, works around this the same way: something
you run *after* cloning does the install. Here that is `bun install` or
`make hooks`.

If you want hooks bootstrapped in every repository you clone, the one genuine
clone-time mechanism is a git template directory, configured once per machine:

```bash
mkdir -p ~/.git-template/hooks
git config --global init.templateDir ~/.git-template
```

Anything in `~/.git-template/hooks/` is copied into `.git/hooks/` on every
`git clone` and `git init` thereafter. A `post-checkout` hook there that runs
`lefthook install` when it finds a `lefthook.yml` gives you true
install-on-clone. It is a personal machine setting, not something the repo can
configure on your behalf.

## Shared context

`.aide/shared/` holds decisions exported as git-friendly markdown, one
directory per topic and one write-once file per decision version. It is
committed; the BoltDB store it comes from (`.aide/memory/`) is gitignored and
machine-local.

Two consequences worth knowing:

- **CI cannot regenerate it.** Because the source of truth is a local
  database that never reaches git, there is no "re-export and diff" check.
  What CI does run is `scripts/check-shared-context.sh`, which fails the build
  if a published decision hard-codes a developer's home directory.
- **Version files are never rewritten.** Once a decision version is committed,
  a later export leaves it alone. Fixing bad published text means purging the
  topic (`aide decision delete <topic>`) and re-recording it — a deletion
  propagates as a tombstone, which unpublishes the old files everywhere.

Exports merge additively between developers: removal is driven by tombstones,
never by "absent from my local store", so two people with divergent local
stores do not clobber each other.

To pick up decisions others have committed:

```bash
aide share import
```

## Install a local build

### Claude Code

```bash
claude --plugin-dir /path/to/aide
```

### OpenCode

```bash
bunx @jmylchreest/aide-plugin@latest install --plugin-path /path/to/aide
```

:::note
The `aide` Go binary comes with the plugin: npm installs ship it as a
per-platform package (`@jmylchreest/aide-binary-<platform>-<arch>`, an
optionalDependency pinned to the plugin version), and Claude Code marketplace
installs download it on first run. Building from source is only needed for
development or customization.
:::
