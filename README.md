# Loom

A multi-agent orchestration system for kiro-cli. One binary, zero runtime dependencies.

Loom spawns and coordinates AI agents across tmux sessions. An orchestrator breaks work into features, leads decompose features into tasks, and workers (builders, reviewers, explorers, researchers) execute in isolation using git worktrees. Agents communicate through an async mail system and share institutional knowledge through a built-in memory store.

## Core Concepts

- **Orchestrator** — persistent kiro-cli session that receives issues and dispatches leads
- **Leads** — decompose features into tasks, manage workers, merge results
- **Workers** — builders (code in worktrees), reviewers, explorers, researchers
- **Mail** — async message passing; agents send only at critical points
- **Issues** — built-in tracker; humans create issues, orchestrator auto-picks them up
- **Memory** — shared decision log so agents understand *why* things are the way they are
- **Worktrees** — git worktrees in `.loom/worktrees/` for builder isolation

## System Requirements

- `git` ≥ 2.20
- `tmux` ≥ 3.0
- `kiro-cli`

## Install

```bash
curl -sSL https://raw.githubusercontent.com/kv0409/loom/main/install.sh | bash
```

To update to the latest version:

```bash
loom update
```

## Tech Stack

- **Language**: Go
- **External deps**: cobra (CLI), bubbletea+lipgloss (TUI), yaml.v3
- **Everything else**: stdlib + shelling out to git/tmux

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — system design, data flow, component overview
- [CLI Reference](docs/CLI.md) — all commands and flags
- [Agents](docs/AGENTS.md) — roles, lifecycle, prompt templates
- [Mail System](docs/MAIL.md) — message protocol, delivery, notifications
- [Issue Tracker](docs/ISSUES.md) — issue-driven workflow, states, auto-pickup
- [Memory System](docs/MEMORY.md) — decisions, discoveries, conventions
- [Worktree Management](docs/WORKTREES.md) — creation, cleanup, branch naming
- [Dashboard](docs/DASHBOARD.md) — TUI design, views, interactions
- [Integration](docs/INTEGRATION.md) — kiro-cli hooks, MCP server, extension points

## Project Structure

```
loom/
├── cmd/loom/main.go              # Entrypoint
├── internal/
│   ├── agent/                    # Agent lifecycle
│   ├── config/                   # Config loading
│   ├── dashboard/                # bubbletea TUI
│   ├── issue/                    # Issue tracker
│   ├── mail/                     # Mail system
│   ├── memory/                   # Decision/discovery store
│   ├── orchestrator/             # Core orchestration loop
│   ├── tmux/                     # tmux wrapper
│   ├── worktree/                 # Git worktree lifecycle
│   └── store/                    # YAML file helpers
├── mcp/                          # Loom MCP server (for agent integration)
├── templates/                    # Agent prompt templates (go:embed)
├── docs/                         # Design documentation
├── go.mod
├── Makefile
└── README.md
```

## Status

**Planning phase.** See docs/ for the full design.
