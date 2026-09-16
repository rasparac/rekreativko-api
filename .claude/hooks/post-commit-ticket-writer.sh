bash
#!/bin/bash
# Fires after every Bash tool call. If the command was a successful
# `git commit`, tells Claude to delegate a review to ticket-writer.

INPUT=$(cat)

COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
SUCCESS=$(echo "$INPUT" | jq -r '.tool_output.success // empty')

# Only trigger on git commit commands
if ! echo "$COMMAND" | grep -qE '(^|&&|\|\||;)\s*git commit\b'; then
  exit 0
fi

# Only trigger if the commit actually succeeded
if [ "$SUCCESS" != "true" ]; then
  exit 0
fi

cat <<'EOF'
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "A git commit was just made. Use the ticket-writer subagent to review the changes in this commit (git show HEAD) and file any bugs, security issues, or tech debt found as Beads tickets. Do this before responding further to the user."
  }
}
EOF

exit 0