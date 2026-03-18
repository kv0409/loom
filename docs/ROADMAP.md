# Roadmap

## v0.1.0 (Current)

Agent communication via ACP (Agent Control Protocol). Agents are daemon-managed subprocesses spawned via the pending-acp status pattern. The daemon's `watchPendingAgents()` picks up pending-acp agents and creates ACP client subprocesses. Communication uses structured prompts/responses. Output is written to .output files. Human interaction via `loom nudge`.

## v0.2.0 (Planned)

### Other v0.2.0 items
- Dashboard interactive actions (nudge, kill from TUI)
- Session resume across restarts
- Agent timeout and auto-restart
- Improved merge conflict handling
