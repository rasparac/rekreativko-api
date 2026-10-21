# .claude

Claude Code configuration and project knowledge for AI-assisted work.

```
CLAUDE.md (repo root)    always loaded: overview, layout, commands, rules that always apply
<service>/CLAUDE.md      loaded when working in that service: points to its README
.claude/
├── rules/               loaded automatically; files with `paths:` frontmatter only for matching files
│   ├── go-style.md        **/*.go
│   ├── testing.md         **/*_test.go
│   ├── database.md        persistence, migrations
│   ├── events.md          event code
│   ├── gateway.md         gateway/**
│   ├── observability.md   always
│   └── git-and-beads.md   always
├── docs/                read on demand: older domain design notes (service READMEs and code win)
├── skills/              /resolve-ticket
├── agents/              ticket-writer, architecture-reviewer
├── hooks/               post-commit hook that runs ticket-writer
└── settings.json
```

## Where new knowledge goes

- Applies to every task (a hard rule, a command): root `CLAUDE.md` - keep it short.
- A convention for one kind of code: the matching file in `rules/`, or a new one with a `paths:` glob.
- How one service behaves: that service's `README.md` (for humans too).
- A one-off fact about a decision: the beads ticket that made it, or `bd remember`.

Check every statement against the code before adding it; stale guidance is worse than none.
