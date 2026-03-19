# Worktree Management

## Overview

Each builder agent works in an isolated git worktree inside `.loom/worktrees/`. This prevents conflicts between agents and keeps the main working directory clean.

## Naming Convention

```
.loom/worktrees/{issue-id}-{slug}/
```

Branch name (same as directory name):
```
{issue-id}-{slug}
```

Examples:
```
Worktree: .loom/worktrees/LOOM-001-01-login-form/
Branch:   LOOM-001-01-login-form

Worktree: .loom/worktrees/LOOM-002-api-timeout/
Branch:   LOOM-002-api-timeout
```

The `parseNameConvention()` function uses regex `^(LOOM-\d+(?:-\d+)?)` to extract the issue ID from the worktree/branch name. Non-matching names silently break issue association.

## Lifecycle

### Creation (when builder spawns)

```bash
# 1. Create worktree from current HEAD of default branch
git worktree add .loom/worktrees/LOOM-001-01-login-form -b LOOM-001-01-login-form

# 2. Record in agent YAML
# agents/builder-017.yaml → worktree: LOOM-001-01-login-form

# 3. Builder's kiro-cli working directory is set to the worktree path
```

### During Work

- Builder works exclusively in their worktree directory
- All file edits, git commits happen inside the worktree
- Builder commits frequently with descriptive messages
- Builder does NOT push (everything is local)

### Completion (when builder finishes)

```bash
# 1. Builder commits final state
cd .loom/worktrees/LOOM-001-01-login-form
git add -A && git commit -m "feat(auth): implement login form validation"

# 2. Builder sends completion mail to lead
loom mail send lead-auth "LOOM-001-01 complete" --type completion

# 3. Lead reviews (or assigns reviewer)
# 4. Lead merges (or use loom merge):
loom merge LOOM-001-01 --cleanup

# 5. Or manually:
git checkout main
git merge --squash LOOM-001-01-login-form
git commit -m "feat(auth): login form validation (LOOM-001-01)"
git worktree remove .loom/worktrees/LOOM-001-01-login-form
git branch -d LOOM-001-01-login-form
```

### Cleanup on Agent Death

If a builder crashes:
1. Worktree is preserved (NOT auto-deleted)
2. Agent is marked as `dead`
3. Lead is notified
4. Lead can:
   - Inspect the worktree manually
   - Spawn a new builder to continue from the worktree's state
   - Discard the worktree via `loom worktree cleanup`

## Merge Strategy

Configurable in `.loom/config.yaml`:

| Strategy | Command | When to use |
|---|---|---|
| `squash` (default) | `git merge --squash` | Clean history, one commit per task |
| `merge` | `git merge --no-ff` | Preserve individual commits |
| `rebase` | `git rebase main` then fast-forward | Linear history |

The lead agent performs the merge. If conflicts arise:
1. Lead attempts to resolve
2. If unable, lead sends a blocker mail to orchestrator
3. Orchestrator may nudge the human or re-sequence work

## Conflict Prevention

### File Locks

Before a builder starts editing a file, they should acquire a lock:

```bash
loom lock acquire src/auth/login.ts
```

This creates `.loom/locks/src__auth__login.ts.lock`:
```yaml
file: src/auth/login.ts
agent: builder-017
acquired_at: 2026-03-09T18:42:00-04:00
issue: LOOM-001-01
```

If the lock already exists:
```bash
$ loom lock acquire src/auth/login.ts
LOCKED by builder-018 (LOOM-001-02) since 18:40:00
```

The builder should report this as a blocker to their lead.

Locks are released when:
- Builder explicitly releases: `loom lock release src/auth/login.ts`
- Builder's worktree is cleaned up
- `loom worktree cleanup` runs

### Proactive Conflict Avoidance

Leads should assign tasks with non-overlapping file sets. The explorer agent can help identify which files a task will likely touch before assignment.

## Worktree Inspection

```bash
# List all worktrees
$ loom worktree list
WORKTREE                              AGENT        ISSUE        STATUS    FILES CHANGED
LOOM-001-01-login-form                builder-017  LOOM-001-01  active    4 files (+120, -15)
LOOM-002-api-timeout                  builder-019  LOOM-002     active    2 files (+30, -8)

# Show detail
$ loom worktree show LOOM-001-01-login-form
Path:    .loom/worktrees/LOOM-001-01-login-form
Branch:  LOOM-001-01-login-form
Agent:   builder-017
Issue:   LOOM-001-01
Status:  active
Commits: 3
Files:
  M src/components/LoginForm.tsx (+85, -5)
  A src/components/LoginForm.test.tsx (+30)
  M src/utils/validation.ts (+5, -2)
  M src/styles/auth.css (+10, -8)
```

## Limits

- `max_worktrees` in config (default: 4) — prevents disk/memory exhaustion
- If limit reached, new builders queue until a worktree is freed
- Each worktree is a full copy of the repo's working tree (disk space consideration)

## .gitignore

`loom init` adds to `.gitignore`:
```
.loom/
```

Worktrees, mail, issues, memory — none of this should be committed to the project repo.
