#!/usr/bin/env bash
# aide-dev-toggle.sh — Switch aide between local dev and published/marketplace mode
#
# Supports both OpenCode and Claude Code simultaneously.
#
# Usage:
#   ./aide-dev-toggle.sh          Toggle between dev <-> prod
#   ./aide-dev-toggle.sh dev      Switch to dev mode (build + local paths)
#   ./aide-dev-toggle.sh prod     Switch to prod mode (npm/marketplace)
#   ./aide-dev-toggle.sh status   Show current mode
#
# Dev mode:
#   OpenCode:
#     - Builds the Go binary to bin/aide with correct LDFLAGS
#     - Points opencode.json plugin at src/opencode/index.ts
#     - Points opencode.json MCP at bin/aide-wrapper.sh
#   Claude Code:
#     - Builds the Go binary to bin/aide with correct LDFLAGS
#     - Patches installed_plugins.json installPath to this repo
#     - Symlinks .aide/bin/aide to local build
#
# Prod mode:
#   OpenCode:
#     - Points opencode.json plugin at @jmylchreest/aide-plugin
#     - Points opencode.json MCP at bunx @jmylchreest/aide-plugin mcp
#   Claude Code:
#     - Restores installed_plugins.json installPath to cached marketplace version
#     - Removes local .aide/bin/aide symlink (wrapper will re-download)
#
# Note: A Claude Code marketplace upgrade will overwrite the dev installPath,
# effectively reverting to prod. Re-run this script to switch back to dev.

set -euo pipefail

# Resolve both logical and physical paths for detection across symlinks/bind mounts
REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT_PHYS="$(cd "$(dirname "$0")" && pwd -P)"

# --- OpenCode config paths ---
OC_GLOBAL_CONFIG="$HOME/.config/opencode/opencode.json"
OC_PROJECT_CONFIG="$REPO_ROOT/opencode.json"
NPM_PACKAGE="@jmylchreest/aide-plugin"
DEV_PLUGIN="$REPO_ROOT/src/opencode/index.ts"
DEV_MCP_CMD="$REPO_ROOT/bin/aide-wrapper.sh"

