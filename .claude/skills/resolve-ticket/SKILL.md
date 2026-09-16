---
description: List open Beads tickets by priority and let me choose which one to resolve
---

You are helping resolve tickets tracked in Beads (`bd`).

Steps:
1. Run `bd ready --json` to get all tickets that are ready to work on.
2. Sort them by priority (highest priority first — p1 before p2, etc.), and group by type if useful (bug, feature, task).
3. Present the list to me in a clear, numbered format, showing: ID, title, priority, and type. Do not start working yet.
4. Use the AskUserQuestion tool to ask me which ticket I want to resolve (offer the top few as options, plus an option to type a different ID or "none for now").
5. Once I pick one:
   - Run `bd show bd-<id> --json` to get full details and context.
   - Run `bd update bd-<id> --claim --json` to claim it.
   - Restate the ticket's description and acceptance criteria back to me before making any code changes.
   - Implement the fix/feature, following the codebase's existing conventions.
   - If you discover a new related issue while working, create it and link it:
     `bd create "<title>" --description="<description>" --deps discovered-from:bd-<id> --json`
   - When done, summarize the change and ask me to confirm before running:
     `bd close bd-<id> --reason "<short description of the fix>" --json`
6. Try to write unit or integration tests if necessary. If changes are in database layer, only in that case write integration tests.
7. At the very end of the session, remind me to run `bd dolt push` to sync.

Never close a ticket without my explicit confirmation that the work is done and correct.