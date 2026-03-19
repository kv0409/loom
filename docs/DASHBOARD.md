# Dashboard

## Overview

Full TUI dashboard built with bubbletea + lipgloss. Provides real-time visibility into all aspects of the loom system.

## Launch

```bash
loom dash
```

Runs in the current terminal. Reads from `.loom/` files directly.

## Views

The dashboard has these views, accessible via keyboard shortcuts or Tab cycling:

| View | Key | Description |
|---|---|---|
| Overview | `0` or `H` | Summary of agents, issues, activity, and attention items |
| Agents | `a` | Agent list with status, role, heartbeat |
| Issues | `i` | Issue list with status, assignee, priority |
| Memory | `d` | Decisions, discoveries, conventions |
| Activity | `t` | Recent agent activity and tool usage |
| Worktrees | `w` | Active worktrees with diff stats |

Tab cycles through: Overview → Agents → Issues → Memory → Activity → Worktrees.

Additional views accessible via drill-down (Enter):
- **Agent Detail** — from Agents view, shows assigned issues, worktree, mail, output
- **Issue Detail** — from Issues view, shows description, children, related context
- **Memory Detail** — from Memory view, shows full entry content
- **Mail Detail** — from Issue/Agent detail views
- **Diff** — from Worktrees view, shows worktree diff with horizontal scrolling

## Navigation

### Keyboard

| Key | Action |
|---|---|
| `0` or `H` | Switch to Overview |
| `a` | Switch to Agents view |
| `i` | Switch to Issues view |
| `d` | Switch to Memory/Decisions view |
| `t` | Switch to Activity view |
| `w` | Switch to Worktrees view |
| `Tab` | Cycle through views |
| `Enter` | Drill into selected item |
| `Esc` | Go back to parent view |
| `j` / `k` or `↓` / `↑` | Navigate list |
| `/` | Search/filter within current view |
| `q` | Quit dashboard |
| `Ctrl+C` | Quit dashboard |

### Agent-Specific Keys (Agents / Agent Detail views)

| Key | Action |
|---|---|
| `n` | Nudge selected agent (opens nudge type selector) |
| `m` | Message selected agent (opens text input) |
| `o` | View agent output (Agent Detail only) |
| `x` | Kill agent (with confirmation) |

### Diff View Keys

| Key | Action |
|---|---|
| `j` / `k` or `↓` / `↑` | Scroll vertically |
| `h` / `l` or `←` / `→` | Scroll horizontally |

## Search / Filter

Press `/` in any list view to activate search. Type a query and press Enter to filter. The search matches against visible fields (agent names, issue titles/descriptions, memory content, etc.). Press Esc to clear the filter.

## Compose Overlay

Press `Ctrl+S` to send a composed mail message. The compose form (built with `huh`) allows setting To, Subject, Body, Type, and Priority fields.

## Refresh

The dashboard polls `.loom/` files every 2 seconds for updates. No websockets, no IPC — just filesystem reads. This keeps it simple and means the dashboard can run from any terminal.

## Color Scheme

Uses the Tokyo Night truecolor palette via lipgloss:

| Element | Color |
|---|---|
| Active/healthy | Green (`#9ECE6A`) |
| Blocked/warning | Yellow (`#E0AF68`) |
| Dead/error | Red (`#F7768E`) |
| Idle | Gray (`#565F89`) |
| Primary/selected | Blue (`#7AA2F7`) |
| Review/info | Cyan (`#7DCFFF`) |
| Lead | Magenta (`#BB9AF7`) |
| Borders | Subtle (`#414868`) |

Lip Gloss automatically downgrades truecolor to ANSI 256 or 16-color in lesser terminals.
