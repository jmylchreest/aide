# aide + OpenCode Integration

First-class aide integration for [OpenCode](https://opencode.ai) — provides memory injection, skill matching, tool tracking, session management, and MCP tools.

## Quick Start

```bash
bunx @jmylchreest/aide-plugin install
```

That's it. This registers the aide plugin and MCP server in your global OpenCode config (`~/.config/opencode/opencode.json`), so it applies to all projects.

**Note:** On first use (or after updates), the aide binary (62 MB) downloads automatically in the background. This takes 20-30 seconds, then tools become available. Fully automatic - no manual steps needed.

### Other Commands

```bash
bunx @jmylchreest/aide-plugin status      # Check installation status
bunx @jmylchreest/aide-plugin uninstall    # Remove from OpenCode config
```

### Options

| Flag        | Description                                              |
| ----------- | -------------------------------------------------------- |
| `--project` | Apply to project-level `opencode.json` instead of global |
| `--no-mcp`  | Register the plugin only, skip MCP server setup          |

## Alternative Installation Methods

### Manual Config

If you prefer to configure manually, add to your `opencode.json` (either `~/.config/opencode/opencode.json` for global or `./opencode.json` for project-level):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@jmylchreest/aide-plugin"],
  "mcp": {
    "aide": {
      "type": "local",
      "command": ["bunx", "-y", "@jmylchreest/aide-plugin", "mcp"],
      "environment": {
        "AIDE_CODE_WATCH": "1",
        "AIDE_CODE_WATCH_DELAY": "30s",
        "AIDE_FORCE_INIT": "1"
      },
      "enabled": true
    }
  }
}
```

### Local Plugin (Development)

For developing on aide itself, clone the repo and use the plugin directly:

```bash
git clone https://github.com/jmylchreest/aide
cd aide && cd aide && go build -o ../bin/aide ./cmd/aide && cd ..
npm install && npm run build
```

Then either symlink:

```bash
ln -s /path/to/aide/dist/opencode .opencode/plugins/aide
```

Or use the generator script, which creates an `opencode.json` and `.opencode/plugins/aide.ts` wired to your local clone:

```bash
npx tsx adapters/opencode/generate.ts --plugin-path /path/to/aide
```

## What Works

| Feature            | Status | Notes                                                                    |
| ------------------ | ------ | ------------------------------------------------------------------------ |
| Memory injection   | ✅     | Global + project memories injected via system prompt transform           |
| Skill matching     | ✅     | Fuzzy matching on user messages, injected via system transform           |
| Tool tracking      | ✅     | PreToolUse/PostToolUse tracking via `tool.execute.*` hooks               |
| Session summaries  | ✅     | Git-based summaries on session idle (no transcript access)               |
| Context compaction | ✅     | State snapshot + context injection via `experimental.session.compacting` |
| Session cleanup    | ✅     | State cleanup on session delete                                          |
| MCP tools          | ✅     | `memory_search`, `code_search`, `decision_get`, etc. via aide MCP        |
| Decisions          | ✅     | Injected in welcome context from `aide session init`                     |

## Known Limitations

### 1. No Stop Blocking (Persistence / Ralph Mode)

**Impact**: Medium — ralph mode and autopilot work but with a different mechanism than Claude Code.

Claude Code's Stop hook can return `{ decision: "block" }` to prevent the agent from stopping (preventive). OpenCode has no equivalent mechanism — the `session.idle` event fires _after_ the agent has already stopped responding.

**Implementation**: The plugin uses `session.prompt()` to re-prompt the session when idle with a persistence mode active (ralph, autopilot). This is reactive (post-stop re-prompting) rather than preventive (block stop), using the same `checkPersistence()` core logic as Claude Code. The agent may briefly appear idle before being re-prompted to continue.

### 2. No Subagent Hooks

**Impact**: Medium — swarm mode (parallel agents) is not available.

Claude Code provides `SubagentStart` and `SubagentStop` hooks that aide uses for worktree management and parallel agent coordination. OpenCode does not have subagent lifecycle hooks.

**Workarounds**:

- **Multi-instance orchestration**: Spawn multiple OpenCode instances (each on a different port) and coordinate via aide's messaging system. See "Multi-Instance Orchestration" below.
- **tmux-based orchestration**: Tools like [Oh My OpenCode](https://github.com/code-yeongyu/oh-my-opencode) spawn subagent instances in tmux panes connected via `opencode attach`.
- **SDK-based orchestration**: Use `@opencode-ai/sdk` to programmatically create and manage multiple sessions.

### 3. No Transcript Access

**Impact**: Low — session summaries are less detailed.

Claude Code provides `transcript_path` in Stop hook data, allowing aide to parse the full conversation JSONL for detailed summaries. OpenCode does not expose transcripts to plugins.

**Workaround**: The plugin uses `buildSessionSummaryFromState()` which creates summaries from git history and tracked state instead of transcript parsing. Summaries include commits and modified files but miss user prompts and tool details.

### 4. No HUD Display

**Impact**: Low — cosmetic only.

Claude Code's terminal supports a custom status line (HUD) showing mode, agents, and tool activity. OpenCode's TUI does not support external status line injection.

**Workaround**: The plugin resets `.aide/state/hud.txt` on init, but does not update it during the session. The HUD updater is a Claude Code PostToolUse hook that is not triggered in OpenCode. External tools could monitor `.aide/state/` for other state files if needed.

### 5. No Usage Tracking

**Impact**: Low — informational only.

Claude Code exposes OAuth-based API usage statistics (5-hour and weekly limits). OpenCode does not provide equivalent usage data to plugins.

**Workaround**: None. Usage tracking is omitted from the OpenCode integration.

### 6. Skill Injection Timing

**Impact**: Low — potential for duplicate processing.

OpenCode's `message.part.updated` event may fire multiple times for the same message as parts are streamed. The plugin deduplicates by tracking processed part IDs, but edge cases may exist.

**Workaround**: Built-in dedup with a capped set (auto-clears after 1000 entries).

## Multi-Instance Orchestration

For workloads that benefit from parallel agents (what aide's swarm mode does in Claude Code), you can orchestrate multiple OpenCode instances:

### Approach A: SDK-Based (Recommended)

```typescript
import { createOpencode } from "@opencode-ai/sdk";

