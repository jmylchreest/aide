#!/bin/bash
# aide-hud-wrapper.sh - Installed to ~/.claude/bin/aide-hud.sh
#
# This is a thin wrapper that delegates to the real HUD script in the aide plugin.
# This allows the plugin to update without requiring users to reinstall the wrapper.

# Find the newest aide plugin installation and call its HUD script
SCRIPT=$(find ~/.claude/plugins/cache -path "*/aide/*/scripts/aide-hud.sh" -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)

if [[ -n "$SCRIPT" && -f "$SCRIPT" ]]; then
    exec bash "$SCRIPT" "$@"
else
    echo "[aide] not installed"
fi
