# Roadmap

## v0.1.0 (Current)

Agent communication via tmux send-keys. Agents self-poll for mail after each action. Daemon sends [LOOM] notifications as a wake-up signal for idle agents.

Known limitations:
- Notifications can arrive while agent is busy (queued in tmux buffer, usually works)
- No programmatic response capture from agents
- Human interaction via `loom attach` (tmux) and `loom nudge`

## v0.2.0 (Planned)

### ACP Integration

Replace tmux send-keys with Agent Client Protocol (ACP) for agent communication.

kiro-cli supports ACP — a JSON-RPC 2.0 protocol over stdin/stdout that provides:
- `session/new` — create agent sessions programmatically
- `session/prompt` — send messages to agents (proper queuing, no race conditions)
- `session/notification` — receive agent responses, tool calls, turn completion
- `session/cancel` — cancel in-progress work

Architecture change:
- Loom becomes an ACP client
- Each agent spawned as `kiro-cli acp --agent loom-<role>`
- Loom manages JSON-RPC connections to each agent via stdin/stdout pipes
- Messages delivered via `session/prompt` instead of tmux send-keys
- Agent responses captured via `session/notification` for logging and state tracking
- tmux panes kept for human observation (output mirrored)

Benefits:
- Clean message delivery with proper queuing
- Response capture enables: auto-detect idle agents, parse tool calls, log everything
- No tmux buffer race conditions
- Programmatic session management (pause, resume, cancel)

### Other v0.2.0 items
- Dashboard interactive actions (nudge, kill from TUI)
- Session resume across restarts
- Agent timeout and auto-restart
- Improved merge conflict handling