// Spawn worker instances on different ports
const workers = await Promise.all([
  createOpencode({ port: 4100 }),
  createOpencode({ port: 4101 }),
  createOpencode({ port: 4102 }),
]);

// Assign tasks
const tasks = [
  { instance: workers[0], prompt: "Implement the auth module" },
  { instance: workers[1], prompt: "Write unit tests for auth" },
  { instance: workers[2], prompt: "Update documentation for auth" },
];

for (const task of tasks) {
  const session = await task.instance.client.session.create({
    body: { title: task.prompt },
  });
  await task.instance.client.session.prompt({
    path: { id: session.id },
    body: {
      parts: [{ type: "text", text: task.prompt }],
      model: { providerID: "anthropic", modelID: "claude-sonnet-4-5-20250929" },
    },
  });
}

// Monitor via SSE events
for (const worker of workers) {
  const events = await worker.client.event.subscribe();
  // Handle session.idle to know when each worker finishes
}
```

### Approach B: tmux + CLI

```bash
# Start main OpenCode server
opencode serve --port 4096 &

# Spawn workers in tmux panes
tmux split-window "opencode run --attach http://localhost:4096 'Implement auth module'"
tmux split-window "opencode run --attach http://localhost:4096 'Write auth tests'"
```

### Approach C: Oh My OpenCode

[Oh My OpenCode](https://github.com/code-yeongyu/oh-my-opencode) provides a ready-made orchestration layer:

```json
{
  "tmux": { "enabled": true, "layout": "tiled" },
  "agents": {
    "lead": { "mode": "primary" },
    "worker-1": { "mode": "subagent" },
    "worker-2": { "mode": "subagent" }
  }
}
```

All approaches share aide's memory and state through the aide binary and MCP server, enabling cross-instance coordination.

## Architecture

```
┌─────────────────────────────────────────┐
│              OpenCode TUI               │
├─────────────────────────────────────────┤
│         @jmylchreest/aide-plugin           │
│  ┌──────────┐ ┌──────────┐ ┌─────────┐ │
│  │  event    │ │  tool.*  │ │ system  │ │
│  │ handler   │ │ handlers │ │transform│ │
│  └────┬─────┘ └────┬─────┘ └────┬────┘ │
├───────┼────────────┼────────────┼───────┤
│       │    src/core/ (shared)   │       │
│  ┌────┴─────────────────────────┴────┐  │
│  │ session-init │ skill-matcher      │  │
│  │ tool-tracking│ persistence-logic  │  │
│  │ cleanup      │ session-summary    │  │
│  │ pre-compact  │ aide-client        │  │
│  └───────────────┬───────────────────┘  │
├──────────────────┼──────────────────────┤
│            aide binary (CLI)            │
│  ┌───────────────┼───────────────────┐  │
│  │ state │ memory │ session │ mcp    │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

## Troubleshooting

### MCP Tools Not Available Immediately

**This is normal!** On first use (or after plugin updates), the aide binary (62 MB) downloads automatically in the background. This takes 20-30 seconds, after which tools become available.

**Why this is good:**

- ✅ Automatic updates - `@latest` in your config means you always get the newest version
- ✅ No manual binary installation - completely hands-off
- ✅ Cached after first download - subsequent startups are instant

**Verify download progress:** Check OpenCode logs at `~/.local/share/opencode/log/` for:

```
[aide] Downloading... X/62.1 MB
[aide] ✓ Binary installed successfully
aide MCP server starting
```

### If Tools Never Appear

Check if Bun is installed:

```bash
bun --version  # Should show v1.x.x
```

The published package requires Bun (uses `#!/usr/bin/env bun` shebang). Install from https://bun.sh if needed.`

## Environment Variables

| Variable             | Default | Description                                     |
| -------------------- | ------- | ----------------------------------------------- |
| `AIDE_DEBUG`         | unset   | Set to `1` for debug logging to `.aide/_logs/`  |
| `AIDE_FORCE_INIT`    | unset   | Set to `1` to initialize in non-git directories |
| `AIDE_MEMORY_INJECT` | `1`     | Set to `0` to skip memory injection             |
