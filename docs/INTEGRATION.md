# Integration

## Overview

How Loom integrates with kiro-cli, including MCP server, hooks, agent prompt injection, and extension points.

## Agent Communication: Two Modes

### Mode 1: CLI Shelling (Default)

Agents call `loom` commands via `execute_bash` in their kiro session:

```bash
# Agent sends mail
loom mail send lead-auth "LOOM-001-01 complete" --type completion

# Agent updates issue
loom issue update LOOM-001-01 --status done

# Agent records decision
loom memory add decision "Use bcrypt for password hashing" --rationale "Industry standard, configurable cost factor"

# Agent acquires file lock
loom lock acquire src/auth/login.ts

# Agent sends heartbeat
loom agent heartbeat
```

Pros: Works with any kiro-cli version, no special setup.
Cons: Each call spawns a new process, parses YAML, writes files. Slight overhead.

### Mode 2: MCP Server (Recommended)

Loom includes a built-in MCP server that agents connect to via kiro-cli's MCP configuration. This gives agents native tool access.

#### Starting the MCP Server

The MCP server starts automatically with `loom start` (if `mcp.enabled: true` in config). It runs as a subprocess of the loom daemon.

```bash
# Or manually:
loom mcp-server --port 9876
```

#### MCP Tools Exposed

| Tool | Description |
|---|---|
| `loom_mail_send` | Send mail to another agent |
| `loom_mail_read` | Read inbox (unread messages) |
| `loom_mail_count` | Count unread messages |
| `loom_issue_show` | Get issue details |
| `loom_issue_update` | Update issue status/fields |
| `loom_issue_create` | Create a sub-issue |
| `loom_issue_list` | List issues with filters |
| `loom_memory_add` | Record a decision/discovery/convention |
| `loom_memory_search` | Search memory entries |
| `loom_memory_list` | List memory entries |
| `loom_lock_acquire` | Acquire a file lock |
| `loom_lock_release` | Release a file lock |
| `loom_lock_check` | Check if a file is locked |
| `loom_agent_heartbeat` | Update heartbeat timestamp |
| `loom_agent_status` | Report current status |
| `loom_worktree_info` | Get info about own worktree |

#### Agent MCP Configuration

When spawning an agent, loom configures kiro-cli to connect to the MCP server. This is done by:

1. Writing a temporary kiro config that includes the MCP server connection
2. Starting kiro-cli with that config

The exact mechanism depends on how kiro-cli supports MCP configuration:

**Option A: Config file**
```json
// .loom/agents/builder-017.kiro.json
{
  "mcpServers": {
    "loom": {
      "command": "loom",
      "args": ["mcp-server", "--agent-id", "builder-017"],
      "env": {
        "LOOM_ROOT": "/path/to/project/.loom"
      }
    }
  }
}
```

**Option B: CLI flags** (if kiro-cli supports it)
```bash
kiro-cli chat --mcp-server "loom:loom mcp-server --agent-id builder-017"
```

**Option C: Stdio MCP** (most likely for kiro-cli)
The MCP server runs as a stdio process that kiro-cli spawns. Each agent gets its own MCP server instance that knows the agent's identity.

#### MCP Tool Schemas

```json
{
  "name": "loom_mail_send",
  "description": "Send a mail message to another agent",
  "inputSchema": {
    "type": "object",
    "properties": {
      "to": { "type": "string", "description": "Recipient agent ID" },
      "subject": { "type": "string", "description": "Message subject" },
      "body": { "type": "string", "description": "Message body" },
      "type": {
        "type": "string",
        "enum": ["task", "status", "completion", "blocker", "review-request", "review-result", "question", "escalation"],
        "default": "status"
      },
      "priority": {
        "type": "string",
        "enum": ["critical", "normal", "low"],
        "default": "normal"
      },
      "ref": { "type": "string", "description": "Related issue ID" }
    },
    "required": ["to", "subject"]
  }
}
```

```json
{
  "name": "loom_memory_search",
  "description": "Search the shared memory store for decisions, discoveries, and conventions",
  "inputSchema": {
    "type": "object",
    "properties": {
      "query": { "type": "string", "description": "Search query" },
      "type": {
        "type": "string",
        "enum": ["decision", "discovery", "convention"],
        "description": "Filter by memory type"
      },
      "limit": { "type": "integer", "default": 5 }
    },
    "required": ["query"]
  }
}
```

## Kiro-CLI Hooks

If kiro-cli supports lifecycle hooks, loom can use them for:

### Pre-session Hook
Run before kiro-cli starts processing. Could be used to:
- Inject loom context (current issues, recent decisions) into the session
- Set up environment variables (LOOM_AGENT_ID, LOOM_ROOT)

### Post-message Hook
Run after each agent message. Could be used to:
- Auto-extract decisions from agent output and record in memory
- Detect when an agent mentions being blocked and auto-send blocker mail
- Update heartbeat automatically

### Session-end Hook
Run when kiro-cli session ends. Could be used to:
- Auto-cleanup agent registration
- Archive mail
- Release file locks