# --- Claude Code config paths ---
CC_PLUGINS_DIR="$HOME/.claude/plugins"
CC_INSTALLED="$CC_PLUGINS_DIR/installed_plugins.json"
CC_PLUGIN_KEY="aide@aide"
CC_MCP_JSON="$REPO_ROOT/.mcp.json"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}[info]${NC}  $*"; }
ok()    { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
err()   { echo -e "${RED}[err]${NC}   $*" >&2; }

# --------------------------------------------------------------------------
# OpenCode: Detect current mode from a config file
# --------------------------------------------------------------------------
oc_detect_mode() {
    local cfg="$1"
    if [[ ! -f "$cfg" ]]; then
        echo "unknown"
        return
    fi
    if grep -q "src/opencode/index.ts" "$cfg" 2>/dev/null; then
        echo "dev"
    elif grep -q "$NPM_PACKAGE" "$cfg" 2>/dev/null; then
        echo "prod"
    else
        echo "unknown"
    fi
}

# --------------------------------------------------------------------------
# Claude Code: Detect current mode from installed_plugins.json
# --------------------------------------------------------------------------
cc_detect_mode() {
    if [[ ! -f "$CC_INSTALLED" ]]; then
        echo "not-installed"
        return
    fi

    local install_path
    install_path=$(python3 -c "
import json, sys
try:
    d = json.load(open('$CC_INSTALLED'))
    entries = d.get('plugins', {}).get('$CC_PLUGIN_KEY', [])
    if entries:
        print(entries[0].get('installPath', ''))
except Exception:
    pass
" 2>/dev/null)

    if [[ -z "$install_path" ]]; then
        echo "not-installed"
    elif [[ "$install_path" == "$REPO_ROOT" || "$install_path" == "$REPO_ROOT_PHYS" ]]; then
        echo "dev"
    elif [[ "$install_path" == *"plugins/cache"* ]]; then
        echo "prod"
    else
        echo "unknown"
    fi
}

# --------------------------------------------------------------------------
# Claude Code: Get the latest cached (prod) install path
# --------------------------------------------------------------------------
cc_get_prod_path() {
    python3 -c "
import json, os, sys

installed_path = '$CC_INSTALLED'
plugin_key = '$CC_PLUGIN_KEY'
cache_base = '$CC_PLUGINS_DIR/cache/aide/aide'

# First check if there is a non-dev installPath already stored
try:
    d = json.load(open(installed_path))
    entries = d.get('plugins', {}).get(plugin_key, [])
    if entries:
        p = entries[0].get('installPath', '')
        if 'plugins/cache' in p and os.path.isdir(p):
            print(p)
            sys.exit(0)
except Exception:
    pass

# Fall back: find the latest versioned dir in cache
if os.path.isdir(cache_base):
    versions = sorted(
        [v for v in os.listdir(cache_base) if os.path.isdir(os.path.join(cache_base, v))],
        key=lambda v: [int(x) if x.isdigit() else 0 for x in v.split('.')],
        reverse=True,
    )
    if versions:
        print(os.path.join(cache_base, versions[0]))
        sys.exit(0)

sys.exit(1)
" 2>/dev/null
}

# --------------------------------------------------------------------------
# Build the Go binary
# --------------------------------------------------------------------------
build_binary() {
    info "Building aide binary..."
    if ! make -C "$REPO_ROOT" build 2>&1; then
        err "Go build failed"
        return 1
    fi

    if [[ ! -x "$REPO_ROOT/bin/aide" ]]; then
        err "Binary not found at bin/aide after build"
        return 1
    fi

    local version
    version=$("$REPO_ROOT/bin/aide" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.+]+)?' | head -1 || echo "unknown")
    ok "Built bin/aide ${BOLD}v${version}${NC}"

    # Update .aide/bin/aide symlink so hooks find the local build
    mkdir -p "$REPO_ROOT/.aide/bin"
    ln -sf "$REPO_ROOT/bin/aide" "$REPO_ROOT/.aide/bin/aide"
    ok "Symlinked .aide/bin/aide -> bin/aide"
}

# --------------------------------------------------------------------------
# OpenCode: Patch a config file between dev and prod
# --------------------------------------------------------------------------
oc_patch_config() {
    local cfg="$1"
    local target_mode="$2"
    local label="$3"

    if [[ ! -f "$cfg" ]]; then
        warn "Config not found: $cfg — skipping"
        return
    fi

    local current
    current=$(oc_detect_mode "$cfg")

    if [[ "$current" == "$target_mode" ]]; then
        ok "$label already in ${BOLD}$target_mode${NC} mode"
        return
    fi

    # Create backup
    cp "$cfg" "${cfg}.bak"

    if [[ "$target_mode" == "dev" ]]; then
        python3 -c "
import json

cfg_path = '$cfg'
dev_plugin = '$DEV_PLUGIN'
dev_mcp = '$DEV_MCP_CMD'

with open(cfg_path, 'r') as f:
    cfg = json.load(f)

# Plugin: replace any @jmylchreest/aide-plugin or old local path with new dev path
if 'plugin' in cfg:
    cfg['plugin'] = [
        dev_plugin if ('aide-plugin' in p or 'src/opencode/index.ts' in p) else p
        for p in cfg['plugin']
    ]

# MCP command: use local wrapper
mcp_aide = cfg.get('mcp', {}).get('aide', {})
if mcp_aide:
    mcp_aide['command'] = [dev_mcp, 'mcp']

with open(cfg_path, 'w') as f:
    json.dump(cfg, f, indent=2)
    f.write('\n')
"
        ok "$label -> ${BOLD}dev${NC} mode"
    else
        python3 -c "
import json

cfg_path = '$cfg'
npm_pkg = '$NPM_PACKAGE'

with open(cfg_path, 'r') as f:
    cfg = json.load(f)

# Plugin: replace any local path with npm package
if 'plugin' in cfg:
    cfg['plugin'] = [
        npm_pkg if ('aide-plugin' in p or 'src/opencode/index.ts' in p) else p
        for p in cfg['plugin']
    ]

# MCP command: use npx
mcp_aide = cfg.get('mcp', {}).get('aide', {})
if mcp_aide:
    mcp_aide['command'] = ['npx', '-y', npm_pkg, 'mcp']

with open(cfg_path, 'w') as f:
    json.dump(cfg, f, indent=2)
    f.write('\n')
"
        ok "$label -> ${BOLD}prod${NC} mode"
    fi
}

# --------------------------------------------------------------------------
# Claude Code: Patch installed_plugins.json between dev and prod
# --------------------------------------------------------------------------
cc_patch_plugins() {
    local target_mode="$1"

    if [[ ! -f "$CC_INSTALLED" ]]; then
        warn "Claude Code installed_plugins.json not found — skipping"
        return
    fi

    local current
    current=$(cc_detect_mode)

    if [[ "$current" == "not-installed" ]]; then
        warn "aide plugin not found in Claude Code — skipping"
        return
    fi

    if [[ "$current" == "$target_mode" ]]; then
        ok "Claude Code plugin already in ${BOLD}$target_mode${NC} mode"
        return
    fi

    # Create backup
    cp "$CC_INSTALLED" "${CC_INSTALLED}.bak"

    if [[ "$target_mode" == "dev" ]]; then
        # Save the current prod path for later restoration
        local prod_path
        prod_path=$(cc_get_prod_path) || true
        if [[ -n "$prod_path" ]]; then
            echo "$prod_path" > "$REPO_ROOT/.claude-prod-path"
        fi

        python3 -c "
import json

installed_path = '$CC_INSTALLED'
plugin_key = '$CC_PLUGIN_KEY'
dev_path = '$REPO_ROOT'

with open(installed_path, 'r') as f:
    d = json.load(f)

entries = d.get('plugins', {}).get(plugin_key, [])
if entries:
    entries[0]['installPath'] = dev_path

with open(installed_path, 'w') as f:
    json.dump(d, f, indent=2)
    f.write('\n')
"
        ok "Claude Code plugin -> ${BOLD}dev${NC} mode (installPath -> repo)"
    else
        # Restore original prod path
        local prod_path=""
        if [[ -f "$REPO_ROOT/.claude-prod-path" ]]; then
            prod_path=$(cat "$REPO_ROOT/.claude-prod-path")
        fi
        if [[ -z "$prod_path" ]] || [[ ! -d "$prod_path" ]]; then
            prod_path=$(cc_get_prod_path) || true
        fi
        if [[ -z "$prod_path" ]]; then
            err "Cannot determine Claude Code prod install path"
            return 1
        fi

        python3 -c "
import json

installed_path = '$CC_INSTALLED'
plugin_key = '$CC_PLUGIN_KEY'
prod_path = '$prod_path'

with open(installed_path, 'r') as f:
    d = json.load(f)

entries = d.get('plugins', {}).get(plugin_key, [])
if entries:
    entries[0]['installPath'] = prod_path

with open(installed_path, 'w') as f:
    json.dump(d, f, indent=2)
    f.write('\n')
"
        ok "Claude Code plugin -> ${BOLD}prod${NC} mode (installPath -> cache)"
        rm -f "$REPO_ROOT/.claude-prod-path"
    fi
}

# --------------------------------------------------------------------------
# Claude Code: Patch .mcp.json between dev and prod
# --------------------------------------------------------------------------
cc_patch_mcp() {
    local target_mode="$1"

    if [[ ! -f "$CC_MCP_JSON" ]]; then
        warn ".mcp.json not found — skipping"
        return
    fi

    if [[ "$target_mode" == "dev" ]]; then
        python3 -c "
import json

mcp_path = '$CC_MCP_JSON'
dev_cmd = '$DEV_MCP_CMD'

with open(mcp_path, 'r') as f:
    cfg = json.load(f)

aide = cfg.get('mcpServers', {}).get('aide', {})
if aide:
    aide['command'] = dev_cmd
    aide['args'] = ['mcp']

with open(mcp_path, 'w') as f:
    json.dump(cfg, f, indent=2)
    f.write('\n')
"
        ok ".mcp.json -> ${BOLD}dev${NC} mode"
    else
        python3 -c "
import json

mcp_path = '$CC_MCP_JSON'
npm_pkg = '$NPM_PACKAGE'

with open(mcp_path, 'r') as f:
    cfg = json.load(f)

aide = cfg.get('mcpServers', {}).get('aide', {})
if aide:
    aide['command'] = 'npx'
    aide['args'] = ['-y', npm_pkg, 'mcp']

with open(mcp_path, 'w') as f:
    json.dump(cfg, f, indent=2)
    f.write('\n')
"
        ok ".mcp.json -> ${BOLD}prod${NC} mode"
    fi
}

# --------------------------------------------------------------------------
# Status
# --------------------------------------------------------------------------
show_status() {
    echo ""
    echo -e "${BOLD}aide dev mode status${NC}"

    # --- OpenCode ---
    echo ""
    echo -e "  ${BOLD}OpenCode${NC}"
    for cfg_path in "$OC_GLOBAL_CONFIG" "$OC_PROJECT_CONFIG"; do
        local label
        if [[ "$cfg_path" == "$OC_GLOBAL_CONFIG" ]]; then
            label="Global  (~/.config/opencode/opencode.json)"
        else
            label="Project (opencode.json)"
        fi

        if [[ ! -f "$cfg_path" ]]; then
            echo -e "    $label: ${YELLOW}not found${NC}"
            continue
        fi

        local mode
        mode=$(oc_detect_mode "$cfg_path")
        case "$mode" in
            dev)  echo -e "    $label: ${GREEN}dev${NC}" ;;
            prod) echo -e "    $label: ${CYAN}prod${NC}" ;;
            *)    echo -e "    $label: ${YELLOW}unknown${NC}" ;;
        esac
    done

    # --- Claude Code ---
    echo ""
    echo -e "  ${BOLD}Claude Code${NC}"
    local cc_mode
    cc_mode=$(cc_detect_mode)
    case "$cc_mode" in
        dev)           echo -e "    Plugin:  ${GREEN}dev${NC} (installPath -> repo)" ;;
        prod)          echo -e "    Plugin:  ${CYAN}prod${NC} (installPath -> cache)" ;;
        not-installed) echo -e "    Plugin:  ${YELLOW}not installed${NC}" ;;
        *)             echo -e "    Plugin:  ${YELLOW}unknown${NC}" ;;
    esac

    if [[ -f "$CC_MCP_JSON" ]]; then
        if grep -q "$REPO_ROOT" "$CC_MCP_JSON" 2>/dev/null || grep -q "aide-wrapper" "$CC_MCP_JSON" 2>/dev/null; then
            echo -e "    .mcp.json: ${GREEN}dev${NC}"
        elif grep -q "npx" "$CC_MCP_JSON" 2>/dev/null; then
            echo -e "    .mcp.json: ${CYAN}prod${NC}"
        else
            echo -e "    .mcp.json: ${YELLOW}unknown${NC}"
        fi
    fi

    # --- Shared ---
    echo ""
    echo -e "  ${BOLD}Shared${NC}"

    if [[ -x "$REPO_ROOT/bin/aide" ]]; then
        local version
        version=$("$REPO_ROOT/bin/aide" version 2>/dev/null | head -1 || echo "unknown")
        echo -e "    Binary:   ${GREEN}$version${NC}"
    else
        echo -e "    Binary:   ${YELLOW}not built${NC}"
    fi

    if [[ -L "$REPO_ROOT/.aide/bin/aide" ]]; then
        local target
        target=$(readlink "$REPO_ROOT/.aide/bin/aide" 2>/dev/null || echo "?")
        echo -e "    Symlink:  .aide/bin/aide -> $target"
    fi

    echo ""
}

