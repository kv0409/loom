# Dashboard

## Overview

Full TUI dashboard built with bubbletea + lipgloss. Provides real-time visibility into all aspects of the loom system.

## Launch

```bash
loom dash
```

Runs in the current terminal. Does NOT require the tmux session (reads from `.loom/` files directly).

## Main View

```
┌─ LOOM ─────────────────────────────────────────────────────────────────┐
│                                                                        │
│  AGENTS (5 active, 1 idle)                    ISSUES                   │
│  ┌──────────────┬──────────┬───────────────┐  ┌────────┬────────────┐  │
│  │ Agent        │ Role     │ Status        │  │ Status │ Count      │  │
│  ├──────────────┼──────────┼───────────────┤  ├────────┼────────────┤  │
│  │ orchestrator │ queen    │ ● idle        │  │ Open   │ ████ 2     │  │
│  │ lead-auth    │ lead     │ ● active      │  │ WIP    │ ██████ 3   │  │
│  │ builder-017  │ builder  │ ● active      │  │ Review │ ██ 1       │  │
│  │ builder-018  │ builder  │ ● active      │  │ Done   │ ████████ 4 │  │
│  │ builder-019  │ builder  │ ⚠ blocked     │  │ Block  │ █ 1        │  │
│  │ reviewer-003 │ reviewer │ ● active      │  └────────┴────────────┘  │
│  └──────────────┴──────────┴───────────────┘                           │
│                                                                        │
│  WORKTREES (3 active / 4 max)                 MAIL (2 unread)          │
│  ┌────────────────────────────┬─────────────┐ ┌────────────────────┐   │
│  │ loom-LOOM-001-01-login     │ builder-017 │ │ → builder-019      │   │
│  │ loom-LOOM-001-02-jwt       │ builder-018 │ │   BLOCKER: dep...  │   │
│  │ loom-LOOM-002-timeout      │ builder-019 │ │ → lead-auth        │   │
│  └────────────────────────────┴─────────────┘ │   complete: LOO... │   │
│                                                └────────────────────┘   │
│  MEMORY (5 entries)                           RECENT ACTIVITY          │
│  2 decisions · 2 discoveries · 1 convention   18:52 builder-017 commit │
│                                                18:51 reviewer-003 start│
│                                                18:50 builder-019 block │
│                                                                        │
│  [a]gents [i]ssues [m]ail [w]orktrees [d]ecisions [l]ogs [?]help     │
└────────────────────────────────────────────────────────────────────────┘
```

## Navigation

### Keyboard

| Key | Action |
|---|---|
| `a` | Switch to Agents view |
| `i` | Switch to Issues view |
| `m` | Switch to Mail view |
| `w` | Switch to Worktrees view |
| `d` | Switch to Memory/Decisions view |
| `l` | Switch to Logs view |
| `Tab` | Cycle through views |
| `Enter` | Drill into selected item |
| `Esc` / `Backspace` | Go back to parent view |
| `j` / `k` or `↓` / `↑` | Navigate list |
| `/` | Search within current view |
| `n` | Nudge selected agent (opens input) |
| `?` | Help overlay |
| `q` | Quit dashboard |

## Detail Views

### Agent Detail (Enter on an agent)

```
┌─ Agent: builder-017 ──────────────────────────────────────────────────┐
│                                                                        │
│  Role: builder          Status: ● active       Heartbeat: 3s ago      │
│  Spawned by: lead-auth  Spawned at: 18:40:00   PID: 54321             │
│  Tmux: loom:3.1                                                        │
│                                                                        │
│  ASSIGNED ISSUES                                                       │
│  └── LOOM-001-01 [in-progress] Login form validation                  │
│                                                                        │
│  WORKTREE: loom-LOOM-001-01-login-form                                │
│  Branch: loom/LOOM-001-01-login-form                                  │
│  Commits: 3 | Files changed: 4 (+120, -15)                            │
│                                                                        │
│  RECENT MAIL                                                           │
│  ← 18:40 lead-auth: task assignment                                   │
│  → 18:41 lead-auth: status (started)                                  │
│                                                                        │
│  LOCKS HELD                                                            │
│  src/components/LoginForm.tsx                                          │
│  src/utils/validation.ts                                               │
│                                                                        │
│  [a]ttach  [n]udge  [k]ill  [Esc] back                               │
└────────────────────────────────────────────────────────────────────────┘
```

### Issue Detail (Enter on an issue)

```
┌─ LOOM-001: Build authentication system ───────────────────────────────┐
│                                                                        │
│  Type: epic    Priority: high    Status: in-progress                  │
│  Assignee: lead-auth             Created: 18:34 by human              │
│                                                                        │
│  DESCRIPTION                                                           │
│  Implement JWT-based authentication with login, registration,         │
│  and token refresh endpoints. Include middleware for protected routes. │
│                                                                        │
│  CHILDREN                                                              │
│  ├── LOOM-001-01 [done]        Login form validation (builder-017)    │
│  ├── LOOM-001-02 [review]      JWT middleware (builder-018)           │
│  └── LOOM-001-03 [assigned]    Token refresh (builder-019)            │
│      └── depends on: LOOM-001-02                                      │
│                                                                        │
│  RELATED DECISIONS                                                     │
│  └── DEC-001: Use JWT for auth tokens instead of sessions             │
│                                                                        │
│  HISTORY                                                               │
│  18:34 created by human                                                │
│  18:35 assigned to lead-auth by orchestrator                          │
│  18:36 status → in-progress by lead-auth                              │
│                                                                        │
│  [Esc] back                                                            │
└────────────────────────────────────────────────────────────────────────┘
```

### Log View

```
┌─ Logs ────────────────────────────────────────────────────────────────┐
│  Filter: [all agents ▼]  [all levels ▼]  Search: [____________]      │
│                                                                        │
│  18:52:30 [builder-017] Committing changes to LoginForm.tsx           │
│  18:52:28 [reviewer-003] Starting review of LOOM-001-01              │
│  18:52:15 [builder-019] BLOCKED: LOOM-001-02 not yet merged          │
│  18:52:10 [lead-auth]   Assigned reviewer-003 to review LOOM-001-01  │
│  18:51:45 [builder-017] Mail sent: completion to lead-auth            │
│  18:51:30 [builder-018] Working on JWT middleware                     │
│  18:51:00 [orchestrator] All agents healthy                           │
│                                                                        │
│  [↑↓] scroll  [f] filter  [/] search  [Esc] back                    │
└────────────────────────────────────────────────────────────────────────┘
```

## Refresh

The dashboard polls `.loom/` files every 1 second for updates. No websockets, no IPC — just filesystem reads. This keeps it simple and means the dashboard can run from any terminal, not just within the tmux session.

## Interactions from Dashboard

| Action | How | Effect |
|---|---|---|
| Nudge agent | Select agent → `n` → type message → Enter | Sends tmux keys to agent's pane |
| Attach to agent | Select agent → `a` | Opens agent's tmux pane (exits dashboard) |
| Kill agent | Select agent → `k` → confirm | Stops agent, preserves worktree |
| Create issue | `i` view → `c` → fill form | Creates issue, triggers auto-pickup |
| Search memory | `d` view → `/` → type query | Keyword search across all memory |

## Color Scheme

Using lipgloss for consistent styling:

| Element | Color |
|---|---|
| Active/healthy | Green |
| Blocked/warning | Yellow/amber |
| Dead/error | Red |
| Idle | Dim/gray |
| Headers | Bold white |
| Selected item | Inverse/highlight |
| Borders | Subtle gray |

Respects terminal color capabilities (true color, 256, 16).
