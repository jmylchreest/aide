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
SESSION_FILE="$PROJECT_ROOT/.aide/state/session.json"

# Read HUD content if exists (already includes [aide(version)] prefix)
if [[ -f "$HUD_FILE" ]]; then
    HUD_CONTENT="$(cat "$HUD_FILE" 2>/dev/null)"
    if [[ -n "$HUD_CONTENT" ]]; then
        echo "$HUD_CONTENT"
        exit 0
    fi
fi

# Fallback: build minimal status from session state
if [[ -f "$SESSION_FILE" ]]; then
    # Extract basic info with simple parsing
    MODE=$(grep -o '"activeMode"[[:space:]]*:[[:space:]]*"[^"]*"' "$SESSION_FILE" 2>/dev/null | cut -d'"' -f4)
    MODEL=$(grep -o '"modelTier"[[:space:]]*:[[:space:]]*"[^"]*"' "$SESSION_FILE" 2>/dev/null | cut -d'"' -f4)
    AGENTS=$(grep -o '"agentCount"[[:space:]]*:[[:space:]]*[0-9]*' "$SESSION_FILE" 2>/dev/null | grep -o '[0-9]*$')
    TASKS=$(grep -o '"taskCount"[[:space:]]*:[[:space:]]*[0-9]*' "$SESSION_FILE" 2>/dev/null | grep -o '[0-9]*$')
    TOOLS=$(grep -o '"toolCalls"[[:space:]]*:[[:space:]]*[0-9]*' "$SESSION_FILE" 2>/dev/null | grep -o '[0-9]*$')
    STARTED_AT=$(grep -o '"startedAt"[[:space:]]*:[[:space:]]*"[^"]*"' "$SESSION_FILE" 2>/dev/null | cut -d'"' -f4)

    # Calculate duration
    DURATION=""
    if [[ -n "$STARTED_AT" ]]; then
        START_EPOCH=$(date -d "$STARTED_AT" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${STARTED_AT%%.*}" +%s 2>/dev/null)
        if [[ -n "$START_EPOCH" ]]; then
            NOW_EPOCH=$(date +%s)
            DIFF_SECS=$((NOW_EPOCH - START_EPOCH))
            if [[ $DIFF_SECS -ge 3600 ]]; then
                HOURS=$((DIFF_SECS / 3600))
                MINS=$(((DIFF_SECS % 3600) / 60))
                DURATION="${HOURS}h${MINS}m"
            elif [[ $DIFF_SECS -ge 60 ]]; then
                MINS=$((DIFF_SECS / 60))
                DURATION="${MINS}m"
            else
                DURATION="${DIFF_SECS}s"
            fi
        fi
    fi

    STATUS="[aide]"
    # Show mode if active, otherwise show "idle"
    if [[ -n "$MODE" ]]; then
        STATUS="$STATUS mode:$MODE"
    else
        STATUS="$STATUS idle"
    fi
    # Show model tier if available
    [[ -n "$MODEL" ]] && STATUS="$STATUS | $MODEL"
    # Show duration
    [[ -n "$DURATION" ]] && STATUS="$STATUS | $DURATION"
    # Show tool calls
    [[ -n "$TOOLS" && "$TOOLS" != "0" ]] && STATUS="$STATUS | tools:$TOOLS"
    [[ -n "$AGENTS" && "$AGENTS" != "0" ]] && STATUS="$STATUS | agents:$AGENTS"
    [[ -n "$TASKS" && "$TASKS" != "0" ]] && STATUS="$STATUS | tasks:$TASKS"

    echo "$STATUS"
    exit 0
fi

# No state at all - show minimal status
echo "[aide] idle"