# --------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------
ACTION="${1:-}"

# Determine target mode
case "$ACTION" in
    dev)
        TARGET="dev"
        ;;
    prod|package|npm)
        TARGET="prod"
        ;;
    status|info)
        show_status
        exit 0
        ;;
    "")
        # No argument: toggle — but only if both platforms agree on current state
        oc_mode=$(oc_detect_mode "$OC_GLOBAL_CONFIG")
        cc_mode=$(cc_detect_mode)

        # Normalise: treat not-installed/unknown as "ignore" for consensus
        _oc="$oc_mode"; _cc="$cc_mode"
        [[ "$_oc" == "unknown" ]] && _oc=""
        [[ "$_cc" == "unknown" || "$_cc" == "not-installed" ]] && _cc=""

        if [[ -n "$_oc" && -n "$_cc" && "$_oc" != "$_cc" ]]; then
            err "OpenCode is ${BOLD}$oc_mode${NC} but Claude Code is ${BOLD}$cc_mode${NC}"
            err "Use an explicit target to resolve: $0 dev  OR  $0 prod"
            echo ""
            show_status
            exit 1
        fi

        # Use whichever has a known state (or both agree)
        current="${_oc:-$_cc}"
        if [[ "$current" == "dev" ]]; then
            TARGET="prod"
        else
            TARGET="dev"
        fi
        ;;
    *)
        echo "Usage: $0 [dev|prod|status]"
        exit 1
        ;;
