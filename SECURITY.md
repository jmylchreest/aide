# Security Policy

## Supported Versions

Only the latest released version of aide is supported. Fixes land on `main`
and ship in the next release.

## Reporting a Vulnerability

Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/jmylchreest/aide/security/advisories/new).
Please do not open a public issue for an unfixed vulnerability.

Include where you can:

- affected version or commit
- reproduction steps or a proof of concept
- impact you believe the issue has

You can expect an initial response within seven days. Once a fix is available
it is released and the advisory is published with credit unless you ask
otherwise.

## Scope

aide runs locally as a Claude Code and OpenCode plugin: it executes hooks,
spawns the `aide` binary, and stores project state under `.aide/`. Reports
about code execution, data exposure outside the project directory, or the
plugin's network fetches (binary downloads, grammar downloads) are in scope.
