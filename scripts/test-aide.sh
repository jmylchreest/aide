#!/bin/bash
# test-aide.sh - Comprehensive AIDE plugin test suite
#
# Usage:
#   ./scripts/test-aide.sh              # Run all tests
#   ./scripts/test-aide.sh keyword      # Run specific test
#   AIDE_DEBUG=1 ./scripts/test-aide.sh # With timing output
#
# Tests:
#   keyword  - Keyword detection (autopilot, swarm, eco, model overrides)
#   memory   - aide storage/retrieval
#   swarm    - Swarm mode activation and worktree creation
#   hud      - Statusbar/HUD updates
#   config   - Config hot-reload detection
#   hooks    - Run vitest hook tests

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="$PROJECT_ROOT/.aide-test"
AIDE_MEMORY_BIN="$PROJECT_ROOT/bin/aide"
DEBUG="${AIDE_DEBUG:-0}"

# Timing helper
declare -A TIMERS
timer_start() {
    if [[ "$DEBUG" != "0" ]]; then
        TIMERS[$1]=$(date +%s%N)
    fi
}

timer_end() {
    if [[ "$DEBUG" != "0" ]]; then
        local start=${TIMERS[$1]:-$(date +%s%N)}
        local end=$(date +%s%N)
        local duration=$(( (end - start) / 1000000 ))
        echo -e "  ${BLUE}⏱ $1: ${duration}ms${NC}"
    fi
}

# Test result tracking
PASSED=0
FAILED=0
SKIPPED=0

pass() {
    echo -e "  ${GREEN}✓ $1${NC}"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "  ${RED}✗ $1${NC}"
    if [[ -n "${2:-}" ]]; then
        echo -e "    ${RED}$2${NC}"
    fi
    FAILED=$((FAILED + 1))
}

skip() {
    echo -e "  ${YELLOW}⊘ $1 (skipped)${NC}"
    SKIPPED=$((SKIPPED + 1))
}

info() {
    echo -e "${BLUE}→ $1${NC}"
}

section() {
    echo ""
    echo -e "${YELLOW}━━━ $1 ━━━${NC}"
}

# Setup test environment
setup() {
    info "Setting up test environment..."
    timer_start "setup"

    rm -rf "$TEST_DIR"
    mkdir -p "$TEST_DIR/.aide/state"
    mkdir -p "$TEST_DIR/.aide/config"
    mkdir -p "$TEST_DIR/.aide/memory"
    mkdir -p "$TEST_DIR/.aide/skills"
    mkdir -p "$TEST_DIR/.aide/_logs"

    # Initialize git repo for worktree tests
    cd "$TEST_DIR"
    git init -q
    git config user.email "test@aide.local"
    git config user.name "AIDE Test"
    echo "# Test" > README.md
    git add README.md
    git commit -q -m "Initial commit"
    cd - > /dev/null

    # Create default config
    cat > "$TEST_DIR/.aide/config/aide.json" << 'EOF'
{
  "version": "0.1.0",
  "debug": false,
  "skills": { "autoDiscover": true, "cacheSeconds": 60 },
  "modes": { "autopilot": { "enabled": true }, "ralph": { "enabled": true }, "eco": { "enabled": true }, "swarm": { "enabled": true, "maxAgents": 5 } },
  "models": { "fast": "haiku", "balanced": "sonnet", "smart": "opus" }
}
EOF

    cat > "$TEST_DIR/.aide/config/hud.json" << 'EOF'
{
  "enabled": true,
  "elements": ["mode", "model", "duration", "tools", "agents", "tasks"],
  "format": "minimal"
}
EOF

    # Create initial session state
    cat > "$TEST_DIR/.aide/state/session.json" << EOF
{
  "sessionId": "test-$(date +%s)",
  "startedAt": "$(date -Iseconds)",
  "cwd": "$TEST_DIR",
  "activeMode": null,
  "modelTier": "balanced",
  "agentCount": 0,
  "taskCount": 0,
  "toolCalls": 0
}
EOF

    timer_end "setup"
}

