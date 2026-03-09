# Mail System

## Overview

Async message passing between agents. File-based (YAML files on disk), no sockets, no database. Messages are sent at critical points only — not conversational back-and-forth.

## Design Principles

1. **Async, not real-time** — agents check mail when notified, not continuously
2. **Critical points only** — agents send mail at defined trigger points (see below)
3. **Inspectable** — all messages are human-readable YAML files
4. **Auditable** — processed messages move to archive, never deleted
5. **Simple delivery** — write file to inbox → daemon notifies via tmux send-keys

## Directory Structure

```
.loom/mail/
├── inbox/
│   ├── orchestrator/
│   │   ├── 1741560846-lead-auth-completion.yaml
│   │   └── 1741560900-lead-api-status.yaml
│   ├── lead-auth/
│   │   └── 1741560870-builder-017-blocker.yaml
│   └── builder-017/
│       └── 1741560850-lead-auth-task.yaml
└── archive/
    └── 2026-03-09/
        ├── 1741560800-orchestrator-lead-auth-task.yaml
        └── ...
```

## Message Schema

```yaml
# .loom/mail/inbox/builder-017/1741560850-lead-auth-task.yaml
id: "1741560850-lead-auth-task"
from: lead-auth
to: builder-017
type: task                       # see Message Types below
priority: normal                 # critical | normal | low
timestamp: 2026-03-09T18:34:10-04:00
ref: LOOM-001-01                 # related issue (optional)
subject: "Implement login form validation"
body: |
  Implement the login form with email/password validation.
  
  Requirements:
  - Email format validation
  - Password minimum 8 chars
  - Show inline errors
  
  Working in worktree: loom-LOOM-001-01-login-form
  
  When done, run: loom mail send lead-auth "LOOM-001-01 complete" --type completion
read: false                      # set to true when agent reads it
```

## Message Types

| Type | Direction | When |
|---|---|---|
| `task` | lead → worker | Assigning work |
| `status` | any → parent | Progress update (optional, for long tasks) |
| `completion` | worker → lead, lead → orchestrator | Task/feature done |
| `blocker` | worker → lead | Can't proceed, needs help |
| `review-request` | lead → reviewer | Code ready for review |
| `review-result` | reviewer → lead | Review findings (pass/fail) |
| `question` | any → any | Need clarification |
| `escalation` | any → orchestrator | Unresolvable issue, skip hierarchy |
| `nudge` | human → any, orchestrator → any | Guidance or course correction |

## Send Triggers (When Agents MUST Send Mail)

Agents are instructed via their prompt templates to send mail at these points:

### Builder
- Task started (status to lead)
- Blocker hit (blocker to lead)
- Task complete (completion to lead)

### Reviewer
- Review complete (review-result to lead)

### Explorer / Researcher
- Findings ready (completion to requester, with artifact reference)

### Lead
- Plan ready (status to orchestrator)
- Sub-issue assigned (task to worker)
- Worker blocked (may escalate to orchestrator)
- Feature complete (completion to orchestrator)

### Orchestrator
- Issue picked up (status update on issue)
- Lead spawned (internal tracking)

## Delivery Mechanism

1. Sender runs: `loom mail send <to> "<subject>" --type <type> --body "<body>"`
2. Loom CLI writes YAML file to `.loom/mail/inbox/<to>/<timestamp>-<from>-<slug>.yaml`
3. Daemon's mail watcher goroutine detects new file (polling every 2s)
4. Daemon sends tmux notification:
   ```bash
   tmux send-keys -t <agent-tmux-target> \
     "[LOOM] New mail from <from>: <subject>. Run: loom mail read" Enter
   ```
5. Agent reads mail, processes it, marks as read

## Reading Mail

When an agent runs `loom mail read`:
1. Lists all files in `.loom/mail/inbox/<agent-id>/`
2. Displays unread messages (where `read: false`)
3. Marks displayed messages as `read: true`

## Archiving

When an agent is done with a message (or during `loom gc`):
- Message moves from `inbox/<agent>/` to `archive/<date>/`
- Preserves full audit trail

## Priority Handling

- `critical` messages: daemon sends notification immediately + adds `[URGENT]` prefix
- `normal` messages: standard notification
- `low` messages: notification batched (daemon waits until next poll cycle)

## MCP Alternative

When MCP is enabled, agents can use native tools instead of CLI:
- `loom_mail_send(to, subject, body, type, priority, ref)`
- `loom_mail_read(unread_only)`
- `loom_mail_count()`

This avoids the overhead of shelling out to the CLI for every mail operation.
