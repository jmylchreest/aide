#!/bin/bash
# aide-wrapper.sh - Ensures aide binary exists before executing
#
# This wrapper is called by Claude Code's MCP server configuration.
# It finds the aide binary, downloads it if missing, then delegates to it.
#
# CLAUDE_PLUGIN_ROOT = project root, so binary lives at $PLUGIN_ROOT/bin/aide
#
# Logs written to: .aide/_logs/wrapper.log

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"

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

log "Starting wrapper (pid=$$, args=$*)"
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
    PLUGIN_VERSION=$(grep -oP '"version"\s*:\s*"\K[0-9]+\.[0-9]+\.[0-9]+' "$PLUGIN_ROOT/.claude-plugin/plugin.json" 2>/dev/null || true)

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
  log "Downloading binary..."

  DOWNLOADER="$PLUGIN_ROOT/src/lib/aide-downloader.ts"
  log "Using downloader: $DOWNLOADER"

  if ! npx --yes tsx "$DOWNLOADER" --dest "$BIN_DIR" 2>&1 | tee -a "$LOG_FILE" >&2; then
    log "ERROR: Downloader failed"
    exit 1
  fi

  if [[ ! -x "$BINARY" ]]; then
    log "ERROR: Binary not found after download"
    exit 1
  fi

  log "Binary ready at $BINARY"
fi

log "Executing: $BINARY $*"
exec "$BINARY" "$@"