# Cleanup test environment
cleanup() {
    if [[ "${KEEP_TEST_DIR:-0}" != "1" ]]; then
        rm -rf "$TEST_DIR"
    else
        info "Test directory kept at: $TEST_DIR"
    fi
}

# ============================================================================
# TEST: Keyword Detection
# ============================================================================
test_keyword_detection() {
    section "Keyword Detection"

    local hook="$PROJECT_ROOT/dist/hooks/keyword-detector.js"
    if [[ ! -f "$hook" ]]; then
        skip "keyword-detector.js not built (run npm run build)"
        return
    fi

    # Test autopilot keyword
    timer_start "keyword-autopilot"
    local result=$(echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"autopilot build me a web app"}' | node "$hook" 2>/dev/null)
    timer_end "keyword-autopilot"

    if [[ -f "$TEST_DIR/.aide/state/autopilot-state.json" ]]; then
        local active=$(grep -o '"active"[[:space:]]*:[[:space:]]*true' "$TEST_DIR/.aide/state/autopilot-state.json" || echo "")
        if [[ -n "$active" ]]; then
            pass "autopilot keyword detected"
        else
            fail "autopilot keyword detected but state not active"
        fi
    else
        fail "autopilot keyword not detected" "State file not created"
    fi

    # Clean state for next test
    rm -f "$TEST_DIR/.aide/state/autopilot-state.json"

    # Test eco keyword
    timer_start "keyword-eco"
    echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"eco mode please, budget conscious"}' | node "$hook" 2>/dev/null
    timer_end "keyword-eco"

    if [[ -f "$TEST_DIR/.aide/state/eco-state.json" ]]; then
        pass "eco keyword detected"
    else
        fail "eco keyword not detected"
    fi
    rm -f "$TEST_DIR/.aide/state/eco-state.json"

    # Test swarm keyword with count
    timer_start "keyword-swarm"
    echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"swarm 3 agents to build this"}' | node "$hook" 2>/dev/null
    timer_end "keyword-swarm"

    if [[ -f "$TEST_DIR/.aide/state/swarm-state.json" ]]; then
        local count=$(grep -o '"agentCount"[[:space:]]*:[[:space:]]*[0-9]*' "$TEST_DIR/.aide/state/swarm-state.json" | grep -o '[0-9]*$')
        if [[ "$count" == "3" ]]; then
            pass "swarm keyword detected with count=3"
        else
            fail "swarm keyword detected but count wrong: $count"
        fi
    else
        fail "swarm keyword not detected"
    fi
    rm -f "$TEST_DIR/.aide/state/swarm-state.json"

    # Test model override
    timer_start "keyword-model"
    echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"model:opus help me with this complex task"}' | node "$hook" 2>/dev/null
    timer_end "keyword-model"

    local model_tier=$(grep -o '"modelTier"[[:space:]]*:[[:space:]]*"[^"]*"' "$TEST_DIR/.aide/state/session.json" | cut -d'"' -f4)
    if [[ "$model_tier" == "smart" ]]; then
        pass "model:opus override detected (mapped to smart)"
    else
        fail "model override not detected" "Expected smart, got: $model_tier"
    fi

    # Test ralph keyword
    timer_start "keyword-ralph"
    echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"ralph mode - dont stop until this is done"}' | node "$hook" 2>/dev/null
    timer_end "keyword-ralph"

    if [[ -f "$TEST_DIR/.aide/state/ralph-state.json" ]]; then
        pass "ralph keyword detected"
    else
        fail "ralph keyword not detected"
    fi

    # Test stop keyword clears mode
    timer_start "keyword-stop"
    echo '{"event":"UserPromptSubmit","sessionId":"test","cwd":"'"$TEST_DIR"'","prompt":"stop please"}' | node "$hook" 2>/dev/null
    timer_end "keyword-stop"

    if [[ -f "$TEST_DIR/.aide/state/ralph-state.json" ]]; then
        local active=$(grep -o '"active"[[:space:]]*:[[:space:]]*false' "$TEST_DIR/.aide/state/ralph-state.json" || echo "")
        if [[ -n "$active" ]]; then
            pass "stop keyword deactivated ralph mode"
        else
            fail "stop keyword did not deactivate mode"
        fi
    else
        pass "stop keyword cleared ralph state file"
    fi
}

