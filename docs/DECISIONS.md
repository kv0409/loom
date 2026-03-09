# Design Decisions Log

Decisions made during the planning session on 2026-03-09.

## DD-001: Name — Loom

**Decision**: Name the system "Loom", CLI `loom`, folder `.loom/`.
**Rationale**: 4 chars, no conflicts with existing tools, rich metaphor (threads of work woven into fabric), good keyboard feel (right-hand roll).
**Alternatives considered**: Hive, Forge, Fleet, Cadre, Lattice, Grove, Pulse, Nerve.

## DD-002: Language — Go

**Decision**: Build in Go.
**Rationale**: Goroutines + channels map perfectly to agent orchestration. bubbletea is best-in-class TUI. Single binary, no runtime. 3-4x faster development than Rust for this use case. The bottleneck is LLM inference, not the orchestrator.
**Alternatives considered**: Rust (overkill perf, slower dev), TypeScript (requires runtime).

## DD-003: Agent Sessions — Interactive kiro-cli in tmux

**Decision**: Spawn agents as regular interactive kiro-cli sessions in tmux panes.
**Rationale**: Agents need bidirectional communication (receive mail/nudges, send updates). `--no-interactive` blocks inbound. Interactive + tmux send-keys enables both directions. The `loom` CLI becomes the universal interface for humans and agents.
**Alternatives considered**: `--no-interactive` (no inbound), ACP mode (unclear support).

## DD-004: Communication — File-based async mail

**Decision**: YAML files in `.loom/mail/inbox/` with daemon polling + tmux send-keys notification.
**Rationale**: No sockets, no database, fully inspectable, auditable. Daemon polls every 2s and notifies agents via tmux. Simple, robust, zero additional dependencies.
**Alternatives considered**: Unix sockets (complex), shared memory (fragile), database (overkill).

## DD-005: Data Format — YAML

**Decision**: Use YAML for all data files (issues, agents, mail, memory, config).
**Rationale**: Human-readable and editable. Agents and humans both interact with these files. Worth the one dependency (yaml.v3).
**Alternatives considered**: JSON (stdlib but ugly for multi-line content), TOML (less common), custom format (maintenance burden).

## DD-006: Minimal Dependencies

**Decision**: 3 external dependency trees only: cobra, bubbletea ecosystem, yaml.v3.
**Rationale**: User requirement. Everything else from Go stdlib or shelling out to git/tmux. Keeps binary small (~8-10MB), build fast, supply chain minimal.

## DD-007: Agent Integration — MCP Server (primary) + CLI (fallback)

**Decision**: Build a Loom MCP server that agents connect to for native tool access. CLI shelling as fallback.
**Rationale**: MCP is cleaner than execute_bash for every operation. Agents get typed tools (loom_mail_send, loom_issue_update, etc.) instead of string-based CLI calls. CLI fallback ensures it works even without MCP support.

## DD-008: Issue-Driven Orchestration

**Decision**: Humans create issues via `loom issue create`. The daemon detects new issues and notifies the orchestrator, which auto-picks them up.
**Rationale**: Decouples human intent from agent execution. Human doesn't need to interact with the orchestrator directly — just create an issue and walk away. The orchestrator is always watching.

## DD-009: Memory System — File-based with keyword search

**Decision**: Decisions, discoveries, and conventions stored as YAML files. Search via built-in BM25-style keyword scoring.
**Rationale**: No vector DB, no embeddings, no external service. Keyword search on structured fields (title, context, rationale) is sufficient for the scale of a single project session. Can upgrade to semantic search later if needed.

## DD-010: Worktrees in .loom/worktrees/

**Decision**: All git worktrees stored under `.loom/worktrees/` with branch prefix `loom/`.
**Rationale**: Keeps main directory clean. `.loom/` is gitignored. `loom/` branch prefix makes cleanup easy (`git branch --list 'loom/*'`). Worktree name includes issue ID for traceability.
