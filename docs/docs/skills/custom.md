---
sidebar_label: Custom Skills
sidebar_position: 3
title: Custom Skills
---

# Custom Skills

You can create project-specific or user-wide skills that extend AIDE's capabilities. Custom skills use the same format as built-in skills and are automatically discovered and hot-reloaded.

## Creating a Skill

Create a markdown file with YAML frontmatter in one of the discovery directories:

```markdown title=".aide/skills/deploy.md"
---
name: deploy
triggers:
  - deploy
  - ship it
  - push to production
---

# Deploy Workflow

Follow these steps to deploy:

1. Run the test suite: `npm test`
2. Build the project: `npm run build`
3. Run the deploy script: `./scripts/deploy.sh`
4. Verify the deployment: `curl https://api.example.com/health`

## Rollback

If something goes wrong:

1. Run `./scripts/rollback.sh`
2. Check logs: `kubectl logs -f deployment/api`
```

### Frontmatter Fields

| Field      | Required | Description                               |
| ---------- | -------- | ----------------------------------------- |
| `name`     | Yes      | Unique identifier for the skill           |
| `triggers` | Yes      | Array of strings that activate this skill |

### Trigger Design Tips

- Use **2-4 word phrases** that are natural to type
- Include **common variations** (e.g., both "deploy" and "ship it")
- Avoid triggers that are too generic (e.g., "help") as they'll match too often
- Fuzzy matching handles typos, so you don't need to list misspellings

## Discovery Directories

Skills are discovered in priority order (first match wins):

| Priority | Location          | Scope                          |
| -------- | ----------------- | ------------------------------ |
| 1        | `.aide/skills/`   | Project-specific               |
| 2        | `skills/`         | Project-specific (alternative) |
| 3        | Plugin `skills/`  | Built-in (ships with AIDE)     |
| 4        | `~/.aide/skills/` | User-wide (all projects)       |

### Overriding Built-in Skills

To customize a built-in skill, create a file with the **same name** in your project's `.aide/skills/` directory. Your version takes precedence:

```markdown title=".aide/skills/test.md"
---
name: test
triggers:
  - write tests
  - add tests
  - test this
---

# Test Workflow (Custom)

Use our project's testing conventions:

- Framework: **Vitest** (not Jest)
- Test location: `src/__tests__/`
- Naming: `*.test.ts`
- Run: `pnpm test`
- Coverage: `pnpm test:coverage` (minimum 80%)
```

## Skill Content Best Practices

### Structure

Skills work best when they provide **clear, actionable instructions**:

```markdown
---
name: my-skill
triggers:
  - my trigger
---

# Skill Title

Brief description of what this skill does.

## Steps

1. First step with specific commands
2. Second step with expected output
3. Verification step

## Common Issues

- Issue A: How to fix
- Issue B: How to fix
```

### What to Include

- **Specific commands** to run (with exact flags)
- **File paths** relevant to the workflow
- **Expected output** so the AI can verify success
- **Error handling** for common failure modes
- **Project conventions** (naming, directory structure, etc.)

### What to Avoid

- Overly generic instructions the AI already knows
- Very long skills (keep focused; split into multiple skills if needed)
- Triggers that overlap heavily with built-in skills

## Examples

### Database Migration Skill

```markdown title=".aide/skills/migrate.md"
---
name: migrate
triggers:
  - migrate
  - migration
  - schema change
  - add column
---

# Database Migration

## Creating a Migration

1. Generate: `npx prisma migrate dev --name <description>`
2. Review the generated SQL in `prisma/migrations/`
3. Test: `npx prisma migrate reset` (dev only)

## Conventions

- Use snake_case for table and column names
- Always add NOT NULL with defaults for new columns
- Include DOWN migration comments
```

### Release Skill

```markdown title=".aide/skills/release.md"
---
name: release
triggers:
  - release
  - cut a release
  - bump version
  - new version
---

# Release Process

1. Update CHANGELOG.md with changes since last release
2. Bump version: `npm version <major|minor|patch>`
3. Push with tags: `git push && git push --tags`
4. CI will handle publishing to npm

## Version Guidelines

- **patch**: Bug fixes, dependency updates
- **minor**: New features (backward compatible)
- **major**: Breaking changes
```

## Hot Reloading

Skills are automatically reloaded when files change. You can:

1. Edit a skill file
2. The change takes effect immediately
3. No need to restart AIDE or your AI assistant

This makes iterating on skill content fast and frictionless.