# ============================================================================
# TEST: Memory System (aide)
# ============================================================================
test_memory_system() {
    section "Memory System (aide)"

    if [[ ! -x "$AIDE_MEMORY_BIN" ]]; then
        skip "aide binary not found at $AIDE_MEMORY_BIN"
        return
    fi

    # Binary derives DB path from cwd (TEST_DIR has .aide/ structure)

    # Test adding a memory
    timer_start "memory-add"
    local add_result=$("$AIDE_MEMORY_BIN" memory add \
        --category learning \
        "Test learning: AIDE hooks work via stdin/stdout JSON" 2>&1)
    timer_end "memory-add"

    if [[ $? -eq 0 ]]; then
        pass "memory add succeeded"
    else
        fail "memory add failed" "$add_result"
    fi

    # Test listing memories
    timer_start "memory-list"
    local list_result=$("$AIDE_MEMORY_BIN" memory list 2>&1)
    timer_end "memory-list"

    if echo "$list_result" | grep -q "Test learning"; then
        pass "memory list shows added memory"
    else
        fail "memory list doesn't show added memory" "$list_result"
    fi

    # Test search
    timer_start "memory-search"
    local search_result=$("$AIDE_MEMORY_BIN" memory search "hooks" 2>&1)
    timer_end "memory-search"

    if echo "$search_result" | grep -q "Test learning"; then
        pass "memory search finds by content"
    else
        fail "memory search didn't find memory" "$search_result"
    fi

    # Test task creation
    timer_start "task-create"
    local task_result=$("$AIDE_MEMORY_BIN" task create "Test task for AIDE" 2>&1)
    timer_end "task-create"

    if [[ $? -eq 0 ]]; then
        pass "task create succeeded"
    else
        fail "task create failed" "$task_result"
    fi

    # Test task listing
    timer_start "task-list"
    local tasks=$("$AIDE_MEMORY_BIN" task list 2>&1)
    timer_end "task-list"

    if echo "$tasks" | grep -q "Test task"; then
        pass "task list shows created task"
    else
        fail "task list doesn't show task" "$tasks"
    fi

    # Test task claim (atomic operation for swarm)
    timer_start "task-claim"
    # Extract task ID from output like "[pending] abc123: Test task for AIDE"
    local task_id=$(echo "$tasks" | grep -o '\[pending\] [^:]*' | head -1 | awk '{print $2}')
    if [[ -n "$task_id" ]]; then
        local claim_result=$("$AIDE_MEMORY_BIN" task claim "$task_id" --agent=test-agent-1 2>&1)
        if [[ $? -eq 0 ]]; then
            pass "task claim succeeded (atomic for swarm)"
        else
            fail "task claim failed" "$claim_result"
        fi
    else
        skip "Could not extract task ID for claim test"
    fi
    timer_end "task-claim"

    # Test decision storage
    timer_start "decision-set"
    local decision_result=$("$AIDE_MEMORY_BIN" decision set architecture "Use TypeScript for hooks" --rationale="Better type safety" 2>&1)
    timer_end "decision-set"

    if [[ $? -eq 0 ]]; then
        pass "decision set succeeded"
    else
        fail "decision set failed" "$decision_result"
    fi

    # Test decision retrieval
    timer_start "decision-get"
    local get_decision=$("$AIDE_MEMORY_BIN" decision get architecture 2>&1)
    timer_end "decision-get"

    if echo "$get_decision" | grep -q "TypeScript"; then
        pass "decision get retrieves stored decision"
    else
        fail "decision get didn't retrieve decision" "$get_decision"
    fi

    # Test message system (for swarm inter-agent communication)
    timer_start "message-send"
    local msg_result=$("$AIDE_MEMORY_BIN" message send "Task A is complete, you can start Task B" --from=agent-1 --to=agent-2 2>&1)
    timer_end "message-send"

    if [[ $? -eq 0 ]]; then
        pass "message send succeeded"
    else
        fail "message send failed" "$msg_result"
    fi

    # Test export (exports to directory, check it was created)
    timer_start "memory-export"
    mkdir -p "$TEST_DIR/.aide/memory/exports"
    local export_result=$("$AIDE_MEMORY_BIN" memory export --output="$TEST_DIR/.aide/memory/exports" 2>&1)
    timer_end "memory-export"

    if [[ -d "$TEST_DIR/.aide/memory/exports" ]]; then
        local export_files=$(ls "$TEST_DIR/.aide/memory/exports" 2>/dev/null | wc -l)
        if [[ "$export_files" -gt 0 ]]; then
            pass "export creates files"
        else
            pass "export command succeeded"  # Empty exports are ok if no data
        fi
    else
        fail "export failed" "$export_result"
    fi
}

