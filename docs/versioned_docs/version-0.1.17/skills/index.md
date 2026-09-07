---
sidebar_label: Overview
sidebar_position: 1
title: Skills
---

# Skills

Skills are markdown files that inject context, instructions, and workflows into your AI session when triggered by keywords. They're the primary way AIDE extends your assistant's capabilities.

## How Skills Work

When you type a message, AIDE's skill matcher scans it against all known skill triggers. If a match is found, the skill's markdown content is injected into the conversation, giving the AI detailed instructions for a specific workflow.

```
You: "write tests for the auth module"
     ↓
     AIDE matches "write tests" → test skill
     ↓
     Injects test skill instructions into context
     ↓
     AI follows structured TDD workflow
```

### Fuzzy Matching

Trigger matching uses **Levenshtein distance** for typo tolerance. You don't need to type triggers exactly:

- `"write tsets for auth"` still matches the **test** skill
- `"debug ths issue"` still matches the **debug** skill

## Skill Categories

Skills fall into several categories:

| Category        | Skills                                             | Purpose                                      |
| --------------- | -------------------------------------------------- | -------------------------------------------- |
| **Planning**    | plan-swarm, decide, design                         | Decompose work, make decisions, create specs |
| **Development** | test, implement, build-fix                         | TDD, implementation, fixing build issues     |
| **Quality**     | verify, review, patterns, assess-findings, semgrep | QA, code review, static analysis, security   |
| **Operations**  | swarm, autopilot, debug, perf                      | Parallel execution, persistence, debugging   |
| **Knowledge**   | memorise, recall, forget                           | Memory management                            |
| **Code**        | code-search, survey, git, worktree-resolve         | Code navigation, codebase survey, git ops    |
| **Docs**        | docs                                               | Documentation updates                        |
| **Utility**     | context-usage                                      | Session diagnostics                          |

## Discovery Order

Skills are discovered from multiple locations. Higher priority sources override lower ones:

1. **Project-local** `.aide/skills/` — Project-specific overrides
2. **Project-local** `skills/` — Alternative project location
3. **Plugin-bundled** `skills/` — Ships with AIDE
4. **Global** `~/.aide/skills/` — User-wide custom skills

This means you can override any built-in skill by creating a file with the same name in your project's `.aide/skills/` directory.

## Hot Reloading

Skills are **automatically reloaded** when files change. Edit a skill file and it takes effect immediately — no restart needed.

## Next Steps

- [Built-in Skills](./built-in.md) — Full reference of all 24 built-in skills
- [Custom Skills](./custom.md) — Create your own project-specific skills
