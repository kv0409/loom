# AGENTS.md

## Commands

```bash
make build    # REQUIRED — injects version/commit via ldflags. Plain `go build` produces a versionless binary.
make vet      # go vet ./... — run after every change
```

## Conventions

- All CLI commands live in `cmd/loom/main.go` (monolithic by design). Don't create separate command files — add new commands there.
- Function parameters use options structs (`SpawnOpts`, `ListOpts`, `CreateOpts`). Don't add bare parameter lists.
- Errors are wrapped with `fmt.Errorf("context: %w", err)`. No sentinel errors, no custom error types.
- Process kills use negative PID to kill the entire process group: `syscall.Kill(-pid, SIGTERM)`. Positive PID leaves children orphaned.
- Daemon self-daemonizes by re-exec with `LOOM_DAEMON=1` env var — not fork. Don't change this pattern.
- `agent.NextID("orchestrator")` always returns `"orchestrator"` (not `orchestrator-001`). Only one orchestrator exists.
- Worktree/branch names MUST match `<ISSUE-ID>-<slug>` (e.g., `LOOM-042-fix-spawn`). `parseNameConvention()` uses regex `^(LOOM-\d+(?:-\d+)?)` to extract the issue ID — non-matching names silently break association.
- Lock file paths encode `/` as `__`: path `/a/b` → lock file `__a__b.lock.yaml`.

## Gotchas

- `go:embed` assets (`agents/*.json`, `templates/*.md`, `agents/hooks/*`) are copied to `.loom/` and `.kiro/` during `loom init`. Changing embedded files requires `loom init --force` in existing projects.
- `store.NextCounter()` is read-increment-write without locking — concurrent calls can produce duplicate IDs.
- File locks are advisory only. Nothing prevents agents from writing without acquiring a lock. Two agents can clobber the same YAML file.
- Corrupted YAML files silently vanish from `List()` calls (error → `continue`). Daemon watchers also swallow all errors.
- The daemon uses a Unix socket (`.loom/daemon.sock`) for its API despite the "no sockets" design principle — this is the one exception.
- Config changes require daemon restart (`loom restart`).
- Deregistering an agent triggers immediate inbox GC — all its unread mail is deleted.
- `store.WriteYAML()` uses temp-file-then-rename for crash safety, but there's no protection against concurrent writes to the same file.

## Boundaries

- ⚠️ **Ask first:**
  - Adding dependencies to `go.mod`
  - Changing daemon watcher count or polling intervals
  - Modifying embedded assets in `agents/` or `templates/`
  - Changing YAML schemas (agent, issue, mail, memory files)
  - Altering MCP tool definitions in `internal/mcp/`

- 🚫 **Never:**
  - Commit `.loom/` or `.kiro/` (runtime state, gitignored)
  - Introduce databases or HTTP servers — file-based IPC is a core design choice (Unix socket for daemon API is the sole exception)
  - Use `panic` or `os.Exit` outside of `main()`
  - Edit `go.sum` manually — use `go mod tidy`
