# Architecture

## Overview

Loom is a standalone CLI tool (single Go binary) that orchestrates multiple kiro-cli agent sessions via ACP (Agent Control Protocol). It provides built-in issue tracking, async mail, shared memory, worktree management, and a TUI dashboard.

## System Diagram

```
                    ┌──────────────────┐
                    │   Human (User)   │
                    └────────┬─────────┘
                             │
                    loom issue create / loom task / loom nudge
                             │
                    ┌────────▼─────────┐
                    │   Loom CLI/Daemon │◄──── loom dash (TUI)
                    │   (Go binary)    │
                    └────────┬─────────┘
                             │
              Writes to .loom/issues/ + manages ACP subprocesses
                             │
              ┌──────────────▼──────────────┐
              │  ACP-managed agent processes │
              │                              │
              │  ┌────────────────────────┐  │
              │  │ Orchestrator           │  │
              │  │ (persistent kiro-cli)  │  │
              │  └───────────┬────────────┘  │
              │              │               │
              │    Reads issues, spawns leads │
              │              │               │
              │  ┌───────────▼────────────┐  │
              │  │ Lead Agents            │  │
              │  │ (kiro-cli per feature) │  │
              │  └───────────┬────────────┘  │
              │              │               │
              │    Decomposes into tasks,    │
              │    spawns workers            │
              │              │               │
              │  ┌───────────▼────────────┐  │
              │  │ Workers (kiro-cli each)│  │
              │  │ builder → worktree     │  │
              │  │ reviewer → reads code  │  │
              │  │ explorer → searches    │  │
              │  │ researcher → web/docs  │  │
              │  └────────────────────────┘  │
              └──────────────────────────────┘
```

## Data Flow

### Issue-Driven Workflow

```
1. Human runs: loom issue create "Build auth system" --priority high
2. Loom CLI writes .loom/issues/LOOM-001.yaml
3. Loom daemon (watcher goroutine) detects new issue file
4. Daemon sends ACP prompt to orchestrator: "[LOOM] New issue LOOM-001. Run: loom issue show LOOM-001"
5. Orchestrator reads issue, creates a plan, spawns lead agent
6. Lead decomposes into sub-issues (LOOM-001-01, LOOM-001-02, ...)
7. Lead spawns workers, assigns sub-issues
8. Workers execute, report via mail
9. Lead merges worktrees, closes sub-issues
10. Lead reports completion to orchestrator via mail
11. Orchestrator closes parent issue
```

### Agent Communication Flow

```
Agent A                    .loom/mail/              Agent B
   │                                                   │
   ├─ loom mail send B ──► inbox/B/timestamp.yaml      │
   │                           │                       │
   │                    Daemon detects new file         │
   │                           │                       │
   │                    ACP prompt to B's process       │
   │                           │                       │
   │                           └──────────────────────► │
   │                                    "[LOOM] Mail"   │
   │                                                    │
   │                                    B reads mail    │
   │                                    B processes     │
   │                                    B responds      │
```

## Component Architecture

### Loom Binary (single process)

```
┌─────────────────────────────────────────────┐
│ loom binary                                 │
│                                             │
│  ┌─────────┐  ┌──────────┐  ┌───────────┐  │
│  │ CLI     │  │ Daemon   │  │ Dashboard │  │
│  │ (cobra) │  │ (re-exec)│  │ (bubbletea│  │
│  └────┬────┘  └────┬─────┘  └─────┬─────┘  │
│       │            │               │        │
│  ┌────▼────────────▼───────────────▼─────┐  │
│  │          Internal Modules             │  │
│  │                                       │  │
│  │  agent/   issue/   mail/    memory/   │  │
│  │  acp/     config/  store/   worktree/ │  │
│  │  mcp/     lock/    nudge/   cli/      │  │
│  └───────────────────┬───────────────────┘  │
│                      │                      │
│              ┌───────▼───────┐              │
│              │  .loom/ (fs)  │              │
│              └───────────────┘              │
└─────────────────────────────────────────────┘
```

### Daemon

The daemon self-daemonizes by re-exec with `LOOM_DAEMON=1` env var (not fork). It:
1. Polls `.loom/issues/` for new/changed issue files (every 2s)
2. Polls `.loom/mail/inbox/*/` for new messages (every 2s)
3. Polls for pending-acp agents to activate (every 2s)
4. Monitors ACP output from active agents (every 1s)
5. Checks agent heartbeats (every 30s)
6. Runs worktree garbage collection (every 5min)
7. Delivers notifications via ACP prompts
8. Cleans up dead agents (stale heartbeat > configurable timeout)
9. Auto-shuts down after idle timeout (default 5min with no active agents/issues)

The daemon exposes a Unix socket API at `.loom/daemon.sock` for commands like nudge, kill, cancel, and message delivery.

### MCP Server (Optional, Recommended)

