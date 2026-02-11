---
name: worktree-resolve
description: Resolve and merge git worktrees after swarm completion
triggers:
  - resolve worktrees
  - merge worktrees
  - cleanup swarm
  - finish swarm
---

# Worktree Resolution

**Recommended model tier:** balanced (sonnet) - this skill performs straightforward operations

Merge all swarm worktrees back into the main branch after testing.

## Workflow

### 1. List Active Worktrees

```bash
git worktree list
```

Check the AIDE worktree state file for metadata and status:
```bash
cat .aide/state/worktrees.json
```

**Worktree Status Values:**
- `active` - Agent is still working on this worktree
- `agent-complete` - Agent finished, ready for merge review
- `merged` - Successfully merged to main

**Only merge worktrees with status `agent-complete`.**

Example state file:
```json
{
  "active": [
    {
      "name": "story-auth",
      "path": ".aide/worktrees/story-auth",
      "branch": "feat/story-auth",
      "taskId": "story-auth",
      "agentId": "agent-auth",
      "status": "agent-complete",
      "createdAt": "2026-02-07T...",
      "completedAt": "2026-02-07T..."
    }
  ],
  "baseBranch": "main"
}
```

### 2. For Each Worktree Branch

Run these steps for every `feat/*` branch from swarm:

#### a) Test the Branch

```bash
# Checkout the worktree
cd .aide/worktrees/<name>

# Run tests
npm test  # or appropriate test command

# Run linting
npm run lint  # or appropriate lint command

# Check build
npm run build  # if applicable
```

#### b) Review Changes

```bash
# Back in main repo
git log main..feat/<name> --oneline
git diff main...feat/<name> --stat
```

#### c) Check Merge Compatibility

```bash
# Dry-run merge check
git merge-tree $(git merge-base main feat/<name>) main feat/<name>
```

If conflicts shown, note them for manual resolution.

### 3. Merge Clean Branches

For branches that pass tests and have no conflicts:

```bash
git checkout main
git merge feat/<branch-name> --no-edit
```

### 4. Handle Conflicts (Intelligent Resolution)

When merge conflicts occur, **do not use `-X theirs` or `-X ours`** - these blindly discard changes.

Instead, resolve conflicts intelligently:

#### Step 1: Attempt merge and identify conflicts
```bash
git merge feat/<name> --no-edit
# If conflicts occur, git will list the conflicted files
```

#### Step 2: For each conflicted file, read and analyze
```bash
# Read the file with conflict markers
cat <conflicted-file>
```

The conflict markers show:
```
<<<<<<< HEAD
[changes from main branch]
=======
[changes from feature branch]
>>>>>>> feat/<name>
```

#### Step 3: Resolve as a code review expert

Act as an expert code reviewer. For each conflict:

1. **Analyze both code paths** - What was each change trying to achieve?
2. **Understand the intent** - Are they complementary, overlapping, or contradictory?
3. **Modify the feature branch locally** to incorporate main's changes while preserving the feature's logic
4. **Edit the file** to remove conflict markers and combine both sets of functionality correctly

The resolution must:
- Remove all conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`)
- Preserve the functional intent of BOTH changes
- Be syntactically and semantically correct
- Maintain code style consistency

#### Step 4: Verify and complete
```bash
# Stage the resolved files
git add <resolved-file>

# Run tests to verify the resolution works
npm test  # or appropriate test command

