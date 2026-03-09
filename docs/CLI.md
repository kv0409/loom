# CLI Reference

## Lifecycle

### `loom init`
Initialize `.loom/` in the current git repository.
- Creates directory structure
- Copies default templates to `.loom/templates/`
- Adds `.loom/` to `.gitignore`
- Creates default `config.yaml`

```bash
loom init
loom init --config path/to/config.yaml   # use custom config
```

### `loom start`
Launch the orchestrator and daemon.
- Creates tmux session `loom`
- Spawns orchestrator kiro-cli in window 0
- Starts daemon (issue watcher, mail notifier, heartbeat monitor)
- If `.loom/hive.lock` exists from a crash, offers to resume or start fresh

```bash
loom start
loom start --resume          # auto-resume without prompting
loom start --fresh           # discard previous state
```

### `loom stop`
Graceful shutdown.
- Sends shutdown signal to all agents
- Waits for in-progress work to reach a checkpoint (configurable timeout)
- Archives active mail
- Preserves worktrees (does NOT auto-delete)
- Kills tmux session
- Removes `hive.lock`

```bash
loom stop
loom stop --force            # kill immediately, no graceful wait
loom stop --cleanup          # also remove worktrees
```

### `loom status`
Quick health check (no TUI, just stdout).

```bash
loom status
# Output:
# Loom: running (pid 12345)
# Agents: 5 active, 1 idle, 0 dead
# Issues: 3 open, 2 in-progress, 7 done
# Worktrees: 2 active
# Mail: 1 undelivered
```

---

## Issues

### `loom issue create`
Create a new issue. The orchestrator auto-picks it up if running.

```bash
loom issue create "Build authentication system"
loom issue create "Build auth" --priority high --type epic
loom issue create "Fix login bug" --type bug --parent LOOM-001
loom issue create "Research OAuth providers" --type spike
```

Flags:
- `--priority` — critical | high | normal (default) | low
- `--type` — epic | task (default) | bug | spike
- `--parent` — parent issue ID (for sub-issues)
- `--description` or `-d` — longer description (opens $EDITOR if omitted and --no-edit not set)
- `--no-edit` — skip editor, use title as full description

### `loom issue list`
List issues with filters.

```bash
loom issue list                              # all open issues
loom issue list --status in-progress         # by status
loom issue list --assignee builder-017       # by assignee
loom issue list --type epic                  # by type
loom issue list --all                        # include closed
```

### `loom issue show`
Show full issue detail including history.

```bash
loom issue show LOOM-001
```

### `loom issue update`
Update issue fields.

```bash
loom issue update LOOM-001 --status in-progress
loom issue update LOOM-001 --priority critical
loom issue update LOOM-001 --assignee lead-auth
```

### `loom issue close`
Close an issue.

```bash
loom issue close LOOM-001
loom issue close LOOM-001 --reason "Completed, merged in worktree loom-LOOM-001"
```

---

## Agents

### `loom agents`
List all agents.

```bash
loom agents
# Output:
# ID            ROLE      STATUS    WORKTREE              ISSUES    HEARTBEAT
# orchestrator  queen     idle      —                     —         2s ago
# lead-auth     lead      planning  —                     LOOM-001  5s ago
# builder-017   builder   coding    loom-LOOM-001-01-..   LOOM-001-01  3s ago
# builder-019   builder   blocked   loom-LOOM-001-02-..   LOOM-001-02  45s ago
```

### `loom agent show <name>`
Detail view of one agent.

```bash
loom agent show builder-017
```

### `loom attach <name>`
Attach to an agent's tmux pane. You can observe and type directly into their kiro session.

```bash
loom attach builder-017
# Ctrl+B D to detach back to your terminal
```

### `loom nudge <name> <message>`
Send an inline message to an agent's kiro session (via tmux send-keys).

```bash
loom nudge builder-017 "Try using the existing auth middleware in src/middleware/auth.ts"
```

