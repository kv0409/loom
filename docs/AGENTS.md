# Agents

## Roles

### Orchestrator (Queen)
- **Count**: Exactly 1, always running
- **Spawned by**: `loom start`
- **Purpose**: Receives issues, creates plans, spawns leads, monitors progress
- **Writes code**: Never
- **Worktree**: None (works in main repo, read-only)

### Lead
- **Count**: 1 per epic/major feature
- **Spawned by**: Orchestrator
- **Purpose**: Decomposes feature into tasks, spawns workers, manages merges, resolves conflicts
- **Writes code**: Only merge conflict resolution
- **Worktree**: None (coordinates, doesn't build)

### Builder
- **Count**: 1+ per lead (up to `max_agents_per_lead`)
- **Spawned by**: Lead
- **Purpose**: Implements one task in isolation
- **Writes code**: Yes, exclusively in assigned worktree
- **Worktree**: One dedicated worktree per builder

### Reviewer
- **Count**: 1 per review request
- **Spawned by**: Lead
- **Purpose**: Reviews builder's work, reports findings
- **Writes code**: Never
- **Worktree**: Reads builder's worktree (read-only)

### Explorer
- **Count**: As needed
- **Spawned by**: Lead or Orchestrator
- **Purpose**: Searches codebase, finds patterns, traces call chains, gathers context
- **Writes code**: Never
- **Worktree**: None (reads main repo)

### Researcher
- **Count**: As needed
- **Spawned by**: Lead or Orchestrator
- **Purpose**: External research — docs, best practices, OSS examples
- **Writes code**: Never
- **Worktree**: None

---

## Agent Lifecycle

```
                    ┌─────────────┐
                    │ PENDING-ACP │
                    └──────┬──────┘
                           │
              Create agent YAML in .loom/agents/
              Create mailbox in .loom/mail/inbox/
                           │
                    ┌──────▼──────┐
                    │ ACTIVATING  │
                    └──────┬──────┘
                           │
              Daemon creates ACP client subprocess
              Send agent's prompt via ACP
                           │
                    ┌──────▼──────┐
                    │   ACTIVE    │
                    └──────┬──────┘
                           │
              Agent works, sends heartbeats
              Sends/receives mail
              Updates issues
                           │
              (heartbeat timeout or process exit)
                           │
                    ┌──────▼──────┐
                    │    DEAD     │
                    └──────┬──────┘
                           │
              Daemon notifies parent agent
              Worktree preserved for inspection
              Inbox GC'd on deregister
                           │
                    ┌──────▼──────┐
                    │    GONE     │
                    └─────────────┘
```

### Agent States
- `pending-acp` — registered, waiting for daemon to create ACP session
- `activating` — ACP session being initialized
- `active` — working on a task
- `idle` — ready, waiting for work
- `dead` — crashed or timed out (detected by heartbeat monitor)

---

## Agent YAML Schema

```yaml
# .loom/agents/builder-017.yaml
id: builder-017
role: builder                    # orchestrator | lead | builder | reviewer | explorer | researcher
status: active                   # pending-acp | activating | active | idle | dead
pid: 54321                       # kiro-cli process ID
spawned_by: lead-auth            # parent agent
spawned_at: 2026-03-09T18:40:00-04:00
heartbeat: 2026-03-09T18:52:30-04:00
assigned_issues:
  - LOOM-001-01
worktree: LOOM-001-01-login-form
acp_session_id: "sess-abc123"    # ACP session identifier
initial_task: "Implement login form validation"
nudge_count: 0
last_nudge: null
file_scope:                      # file/directory scope hints (from --scope)
  - src/auth/login.ts
  - src/auth/types.ts
config:
  kiro_mode: acp
  mcp_enabled: true
  model: opus                    # model override (from --model or config)
```

---

## Agent Spawning

When an agent is spawned (via `loom spawn`):

1. Generate agent ID: `{role}-{3-digit-counter}` (e.g., `builder-017`). Exception: `orchestrator` always returns `"orchestrator"`.
2. Write agent YAML to `.loom/agents/{id}.yaml` with `status: pending-acp`
3. Create mailbox directory `.loom/mail/inbox/{id}/`
4. If builder: create worktree (see [Worktrees](WORKTREES.md))
5. If builder and `--scope` was provided: store file-scope hints in agent YAML (`file_scope` field)
6. Daemon's `watchPendingAgents()` detects the pending-acp agent, sets status to `activating`, and creates an ACP client subprocess
7. Send the agent's prompt via ACP protocol, status becomes `active`

### Prompt Injection

The initial prompt sent to each agent includes:
- Their role and identity (agent ID)
- The task/issue they're assigned to
- Instructions on using `loom` CLI (or MCP tools if enabled)
- When to send mail (critical points only)
- How to record decisions in memory
- How to update issue status
- Constraints and conventions from the project
- File-scope hints (if provided via `--scope` at spawn time)

Templates live in `.loom/templates/` and are rendered with Go's `text/template` using agent-specific variables.

---

## Heartbeat Protocol

Every agent is instructed (via prompt) to periodically run:
```bash
loom agent heartbeat
```
(or via MCP: `loom_agent_heartbeat` tool)

This updates the `heartbeat` timestamp in the agent's YAML.

The daemon checks heartbeats every 30s (configurable). If an agent's heartbeat is older than `heartbeat_timeout_seconds` (default 300s / 5min), the daemon:
1. Marks agent as `dead`
2. Notifies the agent's parent (lead or orchestrator)
3. Does NOT auto-cleanup (preserves worktree for inspection)

---

## Agent Hierarchy

```
orchestrator
├── lead-auth (LOOM-001: Build auth system)
│   ├── explorer-001 (codebase exploration)
│   ├── builder-017 (LOOM-001-01: login form)
│   ├── builder-018 (LOOM-001-02: JWT middleware)
│   └── reviewer-003 (review builder-017's work)
├── lead-api (LOOM-002: API refactor)
│   ├── researcher-001 (research REST best practices)
│   ├── builder-019 (LOOM-002-01: endpoint restructure)
│   └── builder-020 (LOOM-002-02: error handling)
```

Each lead manages its own workers. The orchestrator only talks to leads. Leads only talk to their workers and the orchestrator. Workers only talk to their lead.

Exception: any agent can escalate directly to the orchestrator via mail with type `escalation`.
