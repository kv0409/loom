# CLI Reference

## Initialization

### `loom init`
Initialize `.loom/` in the current git repository.
- Creates directory structure
- Copies default templates to `.loom/templates/`
- Installs agent configs and hook scripts
- Adds `.loom/` to `.gitignore`
- Creates default `config.yaml`

```bash
loom init
loom init --force                # overwrite existing .loom/ directory
loom init --refresh              # update templates, agents, and hooks without wiping state
```

---

## Lifecycle

### `loom start`
Launch the orchestrator and daemon.
- Spawns orchestrator kiro-cli as ACP subprocess
- Starts daemon (issue watcher, mail notifier, heartbeat monitor)
- If `.loom/loom.lock` exists from a crash, checks if the process is alive

```bash
loom start
loom start --resume              # auto-resume without prompting
loom start --fresh               # discard previous state
loom start --no-dashboard        # skip auto-opening the dashboard
```

### `loom stop`
Graceful shutdown.
- Sends shutdown signal to all agents
- Kills agent processes
- Removes `loom.lock`

```bash
loom stop
loom stop --force                # send SIGKILL instead of SIGTERM
loom stop --daemon-only          # stop only the daemon; leave agents running
loom stop --clean                # also remove all worktrees including unmerged branches
```

### `loom restart`
Restart the daemon.

```bash
loom restart
loom restart --no-dashboard      # skip auto-opening the dashboard
```

### `loom status`
Quick health check (no TUI, just stdout).

```bash
loom status
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
loom issue create "Subtask" --parent LOOM-001 --depends-on LOOM-001-01,LOOM-001-02
```

Flags:
- `--priority` — critical | high | normal (default) | low
- `--type` — epic | task (default) | bug | spike
- `--parent` — parent issue ID (for sub-issues)
- `--description` or `-d` — longer description
- `--depends-on` — comma-separated dependency issue IDs
- `--dispatch` — comma-separated key=value dispatch directives

### `loom issue list`
List issues with filters.

```bash
loom issue list                              # all open issues
loom issue list --status in-progress         # by status
loom issue list --assignee builder-017       # by assignee
loom issue list --type epic                  # by type
loom issue list --all                        # include closed/cancelled
loom issue list --ready                      # only dependency-ready issues
loom issue list --tree                       # show parent/child hierarchy
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
loom issue update LOOM-001 --unassign
loom issue update LOOM-001 --dispatch role=builder,scope=src/auth
```

Flags:
- `--status` — new status
- `--priority` — new priority
- `--assignee` — new assignee agent ID
- `--unassign` — remove current assignee
- `--dispatch` — comma-separated key=value dispatch directives

### `loom issue close`
Close an issue.

```bash
loom issue close LOOM-001
loom issue close LOOM-001 --reason "Completed, merged in worktree"
```

---

## Task

### `loom task`
Create a task from natural language (shorthand for issue creation).

```bash
loom task "Fix the login timeout bug"
```

---

## Agents

### `loom agents`
List all agents.

```bash
loom agents
```

### `loom agent show <name>`
Detail view of one agent.

```bash
loom agent show builder-017
```

### `loom agent heartbeat`
Update the calling agent's heartbeat timestamp. Typically called by agents, not humans.

```bash
loom agent heartbeat
```

### `loom agent cancel <name>`
Cancel an in-progress ACP prompt for an agent.

```bash
loom agent cancel builder-017
```

### `loom nudge <agent> <type>`
Send a predefined nudge signal to an agent via ACP prompt.

Available nudge types:
- `check-inbox` — remind agent to check mail
- `heartbeat-stale` — remind agent to send heartbeat
- `child-needs-attention` — alert lead about a child agent
- `resume-work` — prompt agent to resume work
- `report-status` — request status update

```bash
loom nudge builder-017 resume-work
loom nudge lead-auth check-inbox
```

### `loom kill <name>`
Force-stop an agent. Cleans up agent registration but preserves worktree.

```bash
loom kill builder-019
loom kill builder-019 --cleanup    # also remove worktree
```

