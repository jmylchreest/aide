---
sidebar_position: 1
slug: /
---

# Introduction

AIDE (AI Development Environment) is a plugin that adds multi-agent orchestration, persistent memory, and intelligent workflows to your AI coding assistant. It supports **Claude Code** and **OpenCode** through a shared core with platform-specific adapters.

## What AIDE does

1. **Remembers context** across sessions with persistent memory and decisions
2. **Orchestrates agents** with swarm mode, spawning parallel workers with full SDLC pipelines
3. **Indexes your code** using tree-sitter for fast symbol search across your codebase
4. **Surveys codebase structure** discovering modules, tech stack, entry points, and change hotspots
5. **Analyses code quality** with 5 built-in static analysers (complexity, coupling, secrets, clones, security)
6. **Activates skills** by keyword with fuzzy matching -- 24 built-in workflows for testing, debugging, reviewing, and more

## Key features

- **Persistent memory**: Memories and decisions auto-inject at session start and survive across sessions
- **Swarm mode**: Spawn N parallel agents, each running a full SDLC pipeline (design, test, implement, verify, docs)
- **Code indexing**: Tree-sitter-based symbol search across TypeScript, JavaScript, Go, Python, Rust, and more
- **Static analysis**: Detect complexity, coupling, hardcoded secrets, duplicated code, and security issues without external tools
- **Codebase survey**: Discover modules, tech stack, entry points, and high-change files automatically
- **Skills system**: Markdown files that inject context when triggered by keywords with fuzzy matching
- **Multi-platform**: Works with Claude Code and OpenCode through a shared core

## How it works

```bash
# Spawn 3 parallel agents to implement a feature
swarm 3 implement the dashboard

# Store a preference for future sessions
remember that I prefer vitest for testing

# Next session, AIDE auto-injects your preferences
what testing framework should I use?
# => "Based on your preferences, you prefer vitest for testing."
```

## Next steps

- [Getting Started](/docs/getting-started/) -- Install AIDE for your platform
- [Features](/docs/features/memory) -- Learn about memory, code indexing, and static analysis
- [Skills](/docs/skills/) -- See all 24 built-in skills
