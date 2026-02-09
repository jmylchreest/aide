#!/bin/bash
# aide-wrapper.sh - Ensures aide binary exists before executing
#
# This wrapper is called by an assistant's MCP server configuration.
# It finds the aide binary, downloads it if missing, then delegates to it.
#
# Plugin root resolution order:
#   1. AIDE_PLUGIN_ROOT  (canonical, platform-agnostic)
#   2. CLAUDE_PLUGIN_ROOT (set by Claude Code)
#   3. SCRIPT_DIR/..      (fallback: infer from wrapper location)
#
# Lives at: <plugin-root>/bin/aide-wrapper.sh
# Binary at: <plugin-root>/bin/aide
#
# Logs written to: .aide/_logs/wrapper.log

set -eo pipefail

# Resolve symlinks so that invoking via node_modules/.bin/aide-wrapper
# (which is a symlink to the real package) gives us the real package dir.
REAL_SCRIPT="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
SCRIPT_DIR="$(cd "$(dirname "$REAL_SCRIPT")" && pwd)"
PLUGIN_ROOT="${AIDE_PLUGIN_ROOT:-${CLAUDE_PLUGIN_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}}"

# Binary extension
EXT=""
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
  EXT=".exe"
fi

BINARY="$PLUGIN_ROOT/bin/aide${EXT}"
BIN_DIR="$PLUGIN_ROOT/bin"

# Setup logging
LOG_DIR="$PLUGIN_ROOT/.aide/_logs"
LOG_FILE="$LOG_DIR/wrapper.log"
mkdir -p "$LOG_DIR" 2>/dev/null || true

log() {
  local timestamp
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  local msg="[$timestamp] [aide-wrapper] $*"
  echo "$msg" >&2
  echo "$msg" >> "$LOG_FILE" 2>/dev/null || true
}

PARENT_PID=$PPID
PARENT_CMD=$(ps -o comm= -p "$PPID" 2>/dev/null || echo "unknown")
PARENT_ARGS=$(ps -o args= -p "$PPID" 2>/dev/null || echo "unknown")
log "Starting wrapper (pid=$$, ppid=$PARENT_PID, parent=$PARENT_CMD, parent_args=$PARENT_ARGS, args=$*)"
log "PLUGIN_ROOT=$PLUGIN_ROOT"
log "BINARY=$BINARY"

# Version comparison: returns 0 (true) if $1 >= $2 (semver base only)
version_gte() {
  local IFS=.
  local a=($1) b=($2)
  for i in 0 1 2; do
    local va=${a[$i]:-0} vb=${b[$i]:-0}
    if (( va > vb )); then return 0; fi
    if (( va < vb )); then return 1; fi
  done
  return 0
}

# Determine if we need to download
NEEDS_DOWNLOAD=false

if [[ ! -x "$BINARY" ]]; then
  NEEDS_DOWNLOAD=true
  log "Binary not found or not executable"
else
  # Check if this is a dev build that should be preserved
  BINARY_VERSION=$("$BINARY" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.+]+)?' | head -1 || true)
  log "Binary version: ${BINARY_VERSION:-unknown}"

  if [[ -n "$BINARY_VERSION" && "$BINARY_VERSION" == *"-dev."* ]]; then
    # Dev build — check base version against plugin version
    BASE_VERSION="${BINARY_VERSION%%-*}"
    # Read plugin version: try .claude-plugin/plugin.json (dev), fall back to package.json (npm)
    PLUGIN_VERSION=$(grep -oP '"version"\s*:\s*"\K[0-9]+\.[0-9]+\.[0-9]+' "$PLUGIN_ROOT/.claude-plugin/plugin.json" 2>/dev/null \
      || grep -oP '"version"\s*:\s*"\K[0-9]+\.[0-9]+\.[0-9]+' "$PLUGIN_ROOT/package.json" 2>/dev/null \
      || true)

    if [[ -n "$PLUGIN_VERSION" ]] && version_gte "$BASE_VERSION" "$PLUGIN_VERSION"; then
      log "Dev build v$BINARY_VERSION (base $BASE_VERSION >= plugin v$PLUGIN_VERSION), using local build"
    else
      NEEDS_DOWNLOAD=true
      log "Dev build v$BINARY_VERSION is older than plugin v${PLUGIN_VERSION:-unknown}, re-downloading"
    fi
  else
    log "Release binary v${BINARY_VERSION:-unknown}"
  fi
fi

if [[ "$NEEDS_DOWNLOAD" == "true" ]]; then
  LOCKFILE="$BIN_DIR/.aide-download.lock"

  # Resolve the downloader script — prefer src/ (dev) but fall back to dist/ (npm install)
  if [[ -f "$PLUGIN_ROOT/src/lib/aide-downloader.ts" ]]; then
    DOWNLOADER="$PLUGIN_ROOT/src/lib/aide-downloader.ts"
  elif [[ -f "$PLUGIN_ROOT/dist/lib/aide-downloader.js" ]]; then
    DOWNLOADER="$PLUGIN_ROOT/dist/lib/aide-downloader.js"
  else
    log "ERROR: Cannot find aide-downloader in src/ or dist/ under $PLUGIN_ROOT"
    exit 1
  fi

  # Remove the stale/outdated binary before entering the lock so that
  # the re-check inside the critical section can distinguish between
  # "no binary yet" and "a concurrent process just downloaded the right one".
  if [[ -f "$BINARY" ]]; then
    log "Removing outdated binary before download"
    rm -f "$BINARY" 2>/dev/null || true
  fi

  # Use flock to ensure only one process downloads at a time.
  # If another process holds the lock, we wait for it to finish.
  (
    flock -w 60 9 || { log "ERROR: Timed out waiting for download lock"; exit 1; }

    # Re-check after acquiring lock — another process may have finished the download
    if [[ -x "$BINARY" ]]; then
      log "Binary appeared while waiting for lock (downloaded by another process)"
    else
      log "Downloading binary..."
      log "Using downloader: $DOWNLOADER"

      # Use tsx for .ts files (dev), node for .js files (npm install)
      if [[ "$DOWNLOADER" == *.ts ]]; then
        RUNNER="npx --yes tsx"
      else
        RUNNER="node"
      fi

      if ! $RUNNER "$DOWNLOADER" --dest "$BIN_DIR" 2>&1 | tee -a "$LOG_FILE" >&2; then
        log "ERROR: Downloader failed"
        exit 1
      fi

      if [[ ! -x "$BINARY" ]]; then
        log "ERROR: Binary not found after download"
        exit 1
      fi
    fi
  ) 9>"$LOCKFILE"

  rm -f "$LOCKFILE"
  log "Binary ready at $BINARY"
fi

log "Executing: $BINARY $*"
exec "$BINARY" "$@"
