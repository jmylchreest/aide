#!/bin/bash
# aide-hud.sh - Status line display for aide plugin
#
# Reads HUD state from .aide/state/hud.txt and outputs formatted status.
# Falls back to minimal display if state doesn't exist.

# Find project root (look for .aide directory)
find_project_root() {
    local dir="$PWD"
    while [[ "$dir" != "/" ]]; do
        if [[ -d "$dir/.aide" ]]; then
            echo "$dir"
            return 0
        fi
        dir="$(dirname "$dir")"
    done
    echo "$PWD"
}

PROJECT_ROOT="$(find_project_root)"
HUD_FILE="$PROJECT_ROOT/.aide/state/hud.txt"

# Read HUD content if exists (already includes [aide(version)] prefix)
if [[ -f "$HUD_FILE" ]]; then
    HUD_CONTENT="$(cat "$HUD_FILE" 2>/dev/null)"
    if [[ -n "$HUD_CONTENT" ]]; then
        echo "$HUD_CONTENT"
        exit 0
    fi
fi

# No state at all - show minimal status
echo "[aide] idle"
