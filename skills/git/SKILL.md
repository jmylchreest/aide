---
name: git
description: Git operations and worktree management
triggers:
  - worktree
  - git worktree
  - create branch
  - manage branches
---

# Git Mode

Expert git operations including worktree management for parallel work.

## Commit Guidelines

### Message Format
```
<type>: <short description>

[optional body]
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`

### Commit Process
1. Check status: `git status`
2. Review diff: `git diff`
3. Stage specific files (not `git add .`)
4. Write descriptive message
5. Never skip hooks unless explicitly asked

## Worktree Management

### Create Worktree
```bash
# For feature work
git worktree add ../feature-branch -b feature/name

# For parallel tasks
git worktree add ../aide-worktrees/task-1 -b aide/task-1
```

### List Worktrees
```bash
git worktree list
```

### Remove Worktree
```bash
git worktree remove ../feature-branch
git branch -d feature/name  # if merged
```

## Common Operations

### Feature Branch Workflow
```bash
git checkout -b feature/name
# ... work ...
git add <specific-files>
git commit -m "feat: add feature"
git push -u origin feature/name
```

### Rebase onto Main
```bash
git fetch origin
git rebase origin/main
# resolve conflicts if any
git push --force-with-lease
```

### Cherry-Pick
```bash
git cherry-pick <commit-hash>
```

### Stash
```bash
git stash push -m "description"
git stash list
git stash pop
```

## Safety Rules

**Never run without explicit user request:**
- `git push --force` (use `--force-with-lease` if needed)
- `git reset --hard`
- `git clean -f`
- `git checkout .` or `git restore .`

**Safe operations:**
- `git status`, `git diff`, `git log`
- `git add <specific-files>`
- `git commit`
- `git push` (without force)
- `git worktree` operations

## Failure Handling

### Commit failures:

1. **Pre-commit hook failure** - Read the error, fix the issue, create NEW commit (never amend after hook failure)
2. **Merge conflict** - Report conflicts, do not auto-resolve without explicit request
3. **Push rejected** - Fetch and rebase, report if conflicts arise

### Worktree failures:

1. **Branch already exists** - Use different name or checkout existing
2. **Dirty working tree** - Stash or commit before worktree operations
3. **Locked worktree** - Check `git worktree list` for stale entries, use `git worktree prune`

### Recovery commands:

```bash
# Recover from bad merge (before commit)
git merge --abort

# Recover from bad rebase (before complete)
git rebase --abort

# Prune stale worktrees
git worktree prune
```

## Verification Criteria

### After commit:

```bash
# Verify commit was created
git log -1 --oneline

# Verify expected files changed
git show --stat HEAD
```

### After push:

```bash
# Verify remote updated
git log origin/<branch> -1 --oneline
```

### After worktree creation:

```bash
# Verify worktree exists
git worktree list | grep <worktree-path>

# Verify branch created
git branch -a | grep <branch-name>
```

## Parallel Work Pattern

For swarm mode or parallel features:

```bash
# Setup
git worktree add ../work-1 -b feature/part-1
git worktree add ../work-2 -b feature/part-2

# Work in parallel (different terminals/agents)
cd ../work-1 && # ... implement part 1
cd ../work-2 && # ... implement part 2

# Merge
git checkout main
git merge feature/part-1
git merge feature/part-2

# Cleanup
git worktree remove ../work-1
git worktree remove ../work-2
git branch -d feature/part-1 feature/part-2
```