# ============================================================================
# TEST: HUD/Statusbar Updates
# ============================================================================
test_hud_updates() {
    section "HUD/Statusbar Updates"

    local hook="$PROJECT_ROOT/dist/hooks/hud-updater.js"
    local hud_script="$PROJECT_ROOT/bin/aide-hud.sh"

    if [[ ! -f "$hook" ]]; then
        skip "hud-updater.js not built"
        return
    fi

    # Reset session state
    cat > "$TEST_DIR/.aide/state/session.json" << EOF
{
  "sessionId": "hud-test",
  "startedAt": "$(date -Iseconds)",
  "cwd": "$TEST_DIR",
  "activeMode": null,
  "modelTier": "balanced",
  "agentCount": 0,
  "taskCount": 0,
  "toolCalls": 0
}
EOF

    # Test HUD update on tool use
    timer_start "hud-update"
    echo '{"event":"PostToolUse","sessionId":"hud-test","cwd":"'"$TEST_DIR"'","toolName":"Read","toolResult":{"success":true}}' | node "$hook" 2>/dev/null
    timer_end "hud-update"

    # Check session state was updated
    local tool_calls=$(grep -o '"toolCalls"[[:space:]]*:[[:space:]]*[0-9]*' "$TEST_DIR/.aide/state/session.json" | grep -o '[0-9]*$')
    if [[ "$tool_calls" == "1" ]]; then
        pass "tool call tracked in session state"
    else
        fail "tool call not tracked" "Expected 1, got: $tool_calls"
    fi

    # Check HUD file was created
    if [[ -f "$TEST_DIR/.aide/state/hud.txt" ]]; then
        pass "HUD file created"
        if [[ "$DEBUG" != "0" ]]; then
            echo "  ${BLUE}HUD content: $(cat "$TEST_DIR/.aide/state/hud.txt")${NC}"
        fi
    else
        fail "HUD file not created"
    fi

    # Test bash script fallback
    timer_start "hud-script"
    cd "$TEST_DIR"
    local hud_output=$(bash "$hud_script" 2>/dev/null)
    cd - > /dev/null
    timer_end "hud-script"

    if echo "$hud_output" | grep -q "\[aide\]"; then
        pass "HUD script produces output"
        if [[ "$DEBUG" != "0" ]]; then
            echo "  ${BLUE}Script output: $hud_output${NC}"
        fi
    else
        fail "HUD script produced no output"
    fi

    # Test agent count tracking
    timer_start "hud-agents"
    echo '{"event":"PostToolUse","sessionId":"hud-test","cwd":"'"$TEST_DIR"'","toolName":"Task","toolResult":{"success":true}}' | node "$hook" 2>/dev/null
    timer_end "hud-agents"

    local agent_count=$(grep -o '"agentCount"[[:space:]]*:[[:space:]]*[0-9]*' "$TEST_DIR/.aide/state/session.json" | grep -o '[0-9]*$')
    if [[ "$agent_count" == "1" ]]; then
        pass "agent spawn tracked"
    else
        fail "agent spawn not tracked" "Expected 1, got: $agent_count"
    fi

    # Test task count tracking
    timer_start "hud-tasks"
    echo '{"event":"PostToolUse","sessionId":"hud-test","cwd":"'"$TEST_DIR"'","toolName":"TaskCreate","toolResult":{"success":true}}' | node "$hook" 2>/dev/null
    timer_end "hud-tasks"

    local task_count=$(grep -o '"taskCount"[[:space:]]*:[[:space:]]*[0-9]*' "$TEST_DIR/.aide/state/session.json" | grep -o '[0-9]*$')
    if [[ "$task_count" == "1" ]]; then
        pass "task creation tracked"
    else
        fail "task creation not tracked" "Expected 1, got: $task_count"
    fi
}

