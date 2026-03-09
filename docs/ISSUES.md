# Issue Tracker

## Overview

Built-in issue tracking. Humans create issues via `loom issue create`, the orchestrator auto-picks them up. Leads create sub-issues for task decomposition. All state is YAML files on disk.

## Issue-Driven Workflow

```
Human creates issue ──► .loom/issues/LOOM-001.yaml written
                                    │
                        Daemon detects new file (polling)
                                    │
                        Daemon notifies orchestrator via tmux
                                    │
                        Orchestrator reads issue
                        Orchestrator creates plan
                        Orchestrator spawns lead
                                    │
                        Lead decomposes into sub-issues
                        LOOM-001-01, LOOM-001-02, etc.
                                    │
                        Lead assigns workers to sub-issues
                                    │
                        Workers execute, update issue status
                                    │
                        Lead merges work, closes sub-issues
                                    │
                        Lead closes parent issue
                        Lead notifies orchestrator
```

## Issue Schema

```yaml
# .loom/issues/LOOM-001.yaml
id: LOOM-001
title: "Build authentication system"
description: |
  Implement JWT-based authentication with login, registration,
  and token refresh endpoints. Include middleware for protected routes.
type: epic                       # epic | task | bug | spike
status: in-progress              # see State Machine below
priority: high                   # critical | high | normal | low
assignee: lead-auth              # agent ID or empty
parent: null                     # parent issue ID (for sub-issues)
depends_on: []                   # issue IDs that must complete first
worktree: null                   # worktree name (for builder tasks)
created_by: human                # "human" or agent ID
created_at: 2026-03-09T18:34:00-04:00
updated_at: 2026-03-09T18:45:00-04:00
closed_at: null
close_reason: null
children:                        # auto-populated when sub-issues reference this as parent
  - LOOM-001-01
  - LOOM-001-02
  - LOOM-001-03
history:
  - at: 2026-03-09T18:34:00-04:00
    by: human
    action: created
  - at: 2026-03-09T18:35:00-04:00
    by: orchestrator
    action: assigned
    to: lead-auth
  - at: 2026-03-09T18:36:00-04:00
    by: lead-auth
    action: status_change
    from: open
    to: in-progress
```

## State Machine

```
                ┌──────┐
                │ open │ ◄── created by human or agent
                └──┬───┘
                   │ assign
                ┌──▼──────┐
                │ assigned │
                └──┬───────┘
                   │ work starts
             ┌─────▼───────┐
             │ in-progress  │◄─────────────┐
             └──┬───────┬───┘              │
                │       │                  │
         done   │       │ blocked    unblocked
                │       │                  │
          ┌─────▼──┐  ┌─▼───────┐          │
          │ review │  │ blocked ├──────────┘
          └──┬─────┘  └─────────┘
             │ approved
          ┌──▼───┐
          │ done │
          └──────┘
```

Valid transitions:
- `open` → `assigned`
- `assigned` → `in-progress`
- `in-progress` → `review` | `blocked` | `done` (skip review for non-code tasks)
- `blocked` → `in-progress` (unblocked)
- `review` → `done` | `in-progress` (review rejected, needs rework)
- Any state → `cancelled`

## Issue ID Format

- Top-level: `LOOM-{number}` (e.g., `LOOM-001`)
- Sub-issues: `LOOM-{parent}-{sub}` (e.g., `LOOM-001-01`)
- Counter stored in `.loom/issues/counter.txt`

## Auto-Pickup

The daemon polls `.loom/issues/` every 2 seconds. When it detects:
- A new file with `status: open` and no `assignee`
- It sends a tmux notification to the orchestrator:
  ```
  [LOOM] New issue LOOM-003: "Fix API timeout". Run: loom issue show LOOM-003
  ```
- The orchestrator then decides how to handle it (assign to existing lead, spawn new lead, or queue it)

## Dependency Tracking

Issues can declare dependencies:
```yaml
depends_on:
  - LOOM-001-01    # must complete before this issue can start
```

When a lead assigns a task with unresolved dependencies:
- The issue stays in `assigned` state
- The lead is responsible for sequencing (not starting the worker until deps are met)
- The daemon does NOT enforce this — it's advisory

## CLI Examples

```bash
# Human creates a feature request
loom issue create "Add dark mode support" --type epic --priority normal

# Human creates a bug report
loom issue create "Login fails with special characters in password" --type bug --priority high

# Lead creates sub-tasks (from within kiro session)
loom issue create "Implement color theme provider" --type task --parent LOOM-005
loom issue create "Update all components to use theme tokens" --type task --parent LOOM-005 --depends-on LOOM-005-01

# Worker updates status
loom issue update LOOM-005-01 --status in-progress
loom issue update LOOM-005-01 --status done

# View the board
loom issue list
loom issue list --tree          # show parent/child hierarchy
loom issue list --tree LOOM-005 # show tree for one epic
```

## Tree View Output

```bash
$ loom issue list --tree LOOM-001
LOOM-001 [epic] [in-progress] Build authentication system (lead-auth)
├── LOOM-001-01 [task] [done] Login form validation (builder-017)
├── LOOM-001-02 [task] [review] JWT middleware (builder-018)
└── LOOM-001-03 [task] [assigned] Token refresh endpoint (builder-019)
    └── depends on: LOOM-001-02
```