A Loom MCP server using stdio JSON-RPC transport that agents connect to, providing native tool access:
- `loom_mail_send` / `loom_mail_read` / `loom_mail_count`
- `loom_issue_show` / `loom_issue_update` / `loom_issue_create` / `loom_issue_close` / `loom_issue_list`
- `loom_memory_add` / `loom_memory_search` / `loom_memory_list`
- `loom_lock_acquire` / `loom_lock_release` / `loom_lock_check`
- `loom_agent_heartbeat` / `loom_agent_status` / `loom_agent_kill`
- `loom_worktree_remove`

This is cleaner than agents shelling out via execute_bash. Each agent gets its own MCP server instance (`loom mcp-server --agent-id <id>`) that knows the agent's identity.

See [Integration](INTEGRATION.md) for details.

## .loom/ Directory Structure

```
.loom/
├── config.yaml                    # System configuration
├── loom.lock                      # PID lock file (prevent double-start)
├── daemon.sock                    # Unix socket for daemon API
│
├── agents/                        # Agent registry + state
│   ├── orchestrator.yaml          # {id, role, status, pid, heartbeat}
│   ├── lead-auth.yaml
│   └── builder-017.yaml
│
├── issues/                        # Built-in issue tracker
│   ├── counter.txt                # Next issue number
│   ├── LOOM-001.yaml              # Individual issues with full history
│   ├── LOOM-001-01.yaml           # Sub-issues
│   └── LOOM-002.yaml
│
├── mail/                          # Async message system
│   ├── inbox/
│   │   ├── orchestrator/          # One folder per agent
│   │   ├── lead-auth/
│   │   └── builder-017/
│   └── archive/                   # Processed messages (audit trail)
│
├── memory/                        # Shared knowledge base
│   ├── decisions/                 # "We chose X because Y"
│   ├── discoveries/               # "Found that module X does Y"
│   └── conventions/               # "All API handlers follow this pattern"
│
├── worktrees/                     # Git worktrees (builder isolation)
│   ├── LOOM-001-01-login-form/
│   └── LOOM-002-api-timeout/
│
├── plans/                         # Feature breakdowns from leads (reserved)
│
├── artifacts/                     # Agent outputs (reserved)
│   ├── reviews/
│   ├── research/
│   └── patches/
│
├── locks/                         # File-level locks (conflict prevention)
│   └── src__auth__login.ts.lock.yaml  # Path encoded with __ separators
│
├── logs/                          # Per-agent session logs
│   ├── orchestrator.log
│   └── builder-017.log
│
└── templates/                     # Agent prompt templates (copied from embedded defaults)
    ├── orchestrator.md            # Customizable per-project
    ├── lead.md
    ├── builder.md
    ├── reviewer.md
    ├── explorer.md
    └── researcher.md
```

## Concurrency Model

- Each agent is a separate kiro-cli ACP subprocess managed by the daemon
- The loom daemon manages them via goroutines (one monitor goroutine per agent)
- File-based communication (YAML files in .loom/) — no shared memory, no sockets between agents
- File locks in `.loom/locks/` prevent conflicting edits across builders
- The daemon is the only process that sends ACP prompts for notifications

## Error Recovery

### Agent Crash
1. Heartbeat monitor detects stale agent (no heartbeat update > timeout)
2. Daemon marks agent as `dead` in agents/*.yaml
3. Daemon notifies the agent's lead (or orchestrator if it's a lead)
4. Lead can re-spawn a replacement worker or reassign the task

### Daemon Crash
1. `loom start` checks for existing `.loom/loom.lock`
2. If lock exists but process is dead → clean up orphaned state
3. Re-read all issue/agent/mail state from disk (it's all files)
4. Resume monitoring

### Worktree Cleanup on Failure
1. Each worktree is tracked in the agent's YAML (agent → worktree mapping)
2. When an agent dies, its worktree is preserved (not auto-deleted)
3. `loom worktree cleanup` removes worktrees for dead/deregistered agents
4. Human can also `loom worktree show <name>` to inspect before cleanup

## Configuration

```yaml
# .loom/config.yaml
project: my-app
limits:
  max_agents: 8
  max_worktrees: 4
  max_agents_per_lead: 3
  heartbeat_timeout_seconds: 300
  idle_timeout_seconds: 600
merge:
  strategy: squash        # squash | merge | rebase
  auto_delete_branch: true
  require_review: true
polling:
  issue_interval_ms: 2000
  mail_interval_ms: 2000
  heartbeat_interval_ms: 30000
  pending_agents_interval_ms: 2000
  acp_output_interval_ms: 1000
  worktree_gc_interval_ms: 300000
  idle_shutdown_seconds: 300
kiro:
  command: kiro-cli       # path to kiro-cli binary
  default_mode: acp       # must be "acp"
models:                   # per-role model selection
  orchestrator: sonnet
  lead: opus
  builder: opus
  reviewer: opus
  explorer: haiku
  researcher: sonnet
mcp:
  enabled: true           # run built-in MCP server for agents
  port: 0                 # 0 = auto-assign (stdio transport)
deny:                     # tool/command deny lists for agents
  tools: []
  commands:
    - "git merge*"
    - "git push*"
    - "git reset --hard*"
    # ... (see config.go DefaultConfig for full list)
```