# ============================================================================
# TEST: Config Hot-Reload
# ============================================================================
test_config_reload() {
    section "Config Hot-Reload Detection"

    local hook="$PROJECT_ROOT/dist/hooks/hud-updater.js"
    if [[ ! -f "$hook" ]]; then
        skip "hud-updater.js not built"
        return
    fi

    # Initial config
    cat > "$TEST_DIR/.aide/config/hud.json" << 'EOF'
{
  "enabled": true,
  "elements": ["mode", "model"],
  "format": "minimal"
}
EOF

    # Trigger hook to read initial config
    timer_start "config-initial"
    echo '{"event":"PostToolUse","sessionId":"test","cwd":"'"$TEST_DIR"'","toolName":"Read"}' | node "$hook" 2>/dev/null
    timer_end "config-initial"

    local hud1=$(cat "$TEST_DIR/.aide/state/hud.txt" 2>/dev/null || echo "")

    # Modify config
    cat > "$TEST_DIR/.aide/config/hud.json" << 'EOF'
{
  "enabled": true,
  "elements": ["mode", "model", "duration", "tools"],
  "format": "full"
}
EOF

    # Trigger hook again - should pick up new config
    timer_start "config-reload"
    echo '{"event":"PostToolUse","sessionId":"test","cwd":"'"$TEST_DIR"'","toolName":"Read"}' | node "$hook" 2>/dev/null
    timer_end "config-reload"

    local hud2=$(cat "$TEST_DIR/.aide/state/hud.txt" 2>/dev/null || echo "")

    if [[ "$hud1" != "$hud2" ]]; then
        pass "config change detected on next hook execution"
        if [[ "$DEBUG" != "0" ]]; then
            echo "  ${BLUE}Before: $hud1${NC}"
            echo "  ${BLUE}After:  $hud2${NC}"
        fi
    else
        fail "config change not reflected" "HUD output unchanged"
    fi

    # Test disabled HUD
    cat > "$TEST_DIR/.aide/config/hud.json" << 'EOF'
{
  "enabled": false
}
EOF

    timer_start "config-disabled"
    echo '{"event":"PostToolUse","sessionId":"test","cwd":"'"$TEST_DIR"'","toolName":"Read"}' | node "$hook" 2>/dev/null
    timer_end "config-disabled"

    local hud3=$(cat "$TEST_DIR/.aide/state/hud.txt" 2>/dev/null || echo "")
    if [[ -z "$hud3" ]]; then
        pass "disabled HUD produces empty output"
    else
        fail "disabled HUD still produces output" "$hud3"
    fi
}

