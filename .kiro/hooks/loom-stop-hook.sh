#!/bin/bash
# loom-stop-hook.sh
# agentStop hook: sends heartbeat and reports unread mail count

loom agent heartbeat 2>/dev/null

MAIL_OUT=$(loom mail read "$LOOM_AGENT_ID" --unread 2>/dev/null)
if [ "$MAIL_OUT" != "No messages" ] && [ -n "$MAIL_OUT" ]; then
  COUNT=$(echo "$MAIL_OUT" | grep -c "^---")
  echo "[LOOM] $COUNT unread mail message(s). Run: loom mail read"
fi