esac

echo ""
echo -e "${BOLD}Switching to ${TARGET} mode${NC}"
echo ""

# Build if switching to dev
if [[ "$TARGET" == "dev" ]]; then
    build_binary
    echo ""
fi

# --- OpenCode ---
echo -e "${BOLD}OpenCode${NC}"
oc_patch_config "$OC_GLOBAL_CONFIG"  "$TARGET" "Global config"
oc_patch_config "$OC_PROJECT_CONFIG" "$TARGET" "Project config"
echo ""

# --- Claude Code ---
echo -e "${BOLD}Claude Code${NC}"
cc_patch_plugins "$TARGET"
cc_patch_mcp "$TARGET"

# Clean up .aide/bin/aide symlink when going to prod
if [[ "$TARGET" == "prod" ]]; then
    if [[ -L "$REPO_ROOT/.aide/bin/aide" ]]; then
        local_target=$(readlink "$REPO_ROOT/.aide/bin/aide" 2>/dev/null || echo "")
        if [[ "$local_target" == "$REPO_ROOT/bin/aide" ]]; then
            rm -f "$REPO_ROOT/.aide/bin/aide"
            ok "Removed local .aide/bin/aide symlink (wrapper will re-download)"
        fi
    fi
fi

echo ""
show_status

if [[ "$TARGET" == "dev" ]]; then
    info "Restart OpenCode and/or Claude Code to pick up changes"
else
    info "Restart OpenCode and/or Claude Code to pick up changes"
    info "Run ${BOLD}./aide-dev-toggle.sh dev${NC} to switch back"
fi
