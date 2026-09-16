## Beads Issue Tracking

This project uses Beads (`bd`) for issue/task tracking. Use the `bd` CLI directly — do not use MCP.

### Workflow
- At session start, context is automatically loaded via `bd prime` (SessionStart hook). This also refreshes after context compaction.
- Before starting work: run `bd ready --json` to check what work is ready, or use `/resolve-ticket` for a guided flow.
- Before claiming a task: `bd update bd-<id> --claim --json`.
- When a task is done: `bd close bd-<id> --reason "<short description of the fix>" --json`.
- If you discover a new bug or task while working, create it and link it to the current one:
  `bd create "<title>" --description="<description>" --deps discovered-from:bd-<current> --json`
- After every git commit, a hook automatically prompts the `ticket-writer` subagent to review the changes and file any issues it finds as Beads tickets, tagged `auto-generated` for human review.

### Rules
- Always use the `--json` flag on every `bd` command for structured, parseable output.
- Always include a `--description` when creating an issue — an issue without context is not useful for future work.
- ALWAYS run `bd dolt push` at the end of the session to sync changes (a Stop hook also reminds you).
- Tickets created automatically by `ticket-writer` carry the `auto-generated` label — review and re-label/promote them before treating them as confirmed backlog items.

### Useful commands
- `bd list --status open --json` — list open issues
- `bd show bd-<id> --json` — show issue details
- `bd blocked --json` — list blocked issues
- `bd doctor` — run a health check if something isn't working

### Agents & commands in this repo
- `/resolve-ticket` — lists ready tickets by priority and walks through claiming, resolving, and closing one.
- `ticket-writer` subagent — read-only code auditor that files Beads tickets for bugs, security issues, and tech debt. Runs automatically after commits (see `.claude/hooks/post-commit-ticket-writer.sh`) or can be invoked directly: "Use the ticket-writer agent to audit the auth module."