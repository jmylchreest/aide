---
title: Privacy Policy
description: "How AIDE handles your data — no telemetry, all state local to your machine."
---

# Privacy Policy

_Last updated: 2026-09-03_

AIDE is a locally-executed developer tool. It has no backend service, no user
accounts, and no analytics. This policy describes what AIDE does with data on
your machine and the only network calls it makes.

## What AIDE collects

**Nothing is collected by the author.** There is no telemetry, usage reporting,
crash reporting, or analytics of any kind. No data is transmitted to
`jmylchreest` or to any service operated on AIDE's behalf.

## What AIDE stores, and where

AIDE writes all of its state to your own filesystem:

- **Project state** — memories, decisions, tasks, findings, code index, and
  session events are stored in the `.aide/` directory inside each project.
- **User state** — configuration and downloaded binaries and grammars are stored
  under your user config and cache directories.

This data stays on your machine. It is yours to inspect, edit, or delete at any
time — removing the `.aide/` directory removes all project state. Because
`.aide/` may contain excerpts of your source code and session activity, treat it
with the same care as the repository itself and keep it out of version control
unless you intend to share it.

## Network requests AIDE makes

AIDE makes no network request other than these:

| Destination | Purpose | Data sent |
| --- | --- | --- |
| `api.github.com`, `github.com` | Check for and download AIDE release binaries and Tree-sitter grammars | Standard HTTP request metadata only |
| `api.anthropic.com/api/oauth/usage` | Read **your own** plan usage to display it in the status dashboard | Your existing Claude credential, supplied by your assistant |

The Anthropic usage request is a read of your own account, authenticated with
the credential your AI assistant already holds. AIDE does not create, store, or
transmit credentials of its own.

## Your AI assistant is separate

AIDE runs as a plugin inside Claude Code, OpenCode, or Codex. Those tools send
your prompts and code to their respective model providers under their own terms.
AIDE does not control or intercept that, and this policy does not cover it —
see the privacy policy of the assistant and model provider you use.

## This documentation site

These docs are published with GitHub Pages, which is subject to
[GitHub's Privacy Statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).
The site itself sets no tracking cookies and embeds no analytics.

## Changes

Changes to this policy are made in the
[AIDE repository](https://github.com/jmylchreest/aide) and carry the commit
history as their record.

## Contact

Questions or concerns: open an issue at
[github.com/jmylchreest/aide/issues](https://github.com/jmylchreest/aide/issues).
For security reports, follow [SECURITY.md](https://github.com/jmylchreest/aide/blob/main/SECURITY.md).
