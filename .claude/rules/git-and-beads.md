# Tickets and git

## Beads (`bd`)

Every task is tracked in beads; a SessionStart hook runs `bd prime`, which prints the full command reference.
Never use TodoWrite, TaskCreate or markdown TODO lists, and keep persistent knowledge in `bd remember`, not
MEMORY.md files.

- Pass `--json` to every `bd` command.
- Work on a ticket: `bd show <id> --json`, then `bd update <id> --claim --json`. `/resolve-ticket` walks through it.
- Found something else while working: `bd search "<keywords>" --json` first; if nothing overlaps,
  `bd create "<title>" --description="<why and what>" --deps discovered-from:<current-id> --json`. Never create a
  ticket without a description.
- Close only when the user confirms the work: `bd close <id> --reason "<what was done>" --json`.
- Tickets labelled `auto-generated` come from the `ticket-writer` agent (run by a post-commit hook): unverified
  until someone checks them against the code.

## Git

- Commit and push only when the user asks. Work on `main` unless told otherwise.
- One commit for the ticket's code, then a separate `chore: sync beads issue export` commit for `.beads/*.jsonl`.
  The commit hook re-exports `.beads/issues.jsonl` after committing, so run `git status` after that commit and
  commit again with the same message if it changed.
- At the end of a session: `bd dolt push` (beads sync lives in `refs/dolt/data` on the git remote, separate from
  the code; `.beads/issues.jsonl` is only an export).