## Agent Prompt Injection

The most critical integration point. When loom spawns an agent, it sends an initial prompt that teaches the agent how to operate within the loom system.

### Prompt Template Variables

Templates in `.loom/templates/` use Go template syntax:

```
{{.AgentID}}          — e.g., "builder-017"
{{.Role}}             — e.g., "builder"
{{.ParentAgent}}      — e.g., "lead-auth"
{{.AssignedIssues}}   — list of issue IDs
{{.IssueDetails}}     — full issue YAML for assigned issues
{{.WorktreePath}}     — absolute path to worktree
{{.WorktreeBranch}}   — branch name
{{.RelevantMemory}}   — recent decisions/conventions that affect assigned issues
{{.RelevantPlan}}     — blueprint/task breakdown from lead
{{.FileScope}}        — comma-separated file-scope hints (string, if provided via --scope)
{{.MCPEnabled}}       — whether MCP tools are available
{{.ProjectRoot}}      — absolute path to project root
{{.LoomRoot}}         — absolute path to .loom/
```

### Example: Builder Prompt Template

```markdown
# You are {{.AgentID}}

You are a builder agent in the Loom orchestration system. You implement one task at a time in an isolated git worktree.

## Your Assignment
{{range .AssignedIssues}}
Issue: {{.ID}} — {{.Title}}
Priority: {{.Priority}}
Description: {{.Description}}
{{end}}

## Your Worktree
- Path: {{.WorktreePath}}
- Branch: {{.WorktreeBranch}}
- All your work MUST happen in this directory. Do NOT modify files outside it.

{{if .FileScope}}
## File Scope Hints
Your lead assigned these file-scope hints — focus your edits here, but you may touch other files if genuinely needed:
{{.FileScope}}
{{end}}

## Relevant Context
{{if .RelevantMemory}}
### Decisions & Conventions
{{range .RelevantMemory}}
- [{{.ID}}] {{.Title}}: {{.Decision}}
{{end}}
{{end}}

{{if .RelevantPlan}}
### Plan
{{.RelevantPlan}}
{{end}}

## Communication
{{if .MCPEnabled}}
Use the loom MCP tools for all coordination:
- `loom_mail_send` — send messages to other agents
- `loom_issue_update` — update your issue status
- `loom_memory_add` — record important decisions
- `loom_lock_acquire` — lock files before editing
- `loom_agent_heartbeat` — send every few minutes
{{else}}
Use the `loom` CLI for all coordination:
- `loom mail send <to> "<subject>" --type <type>` — send messages
- `loom issue update <id> --status <status>` — update issue status
- `loom memory add decision "<title>" --rationale "<why>"` — record decisions
- `loom lock acquire <file>` — lock files before editing
- `loom agent heartbeat` — send every few minutes
{{end}}

## Rules
1. Work ONLY in your worktree: {{.WorktreePath}}
2. Commit frequently with descriptive messages
3. Send heartbeat every few minutes
4. When you start: update issue to `in-progress`
5. When blocked: send blocker mail to {{.ParentAgent}} immediately
6. When done: commit all changes, update issue to `done`, send completion mail to {{.ParentAgent}}
7. Record any architectural decisions in memory
8. Check memory before making decisions: `loom memory search "<topic>"`
9. Acquire locks before editing files: `loom lock acquire <path>`
10. When you see [LOOM] messages, process them immediately

## When You See [LOOM] Messages
These are notifications from the loom system injected into your session.
- "[LOOM] New mail" → Run `loom mail read` and process the message
- "[LOOM] Nudge: ..." → Read the guidance and adjust your approach
- "[LOOM] Shutdown" → Commit current work, update issue status, stop
```

## Environment Variables

Set for each agent's kiro-cli process:

```bash
LOOM_AGENT_ID=builder-017
LOOM_ROLE=builder
LOOM_ROOT=/path/to/project/.loom
LOOM_PROJECT_ROOT=/path/to/project
LOOM_PARENT_AGENT=lead-auth
LOOM_WORKTREE=/path/to/project/.loom/worktrees/loom-LOOM-001-01-login-form
LOOM_FILE_SCOPE=src/auth/login.ts,src/auth/types.ts  # if --scope provided
LOOM_MCP_PORT=9876          # if MCP enabled
```

## Extension Points

### Custom Agent Roles
Users can add custom templates to `.loom/templates/` for specialized roles (e.g., `tester.md`, `documenter.md`). The lead agent can reference these when spawning workers.

### Custom Issue Types
The issue type field is a free string. Users can define project-specific types beyond the defaults (epic, task, bug, spike).

### Hooks Directory
Future: `.loom/hooks/` for user-defined scripts that run on events:
- `on-issue-create.sh`
- `on-agent-spawn.sh`
- `on-merge.sh`
- `on-blocker.sh`

### Plugin MCP Servers
Agents can connect to additional MCP servers beyond loom's built-in one. Configured per-role in `.loom/config.yaml`:

```yaml
roles:
  researcher:
    extra_mcp_servers:
      - name: web-search
        command: web-search-mcp-server
```
