#!/bin/bash
# aide-wrapper.sh - Ensures aide binary exists before executing
#
# This wrapper is called by Claude Code's MCP server configuration.
# It downloads the aide binary if missing, then delegates to it.
#
# Lives at: <repo>/bin/aide-wrapper.sh
# CLAUDE_PLUGIN_ROOT points to <repo>/ (the project root)
#
# Logs written to: .aide/_logs/wrapper.log

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(dirname "$SCRIPT_DIR")}"
PROJECT_ROOT="$PLUGIN_ROOT"
BINARY="$SCRIPT_DIR/aide"

# Setup logging
LOG_DIR="$PROJECT_ROOT/.aide/_logs"
LOG_FILE="$LOG_DIR/wrapper.log"

# Ensure log directory exists
mkdir -p "$LOG_DIR" 2>/dev/null || true

# Log function - writes to both stderr and log file
log() {
  local timestamp
  timestamp="$(date '+%Y-%m-%d %H:%M:%S')"
  local msg="[$timestamp] [aide-wrapper] $*"
  echo "$msg" >&2
  echo "$msg" >> "$LOG_FILE" 2>/dev/null || true
}

# Log startup
log "Starting wrapper (pid=$$, args=$*)"
log "SCRIPT_DIR=$SCRIPT_DIR"
log "PLUGIN_ROOT=$PLUGIN_ROOT"
log "PROJECT_ROOT=$PROJECT_ROOT"
log "BINARY=$BINARY"

# Add .exe extension on Windows
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
  BINARY="$SCRIPT_DIR/aide.exe"
  log "Windows detected, using $BINARY"
fi

# Download if binary doesn't exist or isn't executable
if [[ ! -x "$BINARY" ]]; then
  log "Binary not found or not executable, downloading..."

  DOWNLOADER="$PLUGIN_ROOT/src/lib/aide-downloader.ts"
  log "Using downloader: $DOWNLOADER"

  if ! npx --yes tsx "$DOWNLOADER" --dest "$SCRIPT_DIR" 2>&1 | tee -a "$LOG_FILE" >&2; then
    log "ERROR: Downloader failed with exit code $?"
    exit 1
  fi

  if [[ ! -x "$BINARY" ]]; then
    # Fallback to PATH (e.g., system-wide install)
    if command -v aide &>/dev/null; then
      BINARY="$(command -v aide)"
      log "Download failed, using aide from PATH: $BINARY"
    else
      log "ERROR: Binary not found after download and not in PATH"
      exit 1
    fi
  else
    log "Binary ready at $BINARY"
  fi
else
  log "Binary exists and is executable"
fi

# Log and execute the real binary with all arguments
log "Executing: $BINARY $*"
exec "$BINARY" "$@"
