# Build Plan

## Implementation Order

Build incrementally. Each phase produces a usable (if incomplete) tool.

### Phase 1: Skeleton + Init
- [ ] Go module setup (`go mod init github.com/karanagi/loom`)
- [ ] Cobra CLI skeleton with all subcommands (stubs)
- [ ] `loom init` — create `.loom/` directory structure, default config, copy templates
- [ ] `loom config show` / `loom config set`
- [ ] YAML store helpers (generic read/write/list for `.loom/` files)

**Milestone**: `loom init` works, creates `.loom/` in a git repo.

### Phase 2: Issue Tracker
- [ ] Issue CRUD (`create`, `list`, `show`, `update`, `close`)
- [ ] Issue ID generation (counter-based)
- [ ] Sub-issue support (parent/children)
- [ ] Dependency tracking (`depends_on`)
- [ ] State machine validation (only valid transitions)
- [ ] `--tree` view for `loom issue list`

**Milestone**: Human can create/manage issues from CLI.

### Phase 3: Mail System
- [ ] Mail send (write YAML to inbox)
- [ ] Mail read (list/display inbox, mark as read)
- [ ] Mail archive
- [ ] Mail log (history view)
- [ ] Filename convention: `{timestamp}-{from}-{slug}.yaml`

**Milestone**: `loom mail send` / `loom mail read` work standalone.

### Phase 4: Memory System
- [ ] Memory add (decision/discovery/convention)
- [ ] Memory list (with type filter)
- [ ] Memory show (detail view)
- [ ] Memory search (BM25-style keyword scoring)
- [ ] ID generation per type (DEC-xxx, DISC-xxx, CONV-xxx)

**Milestone**: `loom memory add` / `loom memory search` work standalone.

### Phase 5: Worktree Management
- [ ] Worktree create (wraps `git worktree add`)
- [ ] Worktree remove (wraps `git worktree remove` + branch cleanup)
- [ ] Worktree list (with diff stats from `git diff --stat`)
- [ ] Worktree show (detail view)
- [ ] Worktree cleanup (remove orphaned)
- [ ] File lock acquire/release/check

**Milestone**: `loom worktree` commands manage git worktrees in `.loom/worktrees/`.

### Phase 6: ACP Integration
- [ ] ACP client subprocess management (spawn, kill, communicate)
- [ ] Structured prompt/response protocol
- [ ] Output capture to .output files
- [ ] Process group management (kill -PID for cleanup)

**Milestone**: Loom can create ACP subprocesses and send structured prompts.

### Phase 7: Agent Lifecycle
- [ ] Agent spawn (create YAML with pending-acp status, daemon picks up and creates ACP subprocess, inject prompt)
- [ ] Agent registry (list, show, status)
- [ ] Agent kill (cleanup)
- [ ] Heartbeat update + monitoring
- [ ] Prompt template rendering (Go templates with agent variables)
- [ ] `loom nudge` (ACP prompt to agent)

**Milestone**: Can spawn a kiro-cli agent as ACP subprocess with a rendered prompt.

### Phase 8: Daemon
- [ ] Issue watcher (poll `.loom/issues/` for new files)
- [ ] Mail notifier (poll inboxes, send ACP notifications)
- [ ] Heartbeat monitor (detect dead agents)
- [ ] Pending-acp agent watcher (`watchPendingAgents()`)
- [ ] PID lock file (`loom.lock`)
- [ ] Graceful shutdown
- [ ] Crash recovery (detect orphaned state on start)

**Milestone**: `loom start` launches daemon that watches for issues and delivers mail.

### Phase 9: Orchestrator
- [ ] Orchestrator agent prompt template
- [ ] Auto-spawn orchestrator on `loom start`
- [ ] Issue auto-pickup (daemon notifies orchestrator of new issues)
- [ ] Lead spawning logic (orchestrator creates leads for epics)

**Milestone**: `loom start` → create issue → orchestrator picks it up → spawns lead.

### Phase 10: Dashboard (TUI)
- [ ] Main overview (agents, issues, worktrees, mail, memory summary)
- [ ] Agent list + detail view
- [ ] Issue list + detail + tree view
- [ ] Mail viewer
- [ ] Memory browser + search
- [ ] Log viewer (tail agent logs)
- [ ] Interactive actions (nudge, attach, kill from dashboard)
- [ ] Keyboard navigation

**Milestone**: `loom dash` shows full TUI with all views.

### Phase 11: MCP Server
- [ ] MCP server binary mode (`loom mcp-server`)
- [ ] Implement all MCP tools (mail, issues, memory, locks, heartbeat)
- [ ] Agent spawn with MCP config
- [ ] Stdio transport for kiro-cli integration

**Milestone**: Agents can use native MCP tools instead of CLI shelling.

### Phase 12: Polish
- [ ] `loom export` (summary for PRs/standups)
- [ ] `loom gc` (garbage collection)
- [ ] `loom status` (quick health check)
- [ ] Session resume (`loom start --resume`)
- [ ] Error messages and edge case handling
- [ ] README and usage docs
- [ ] Makefile (build, install, test)
- [ ] Release binaries (goreleaser or manual)

---

## Dependencies (Final)

```
require (
    github.com/spf13/cobra v1.8.0
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/lipgloss v0.9.0
    github.com/charmbracelet/bubbles v0.18.0
    gopkg.in/yaml.v3 v3.0.1
)
```

5 direct dependencies. All well-maintained, widely used, minimal transitive deps.

---

## Estimated Effort

| Phase | Complexity | Approx LOC |
|---|---|---|
| 1. Skeleton | Low | ~300 |
| 2. Issues | Medium | ~500 |
| 3. Mail | Medium | ~400 |
| 4. Memory | Medium | ~400 |
| 5. Worktrees | Medium | ~350 |
| 6. Tmux | Low | ~200 |
| 7. Agents | High | ~600 |
| 8. Daemon | High | ~500 |
| 9. Orchestrator | Medium | ~300 |
| 10. Dashboard | High | ~1200 |
| 11. MCP Server | High | ~800 |
| 12. Polish | Medium | ~400 |
| **Total** | | **~6000** |

This is a substantial but well-scoped project. Each phase is independently testable.
