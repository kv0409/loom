#!/bin/bash
# loom-stop-hook.sh
# Runs at end of every agent turn.
# Nudges parent agent that this agent completed a turn.
# (Heartbeat is handled by preToolUse hook on every shell call.)

# Notify parent if we have one
if [ -n "$LOOM_PARENT_AGENT" ]; then
  loom nudge "$LOOM_PARENT_AGENT" "[LOOM] Agent $LOOM_AGENT_ID completed a turn. Check on it: loom mail read" 2>/dev/null
fi