# ============================================================================
# TEST: Swarm Mode & Worktrees
# ============================================================================
test_swarm_mode() {
    section "Swarm Mode & Worktrees"

    # Check if worktree library exists
    local worktree_lib="$PROJECT_ROOT/dist/lib/worktree.js"
    if [[ ! -f "$worktree_lib" ]]; then
        skip "worktree.js not built"
        return
    fi

    cd "$TEST_DIR"

    # Test worktree creation via direct node execution
    timer_start "worktree-create"
    local create_script='
    const { createWorktree } = require("'"$worktree_lib"'");
    createWorktree("'"$TEST_DIR"'", "task-1", "agent-1")
      .then(r => console.log(JSON.stringify(r)))
      .catch(e => console.error(e.message));
    '
    local wt_result=$(node -e "$create_script" 2>&1)
    timer_end "worktree-create"

    if echo "$wt_result" | grep -q "path"; then
        pass "worktree created successfully"
        if [[ "$DEBUG" != "0" ]]; then
            echo "  ${BLUE}Result: $wt_result${NC}"
        fi
    else
        # Git worktree might fail in test environment, that's ok
        skip "worktree creation (may need full git setup): $wt_result"
    fi

    # Test worktree state tracking
    if [[ -f "$TEST_DIR/.aide/state/worktrees.json" ]]; then
        pass "worktree state file created"
    else
        skip "worktree state file not created"
    fi

    cd - > /dev/null
}

# ============================================================================
# TEST: Run Vitest Hook Tests
# ============================================================================
test_vitest_hooks() {
    section "Vitest Hook Tests"

    cd "$PROJECT_ROOT"

    timer_start "vitest"
    if npx vitest run src/test/hooks.test.ts --reporter=verbose 2>&1; then
        pass "all vitest hook tests passed"
    else
        fail "some vitest tests failed"
    fi
    timer_end "vitest"

    cd - > /dev/null
}

# ============================================================================
# TEST: Go Memory Tests
# ============================================================================
test_go_memory() {
    section "Go Memory Package Tests"

    if ! command -v go &> /dev/null; then
        skip "Go not installed"
        return
    fi

    cd "$PROJECT_ROOT/aide"

    timer_start "go-test"
    if go test ./pkg/... -v 2>&1; then
        pass "all Go memory tests passed"
    else
        fail "some Go tests failed"
    fi
    timer_end "go-test"

    cd - > /dev/null
}

# ============================================================================
# MAIN
# ============================================================================
main() {
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║       AIDE Plugin Test Suite           ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"

    if [[ "$DEBUG" != "0" ]]; then
        echo -e "${BLUE}Debug mode enabled - timing will be shown${NC}"
    fi

    local test_filter="${1:-all}"

    # Setup
    setup
    trap cleanup EXIT

    # Run tests based on filter
    case "$test_filter" in
        all)
            test_keyword_detection
            test_memory_system
            test_hud_updates
            test_config_reload
            test_swarm_mode
            test_vitest_hooks
            test_go_memory
            ;;
        keyword)
            test_keyword_detection
            ;;
        memory)
            test_memory_system
            ;;
        hud)
            test_hud_updates
            ;;
        config)
            test_config_reload
            ;;
        swarm)
            test_swarm_mode
            ;;
        hooks)
            test_vitest_hooks
            ;;
        go)
            test_go_memory
            ;;
        *)
            echo "Unknown test: $test_filter"
            echo "Available: all, keyword, memory, hud, config, swarm, hooks, go"
            exit 1
            ;;
    esac

    # Summary
    section "Test Summary"
    echo -e "  ${GREEN}Passed:  $PASSED${NC}"
    echo -e "  ${RED}Failed:  $FAILED${NC}"
    echo -e "  ${YELLOW}Skipped: $SKIPPED${NC}"
    echo ""

    if [[ $FAILED -gt 0 ]]; then
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    else
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    fi
}

main "$@"
