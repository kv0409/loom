#!/bin/bash
# loom-tool-logger.sh
# postToolUse hook: appends one-line tool call summary to .loom/agents/${LOOM_AGENT_ID}.tools

[ -z "$LOOM_AGENT_ID" ] && exit 0

EVENT=$(cat)
TOOL=$(echo "$EVENT" | jq -r '.tool_name // ""')
[ -z "$TOOL" ] && exit 0

FIRST_VAL=$(echo "$EVENT" | jq -r '(.title | select(. != "")) // (.tool_input.summary | select(. != "")) // (.tool_input.__tool_use_purpose | select(. != "")) // (.tool_input | to_entries[0].value) // ""' 2>/dev/null | head -c 120)
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

TOOLS_FILE="${LOOM_ROOT}/agents/${LOOM_AGENT_ID}.tools"
echo "${TIMESTAMP} ${TOOL}: ${FIRST_VAL}" >> "$TOOLS_FILE"

exit 0
