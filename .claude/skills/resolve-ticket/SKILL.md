---
description: List open Beads tickets by priority and let me choose which one to resolve
---

You are helping resolve tickets tracked in Beads (`bd`).

Steps:
1. Run `bd ready --json` to get all tickets that are ready to work on.
2. Sort them by priority (highest priority first — p1 before p2, etc.), and group by type if useful (bug, feature, task).
3. Present the list to me in a clear, numbered format, showing: ID, title, priority, type, labels, and effort (if the description states "Effort: S/M/L"). Mark tickets labelled `auto-generated` as unverified — they come from the ticket-writer agent and may be false positives. Do not start working yet.
4. Use the AskUserQuestion tool to ask me which ticket I want to resolve (offer the top few as options, plus an option to type a different ID or "none for now").
5. Once I pick one:
   - Run `bd show bd-<id> --json` to get full details and context.
   - Run `bd update bd-<id> --claim --json` to claim it.
   - Restate the ticket's description and acceptance criteria back to me before making any code changes.
   - For `auto-generated` tickets, first verify the claim against the current code. If it is a false positive or already fixed, tell me and propose closing it instead of making changes.
   - Implement the fix/feature, following the codebase's existing conventions. Read the exact lines you are about to edit first, so edits match the file's whitespace.
   - If you discover a new related issue while working, first run `bd search "<keywords>" --json` to check for an existing or overlapping ticket. If none exists, create it and link it:
     `bd create "<title>" --description="<description>" --deps discovered-from:bd-<id> --json`
     If one overlaps, reference it instead of creating a duplicate.
   - When done, summarize the change and ask me to confirm before running:
     `bd close bd-<id> --reason "<short description of the fix>" --json`
6. Try to write unit or integration tests if necessary. If changes are in database layer, only in that case write integration tests.
   - If a test is not feasible (e.g. the code depends on a concrete type that cannot be mocked), do not silently skip it: say why, and offer to file a follow-up ticket for the missing testability (linked with `discovered-from:bd-<id>`).
7. After I confirm the ticket is closed, offer to wrap up. Do not do any of this without my approval:
   - Commit the code changes (only the files touched for this ticket) in one commit.
   - Commit the `.beads/*.jsonl` export separately as `chore: sync beads issue export`.
   - Run `git push`.
   - Leave unrelated uncommitted or untracked files alone and mention them.
8. At the very end of the session, run (or remind me to run) `bd dolt push` to sync.

Never close a ticket without my explicit confirmation that the work is done and correct.
