#!/usr/bin/env bash
# Guard against new inline project-root resolvers.
#
# The Go binary's resolveAnchor (aide/cmd/aide/cmd_anchor.go) is the single
# resolution authority; the TS mirror lives in src/lib/project-root.ts and
# the anchor reader in src/lib/anchor.ts. History shows every "quick inline
# walk" drifts (session-end's stale fork, the HUD's .aide-only walk, the
# OpenCode bypass) — so new resolution logic outside the allowlist fails CI.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# TS: probing for VCS/aide markers outside the blessed resolver files.
# session-end.ts keeps a documented inline FALLBACK (no-ES-imports startup
# constraint); aide-hud.ts keeps a minimal .aide walk as its last rung.
ts_allow='src/lib/project-root\.ts|src/lib/anchor\.ts|src/hooks/session-end\.ts|scripts/aide-hud\.ts|src/test/|packages/'
# Match probes where the marker is the FINAL path segment (a resolution
# probe), not paths beneath .aide/ like join(root, ".aide", "state").
hits=$(grep -rnE 'existsSync\(join\([^)]*["'"'"']\.(git|aide)["'"'"']\s*\)\)' src/ scripts/ --include='*.ts' 2>/dev/null \
  | grep -vE "$ts_allow" || true)
if [ -n "$hits" ]; then
  echo "ERROR: marker probe outside the resolver allowlist (use findProjectRoot/getAnchoredRoot):"
  echo "$hits"
  fail=1
fi

# Go: os.Getwd outside the blessed files (resolution + explicitly reviewed
# uses: cmd_session's cwd-vs-root validation, instance-info display).
go_allow='cmd_anchor\.go|main\.go|_test\.go|cmd_session\.go|cmd_mcp_instance\.go'
hits=$(grep -rn 'os\.Getwd()' aide/cmd/aide/ --include='*.go' 2>/dev/null \
  | grep -vE "$go_allow" || true)
if [ -n "$hits" ]; then
  echo "ERROR: os.Getwd() outside the resolver (derive from the resolved root/dbPath):"
  echo "$hits"
  fail=1
fi

# Go: the triple-Dir dbPath inversion must go through projectRoot()/scope
# helpers, not fresh textual copies.
hits=$(grep -rn 'Dir(filepath\.Dir(filepath\.Dir' aide/ --include='*.go' 2>/dev/null \
  | grep -v '_test\.go' \
  | grep -vE 'cmd/aide/helpers\.go|pkg/grpcapi/server\.go|cmd/aide/cmd_init\.go' || true)
if [ -n "$hits" ]; then
  echo "ERROR: new triple-Dir dbPath inversion (use projectRoot(dbPath)):"
  echo "$hits"
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "resolver guards OK"