### `loom spawn`
Spawn a new agent. Typically called by other agents, not humans.

```bash
loom spawn --role builder --issues LOOM-001-01 --spawned-by lead-auth --slug login-form
loom spawn --role reviewer --issues LOOM-001-01 --spawned-by lead-auth
loom spawn --role builder --issues LOOM-001-02 --slug jwt --model opus --scope src/auth/jwt.ts
```

Flags:
- `--role` — agent role: lead | builder | reviewer | explorer | researcher
- `--issues` — comma-separated issue IDs to assign
- `--spawned-by` — parent agent ID (defaults to `LOOM_AGENT_ID` env var)
- `--slug` — worktree slug for builders
- `--task` — custom task message for the agent
- `--model` — model override: sonnet | opus | haiku (default: from config)
- `--scope` — comma-separated file/directory scope hints for builders
- `--dispatch` — comma-separated key=value dispatch directives

---

## Mail

### `loom mail send`
Send a message to an agent.

```bash
loom mail send orchestrator "Feature auth complete" --body "All sub-issues closed."
loom mail send lead-auth "Blocker on LOOM-001-02" --type blocker --ref LOOM-001-02
```

Flags:
- `--type` — task | status (default) | completion | blocker | review-request | question
- `--priority` — critical | normal (default) | low
- `--from` — sender (default: "human")
- `--ref` — reference issue ID
- `--body` or `-b` — message body

### `loom mail read`
Read inbox.

```bash
loom mail read                     # orchestrator inbox
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
  --rationale "No additional infrastructure" \
  --decision "JWT with RS256 signing" \
  --affects LOOM-001,LOOM-003

loom memory add discovery "Auth module uses singleton pattern" \
  --finding "Private constructor with static getInstance()" \
  --location "src/auth/provider.ts:15-42" \
  --source explorer-05

loom memory add convention "All API handlers return {data, error} shape" \
  --rule "Every endpoint returns { data, error } shape"
```

Flags:
- `--context` — context (decisions)
- `--rationale` — rationale (decisions)
- `--decision` — decision text
- `--finding` — finding text (discoveries)
- `--rule` — rule text (conventions)
- `--location` — code location (discoveries)
- `--affects` — comma-separated affected issue IDs
- `--tags` — comma-separated tags
- `--source` — author (sets decided_by/discovered_by/established_by)

### `loom memory search`
Search across all memory entries.

```bash
loom memory search "auth"
loom memory search "middleware pattern" --limit 10
```

### `loom memory list`
Browse memory entries.

```bash
loom memory list
loom memory list --type decision
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

## Merge

### `loom merge <issue-id>`
Squash-merge an issue's worktree branch into the default branch.

```bash
loom merge LOOM-001-01
loom merge LOOM-001-01 -m "feat(auth): login form validation"
loom merge LOOM-001-01 --cleanup   # also remove worktree and branch after merge
```

### `loom merges`
Show merge queue status.

```bash
loom merges
```

---

## Finding

### `loom finding <message>`
Send a finding to your lead agent (used by explorer/researcher agents).

```bash
loom finding "Found that auth module uses singleton pattern" --ref LOOM-001 --class foundational
```

Flags:
- `--ref` — related issue ID
- `--class` — finding classification: foundational | tactical | observational

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
loom log orchestrator              # one agent's log
loom log --all                     # interleaved stream from all agents
```

### `loom log daemon`
Tail the daemon log.

```bash
loom log daemon
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
loom config set models.builder opus
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
```

### `loom doctor`
Diagnose and fix stale processes, locks, and sockets.

```bash
loom doctor
loom doctor --dry-run              # show what would be cleaned
```

### `loom update`
Update loom to the latest version. Downloads pre-built binaries from GitHub Releases.

```bash
loom update
```

### `loom mcp-server`
Start the built-in MCP server (stdio JSON-RPC transport). Used by agents, not typically run manually.

```bash
loom mcp-server --agent-id builder-017
loom mcp-server --agent-id builder-017 --loom-root /path/to/.loom
```
