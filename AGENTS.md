# AGENTS.md

## Commands

```bash
make build    # REQUIRED — injects version/commit via ldflags. Plain `go build` produces a versionless binary.
make install  # build + install to $GOBIN — keeps the local binary up to date
make vet      # go vet ./... — run after every change
```

## After completing work

When no active work remains (all issues done, no pending changes):

```bash
make install              # update local binary to latest commit
git push                  # push all commits to remote
```

This keeps the local binary and remote repo in sync. Chats, the orchestrator, and the dashboard all use the installed binary — stale binaries cause confusing behavior.

## Releasing

```bash
./scripts/release.sh <major|minor|patch>            # Bumps version, tags, pushes, waits for goreleaser
./scripts/release.sh --no-wait <major|minor|patch>   # Same but doesn't wait for goreleaser — use when you don't need to block
```

Run periodically after a batch of changes lands on main to cut a new patch release. The script builds and installs locally immediately regardless of `--no-wait`. End users update via `loom update`, which downloads pre-built binaries from GitHub Releases — no git/make/Go required.

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

- `go:embed` assets (`agents/*.json`, `templates/*.md`, `agents/hooks/*`) are copied to `.loom/` and `.kiro/` during `loom init`. Changing embedded files requires `loom init --refresh` in existing projects (updates templates, agents, and hooks without wiping state).
- `store.NextCounter()` is read-increment-write without locking — concurrent calls can produce duplicate IDs.
- File locks are advisory only. Nothing prevents agents from writing without acquiring a lock. Two agents can clobber the same YAML file.
- Corrupted YAML files silently vanish from `List()` calls (error → `continue`). Daemon watchers also swallow all errors.
- The daemon uses a Unix socket (`.loom/daemon.sock`) for its API despite the "no sockets" design principle — this is the one exception.
- Config changes require daemon restart (`loom restart`).
- The daemon owns `logs/daemon.log` — `logrotate.Install` redirects fds 1/2 via `syscall.Dup2` and points `log.SetOutput` at the file, so rotation (close/rename/reopen/dup2) catches stdout, stderr, `log.Printf`, and panic traces. `watchLogGC` rotates when size exceeds `limits.log_max_size_mb` (default 10) and sweeps rotated files past `limits.log_retention_days` (14) or `limits.log_max_rotations` (10) on a `polling.log_gc_interval_ms` (24h) tick.
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
  - Edit `.loom/` YAML files directly — always use the `loom` CLI (`loom issue create`, `loom issue update`, `loom issue close`, `loom mail send`, `loom memory add`, etc.). The CLI handles validation, status transitions, counter increments, and daemon notifications. Direct file edits bypass all of that and cause silent corruption.
