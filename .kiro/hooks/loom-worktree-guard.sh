#!/bin/bash
# loom-worktree-guard.sh
# preToolUse hook: blocks writes outside LOOM_WORKTREE

EVENT=$(cat)
TOOL=$(echo "$EVENT" | jq -r '.tool_name // ""')

# Only guard write operations
if [ "$TOOL" != "fs_write" ] && [ "$TOOL" != "write" ]; then
  exit 0
fi

PATH_ARG=$(echo "$EVENT" | jq -r '.tool_input.path // .tool_input.file_path // ""')

if [ -z "$LOOM_WORKTREE" ] || [ -z "$PATH_ARG" ]; then
  exit 0
fi

# Resolve to absolute path
ABS_PATH=$(cd "$(dirname "$PATH_ARG")" 2>/dev/null && pwd)/$(basename "$PATH_ARG")
ABS_WORKTREE=$(cd "$LOOM_WORKTREE" 2>/dev/null && pwd)

case "$ABS_PATH" in
  "$ABS_WORKTREE"/*)
    exit 0
    ;;
  *)
    echo "Blocked: cannot write to $PATH_ARG — outside your worktree ($LOOM_WORKTREE). You may only modify files inside your assigned worktree." >&2
    exit 2
    ;;
esac