# If tests pass, complete the merge
git commit --no-edit
```

#### Step 5: Handle failure

If you **cannot resolve** the conflict (logic is contradictory, tests fail after resolution, or changes are too complex):

1. **Abort the merge** to restore clean state:
   ```bash
   git merge --abort
   ```

2. **Record the failure** using aide messaging:
   ```bash
   aide message send --from=resolver --to=orchestrator "CONFLICT: Cannot merge feat/<name> - <brief reason>"
   ```

3. **Skip this branch** and continue with remaining branches

4. **Report at completion** - list unmerged branches in the final summary for manual review

**Do NOT:**
- Force through a broken resolution
- Use `-X theirs` or `-X ours` to blindly pick one side
- Get stuck - always abort and report if resolution fails

#### Example Resolution

**Conflict:**
```typescript
<<<<<<< HEAD
function getUser(id: string): User {
  return db.users.find(u => u.id === id);
}
=======
function getUser(id: string): User | null {
  const user = db.users.find(u => u.id === id);
  return user ?? null;
}
>>>>>>> feat/null-safety
```

**Analysis:**
- HEAD: Basic lookup returning User
- Feature: Added null-safety with explicit null return

**Resolution:**
```typescript
function getUser(id: string): User | null {
  const user = db.users.find(u => u.id === id);
  return user ?? null;
}
```
*Feature branch improved null safety - this is additive, keep it.*

### 5. Cleanup

After all branches merged:

```bash
# Remove each worktree
git worktree remove .aide/worktrees/<name>

# Delete merged branches
git branch -d feat/<name>

# Prune any orphaned worktrees
git worktree prune

# Clear state file
rm .aide/state/worktrees.json
```

### 6. Final Verification

```bash
# Ensure all tests pass on main
git checkout main
npm test
npm run lint
npm run build

# Check no worktrees remain
git worktree list  # Should only show main
```

## Summary Report

After resolution, report:

```
## Worktree Resolution Complete

### Merged Branches
- feat/task1-agent1: ✓ (3 files, +150/-20)
- feat/task2-agent2: ✓ (5 files, +280/-45)

### Skipped (conflicts/failures)
- feat/task3-agent3: Test failures in auth.test.ts

### Final Status
- All tests passing: ✓
- Lint clean: ✓
- Build successful: ✓
```

## Quick Commands

```bash
# List all feat branches from swarm
git branch --list 'feat/*'

# Merge all clean branches at once (risky - prefer one at a time)
for branch in $(git branch --list 'feat/*' | tr -d ' '); do
  git merge $branch --no-edit || echo "Conflict in $branch"
done

# Bulk cleanup worktrees
git worktree list | grep '.aide/worktrees' | awk '{print $1}' | xargs -I {} git worktree remove {}

# Bulk delete feat branches (only if merged)
git branch --list 'feat/*' | xargs git branch -d
```

## Failure Handling

### If merge fails:

1. **Abort immediately**: `git merge --abort`
2. **Record the failure**:
   ```bash
   aide message send --from=resolver --to=orchestrator "Merge failed: feat/<name> - <reason>"
   ```
3. **Continue with remaining branches** - do not block on one failure
4. **Include in final report** - list all failed branches with reasons

### If tests fail after merge:

1. **Revert the merge**: `git revert -m 1 HEAD`
2. **Record the failure** with aide messaging
3. **Continue with other branches**

### If worktree removal fails:

```bash
# Force remove if necessary
git worktree remove --force .aide/worktrees/<name>

# Prune any orphaned entries
git worktree prune
```

## Verification Criteria

### After each merge:

```bash
# Verify merge commit exists
git log -1 --oneline

# Verify no uncommitted changes
git status --porcelain  # Should be empty

# Verify tests pass
npm test  # or appropriate test command
```

### After full resolution:

```bash
# Verify all worktrees removed
git worktree list  # Should only show main worktree

# Verify all feature branches deleted (or list unmerged)
git branch --list 'feat/*'  # Should be empty for merged branches

# Verify main is clean
git status

# Verify final tests pass
npm test && npm run lint && npm run build
```

## Safety Notes

- **Always test each branch before merging**
- **Merge one at a time** to isolate issues
- **Keep backups** - create a tag before bulk operations: `git tag pre-swarm-merge`
- **Don't force delete** - use `-d` not `-D` to prevent deleting unmerged work
- **Report all failures** - never silently skip branches
