# Loom

A multi-agent orchestration system for kiro-cli. One binary, zero runtime dependencies.

Loom spawns and coordinates AI agents as ACP (Agent Control Protocol) subprocesses. An orchestrator breaks work into features, leads decompose features into tasks, and workers (builders, reviewers, explorers, researchers) execute in isolation using git worktrees. Agents communicate through an async mail system and share institutional knowledge through a built-in memory store.

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
- **Everything else**: stdlib + shelling out to git

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
├── cmd/loom/main.go              # Entrypoint (monolithic CLI)
├── internal/
│   ├── acp/                      # ACP client wrapper
│   ├── agent/                    # Agent lifecycle & registry
│   ├── cli/                      # CLI output helpers
│   ├── config/                   # Config loading & validation
│   ├── daemon/                   # Daemon process, API, lock, doctor
│   ├── dashboard/                # bubbletea TUI
│   │   └── backend/              # Dashboard data loading
│   ├── issue/                    # Issue tracker & merge
│   ├── lock/                     # File-level advisory locks
│   ├── mail/                     # Async mail system
│   ├── mcp/                      # MCP server (stdio JSON-RPC)
│   ├── memory/                   # Decision/discovery/convention store
│   ├── nudge/                    # Predefined nudge types
│   ├── store/                    # YAML file helpers
│   └── worktree/                 # Git worktree lifecycle
├── agents/                       # Embedded agent JSON configs (go:embed)
│   └── hooks/                    # Embedded hook scripts
├── templates/                    # Agent prompt templates (go:embed)
├── scripts/                      # Release tooling
├── docs/                         # Design documentation
├── go.mod
├── Makefile
└── README.md
```

## Status

**Active development.** Core systems (daemon, agents, issues, mail, memory, worktrees, MCP server, TUI dashboard) are implemented and functional. See docs/ for design details.
