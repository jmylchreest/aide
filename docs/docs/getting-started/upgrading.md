---
sidebar_position: 9
---

# Upgrades and running versions

An aide installation has several versions that can differ: the installed plugin,
the npm package used to launch MCP, the executable on disk, the running MCP
process, and the primary process that owns the project's stores. Updating one
does not replace the others automatically.

## Upgrade the distribution you use

- **Claude Code:** update the aide marketplace plugin, then reload the plugin or
  restart Claude Code. The wrapper runs from that plugin's directory and downloads
  a binary suitable for its version when needed.
- **Codex marketplace plugin:** upgrade the marketplace. The native manifest uses
  `bunx -y @jmylchreest/aide-plugin@latest mcp`, which selects the latest **published
  npm release**. Refresh separately installed hooks with
  `bunx @jmylchreest/aide-plugin@latest install --platform codex`. Restart or
  reconnect MCP to load newly installed code.
- **OpenCode:** run `bunx @jmylchreest/aide-plugin@latest install` to update the
  plugin and MCP pins together, then restart OpenCode. A marketplace update in
  another assistant does not update OpenCode's configuration.
- **Standalone executable:** `aide upgrade` updates only the executable you ran.
  It does not update plugin files, npm dependencies, or existing processes.

`@latest` refers to npm's distribution tag, not the newest GitHub tag. An
unversioned package name can also resolve a local installation or cached package.
[Bun documents the difference](https://bun.sh/docs/pm/bunx). Keep the floating
published-release channel in the Codex manifest: an exact pin to a Git tag can
fail before the corresponding npm publication completes.

Do not use `@latest` independently for every component. OpenCode's installer
resolves a release, then pins its plugin and MCP command together. Run the
installer again to advance both. Claude Code loads hooks and skills from its
marketplace cache; replacing only its MCP command with a floating npm launcher
would allow those components to drift apart. Its wrapper therefore stays tied
to the loaded plugin directory. Codex's native MCP launcher floats because
marketplace metadata can advance before npm publication; separately installed
hooks still need their own update.

Package resolution also differs from process lifetime: a new invocation of
`bunx ...@latest` checks npm, but does not change a server that is already running.
Registry-outage behavior depends on the Bun version. Current Bun documentation
describes a cached fallback, but the isolated rehearsal on Bun 1.4.4 failed a new
`@latest` launch on HTTP 503 despite a warm cache. Existing MCP processes kept
serving requests. OpenCode's current plugin update commands manage its own cache,
independently of Bun's MCP launcher cache. Avoid putting npm resolution on every
hook invocation.

## Rehearse a Codex plugin upgrade without publishing

From a checkout with dependencies installed, run:

```bash
python3 scripts/test-plugin-upgrade.py
```

This developer test requires Python 3, Bun, and Go. It builds two synthetic
versions, serves npm-format packages and their runtime dependencies from a
loopback registry, and launches the actual Codex manifest command through Bun.
It uses a disposable project, package cache, and configuration. It never runs
`npm publish`, a marketplace upgrade command, or an installer against your host.
The integration CI job runs it too.

The test checks these transitions:

| Scenario | Expected result |
| --- | --- |
| Marketplace fixture advances before npm | The latest published package remains 0.2.1. |
| Registry advances with a warm unversioned cache | The unversioned launcher still selects 0.2.1. |
| Two `@latest` clients launch after publication | Both load 0.2.2 but attach to the existing 0.2.1 primary, reporting version skew. |
| Only the old primary restarts | A surviving 0.2.1 client can become primary. |
| Every session restarts | The primary and clients use 0.2.2, and stored state survives. |
| Registry returns HTTP 503 | The test records whether Bun launches its cached copy or fails the new launch; existing processes must remain healthy. |

Use `--keep` to retain logs, package archives, registry requests, and `report.json`.
Use `--old-binary /path/to/aide-0.2.1` to exercise a released binary as the baseline.
Without that option both Go binaries use current source with different version
stamps. Both fixture npm packages use the current plugin source in either case.

The marketplace step replaces a **fixture manifest**. This proves package
resolution and process ownership after the new manifest is available, not the
host's actual marketplace download, reload, config precedence, or separately
installed hook migration. Claude Code's GitHub binary-download path and
OpenCode's installer pins have separate tests.

A project MCP override pointing at a local checkout continues to use that
checkout. A marketplace upgrade does not test the npm launch path while that
override is active. The rehearsal deliberately launches from the manifest in
its disposable project instead of reading your development override.

## Find the version actually serving requests

Call the `instance_info` MCP tool. It reports the loaded build, executable path,
PID, and `authority`. When `authority` is `client`, its `daemon` block identifies
the primary process; `version_skew` flags different client and primary versions.
The primary's version matters even when the client and executable are current.

Run `./.aide/bin/aide version` to inspect the project CLI executable. Check the
wrapper log under the loaded plugin's `.aide/_logs/wrapper.log` for the package
root, bundled binary selection, downloaded version, and executed path. A line
such as `Using bundled binary v0.1.18 (plugin v0.1.18)` means an old npm package
was selected before the wrapper ran; it does not show a failed binary download.

If npm and the marketplace disagree, check publication before clearing caches:

```bash
npm view @jmylchreest/aide-plugin dist-tags.latest --prefer-online
npm view @jmylchreest/aide-plugin@0.2.1 version --prefer-online
```

Substitute the expected release for `0.2.1`. Registry failure and an unpublished
version are different from an installed but still-running old process.

## Multiple sessions in one project

The first MCP process owns the stores and serves the project socket. Later
processes attach to it. A newer client does not evict a healthy older primary.
If that primary exits, an existing older client can win the next election.

To complete an upgrade today, stop or disconnect **all aide MCP sessions for the
project**, update their launch configurations, and reconnect them. The first
new process becomes primary. Confirm its version with `instance_info` before
treating the upgrade as complete. Stop a separate `aide serve` owner too if used.
Do not delete the socket or database files to force an upgrade.

On Unix, installing an executable can happen while processes are running: the
installer stages a complete replacement and renames it into place. Existing
processes retain their loaded code. Windows may reject replacement of an in-use
executable; the installer preserves it and reports failure so sessions can be
stopped before retrying. Hook and MCP downloads share an ownership lock, recheck
the installed version after waiting, and do not steal a live installer's lock.

## Release requirements

A release must publish all six binary packages, then the main npm package, and
verify that npm's `latest` serves that version before creating the public GitHub
release. Git tags and marketplace metadata can still appear during the build;
the floating npm channel continues using the preceding published release then.

The 0.2.0 and 0.2.1 release jobs failed while publishing the first binary package
with npm `E404` on a registry `PUT`. GitHub releases had already been published,
while npm remained at 0.1.18. See the
[0.2.1 publishing job](https://github.com/jmylchreest/aide/actions/runs/43713912832/job/132814046138).
This is a publication failure, not evidence that consumers failed to download.
The maintainer confirmed that the npm token had expired and replaced the GitHub
secret. The error alone would not distinguish invalid credentials from package
access restrictions. Replacing the secret does not publish the missing versions;
the release still needs a successful publishing run.

The npm configuration audit on 2026-11-19 found a correct trusted publisher on
the main package, but none on the six binary packages. Those six configurations
were added and all seven were read back to verify the repository, workflow, and
direct publishing permission. This closes the configuration gap that prevented
token-free publication of the binary packages. The [0.2.1 recovery run](https://github.com/jmylchreest/aide/actions/runs/44517222164)
subsequently published all seven packages through OIDC, verified registry visibility
and `latest`, and completed successfully. A fresh npm installation with install
scripts disabled ran the original 0.2.1 binary successfully.

The release workflow uses npm's
[trusted publishing](https://docs.npmjs.com/trusted-publishers/) with GitHub OIDC.
It does not pass `NPM_TOKEN` to npm. Only the publishing job has `id-token: write`;
it runs on a GitHub-hosted runner and serializes publication attempts. Availability
of GitHub's OIDC identity does not prove that npm trusts this workflow; successful
publication without a token fallback verifies the integration.

Before using this workflow, configure the following trusted publisher on **each**
of `@jmylchreest/aide-plugin` and the six `@jmylchreest/aide-binary-` packages
(`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`,
`windows-arm64`):

| npm setting | Value |
| --- | --- |
| Provider | GitHub Actions |
| Organization or user | `jmylchreest` |
| Repository | `aide` |
| Workflow filename | `release.yml` |
| Environment | Leave empty; the publishing job has none |
| Allowed actions | Enable direct `npm publish`; staging alone is insufficient |

New configurations must complete a successful publish within two days or they
expire. Signing provenance alone does not prove publishing access. After checking
all seven trusted publisher configurations, run an actual release through this
workflow. Once all seven packages publish successfully through OIDC, the unused
GitHub `NPM_TOKEN` secret and its npm token can be removed. Keep recovery access
available until OIDC has been verified.

For CLI setup, authenticate interactively with `npm login`, inspect with
`npm trust list <package>`, and add missing configurations with
`npm trust github <package> --repository jmylchreest/aide --file release.yml --allow-publish`.
Do not revoke existing configurations without first checking what uses them.

After setup, verify all seven versions plus the main package's `latest` tag.
To recover an existing stable GitHub release after CI artifacts expire, dispatch
the current workflow with its recovery input:

```bash
gh workflow run release.yml --ref main -f recover_npm_tag=v0.2.1
```

This verifies and reuses the original release assets without rebuilding runtime
code or replacing the tag. Dispatch from the branch containing the workflow fix
if it has not yet merged. Rerunning the original failed run uses its old workflow.
A dry run or `npm whoami` does not validate OIDC publishing access.

### npm 12 and token publishing changes

[npm 12 disables dependency install scripts, Git dependencies, and remote URL
dependencies by default](https://github.blog/changelog/2026-07-08-npm-install-time-security-and-gat-bypass2fa-deprecation/).
The aide npm packages ship prebuilt binaries through registry optional
dependencies and have no consumer `preinstall`, `install`, or `postinstall`
scripts. Source copying happens in the release build before packing. This avoids
requiring consumers to approve a binary-download install script. These npm CLI
defaults do not control Bun's cache, Claude Code's marketplace loader, or a
wrapper's runtime HTTP requests.

[npm plans to remove direct publishing with granular access tokens in January
2027](https://docs.npmjs.com/about-access-tokens/). Replacing an expired token is
therefore a temporary repair. Direct publishing through an authorized OIDC
trusted publisher remains supported and avoids manual token rotation.
Configuring trusted publishers requires interactive authentication with 2FA;
an npm login can still be followed by a separate verification challenge.

[Staged publishing](https://docs.npmjs.com/staged-publishing/) is an alternative
when human approval is wanted for every release. It requires maintainer approval
with 2FA before packages become public. npm's website supports batch approval
where available, but the operations are independent and can partially fail.
Adopting staging would require changing the release workflow to wait for the six
binary packages to become available before exposing the main package. Direct
OIDC publishing fits aide's unattended release workflow.

## Hook timeouts

Hooks use local installed code; their timeout errors are separate from npm
publishing credentials. Check `~/.codex/hooks.json` for the actual command and
time limit. Development hooks can point directly at a checkout even when the
marketplace plugin comes from a different version.

Observation-only handlers (tool tracking, tool recording, HUD updates, usage
catch-up, session summaries, and reflection) run in the background. Both
[Codex](https://learn.chatgpt.com/docs/hooks) and
[Claude Code](https://code.claude.com/docs/en/hooks) support `async: true` command
hooks. The Codex installer migrates existing aide observation entries without
replacing user commands or synchronous policy hooks. Older hosts that do not
support background hooks need a host upgrade.

Analytics emitted inside context-producing hooks also use a separate worker.
Previously, the skill injector synchronously recorded analytics before returning
matched skills, despite the recorder being described as fire-and-forget. OpenCode
queues observation writes, tool statistics, partial memories, idle summaries, and
reflection instead of running their CLI calls on its event loop. Context-window
state and verified-read evidence remain ordered with context transformations,
because later retrieval decisions depend on them.

The queue captures project/session routing, runs one worker at a time per
producer, and bounds pending work to 128 tasks and 8 MiB. Workers have a shared
ten-second command budget and do not inherit host output pipes. This is
best-effort telemetry, not a durable delivery queue: overload, shutdown, or a
backend failure can lose observations. Ambiguous writes are not automatically
retried. Separate host hook processes can complete in a different order; HUD
state is advisory and can lag tool execution.

Synchronous control and context hooks retain a shared subprocess budget as a
backstop. It does not explain or cure backend latency. On this machine, logs
confirmed intermittent state/observe subprocess timeouts; subsequent state
reads took 15–23 ms, and 16 state writes with four concurrent callers took
29–40 ms each without failures. A deliberately stalled-backend test verifies that background
recording can take over five seconds without delaying the host result. That
synthetic test does not identify the cause of the earlier intermittent stalls.
The new worker logs slow command durations without logging argument contents.

Enable aide debug logging to inspect `.aide/_logs/debug.log` and compare the last
completed operation with the hook's host limit. Do not solve repeated stalls by
increasing every hook timeout or by checking npm on every tool call.

## Delivery patterns in comparable tools

| Project or host | Delivery and upgrade behavior | Relevance to aide |
| --- | --- | --- |
| [Context7](https://context7.com/docs/resources/all-clients) | Offers a remote MCP endpoint and local npm launch commands across hosts. | A floating npm launcher is a normal pattern, but depends on successful publication. Its remote service does not have aide's local store ownership problem. |
| [Superpowers](https://github.com/obra/superpowers/blob/main/.opencode/INSTALL.md) | Uses host plugins and a Git-backed OpenCode package; explicitly documents stale lockfile/cache resolutions. | Restarting alone does not prove that new package files were fetched. |
| [Claude Code](https://code.claude.com/docs/en/plugins/loading) | Separates configured, installed, and loaded plugins. Updates on disk need reload or a new session. | Disk upgrades and running code must be diagnosed separately. |
| [OpenCode](https://opencode.ai/docs/plugins/) | Current releases expose explicit plugin check/update commands and distinguish cached loading from update checking. | Check host-version support before changing the existing integration. |
| [claude-mem](https://github.com/thedotmack/claude-mem/issues/3813) | Documents old/new sessions fighting over a shared worker when version mismatch triggers replacement. | A safe live upgrade needs coordinated ownership and compatible clients, not independent restart attempts. |

A coordinated live-upgrade design is described in the repository's `FUTURES.md`.
It is not implemented by `aide upgrade`.