### `loom kill <name>`
Force-stop an agent. Cleans up agent registration but preserves worktree.

```bash
loom kill builder-019
loom kill builder-019 --cleanup    # also remove worktree
```

---

## Mail

### `loom mail send`
Send a message to an agent.

```bash
loom mail send orchestrator "Feature auth complete" --body "All sub-issues closed. Ready for final review."
loom mail send lead-auth "Blocker on LOOM-001-02" --type blocker --ref LOOM-001-02
```

Flags:
- `--type` — task | status | completion | blocker | review-request | question (default: status)
- `--priority` — critical | normal (default) | low
- `--ref` — reference issue ID
- `--body` or `-b` — message body (opens $EDITOR if omitted)

### `loom mail read`
Read inbox.

```bash
loom mail read                     # your inbox (when run by human, shows orchestrator inbox)
loom mail read builder-017         # specific agent's inbox
loom mail read --unread            # only unread
```

### `loom mail log`
Full message history (archived + active).

```bash
loom mail log
loom mail log --agent lead-auth    # filtered by sender/recipient
loom mail log --type blocker       # filtered by type
loom mail log --since 1h           # time filter
```

---

## Memory

### `loom memory add`
Record a decision, discovery, or convention.

```bash
loom memory add decision "Use JWT for auth tokens" \
  --context "Need stateless auth for API" \
  --rationale "No additional infrastructure, industry standard" \
  --affects LOOM-001,LOOM-003

loom memory add discovery "Auth module uses singleton pattern" \
  --source explorer-05

loom memory add convention "All API handlers return {data, error} shape"
```

### `loom memory search`
Search across all memory entries.

```bash
loom memory search "auth"
loom memory search "middleware pattern"
```

Search is keyword-based (BM25-style scoring on title + context + rationale + decision fields). No external dependencies.

### `loom memory list`
Browse memory entries.

```bash
loom memory list
loom memory list --type decisions
loom memory list --affects LOOM-001
```

### `loom memory show`
Show full detail of a memory entry.

```bash
loom memory show DEC-001
```

---

## Worktrees

### `loom worktree list`
List active worktrees.

```bash
loom worktree list
# Output:
# WORKTREE                        AGENT        ISSUE        BRANCH
# loom-LOOM-001-01-login-form     builder-017  LOOM-001-01  loom/LOOM-001-01-login-form
# loom-LOOM-002-api-timeout       builder-019  LOOM-002     loom/LOOM-002-api-timeout
```

### `loom worktree show <name>`
Show worktree detail (path, branch, agent, diff stats).

### `loom worktree cleanup`
Remove worktrees for dead/deregistered agents.

```bash
loom worktree cleanup              # interactive: confirm each
loom worktree cleanup --force      # remove all orphaned without prompting
```

---

## Dashboard

### `loom dash`
Launch the TUI dashboard.

```bash
loom dash
```

See [Dashboard](DASHBOARD.md) for the full TUI design.

---

## Logs

### `loom log`
View agent logs.

```bash
loom log orchestrator              # tail one agent's log
loom log --all                     # interleaved stream from all agents
loom log --all --since 5m          # last 5 minutes
```

---

## Configuration

### `loom config show`
Display current configuration.

### `loom config set`
Update a config value.

```bash
loom config set limits.max_agents 12
loom config set merge.strategy rebase
```

---

## Utility

### `loom gc`
Garbage collection: archive old mail, prune merged branches, clean logs.

```bash
loom gc
loom gc --dry-run                  # show what would be cleaned
```

### `loom export`
Export a summary of work done (for PR descriptions, standups, etc.).

```bash
loom export                        # summary of all closed issues + decisions
loom export --issue LOOM-001       # summary for one feature
loom export --format markdown      # markdown (default) | json
```

### `loom mcp-server`
Start the built-in MCP server (used by agents, not typically run manually).

```bash
loom mcp-server                    # starts on configured port
loom mcp-server --port 9876        # explicit port
```
