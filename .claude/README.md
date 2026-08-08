# .claude Directory

This directory contains documentation and configuration specific to Claude Code and AI-assisted development.

## Structure

```
.claude/
├── README.md          # This file
└── docs/              # Domain documentation for AI context
    ├── arhitecture.md       # High-level architecture overview
    ├── identity_context.md  # Identity & authentication domain
    ├── account_profile.md   # User profiles and settings domain
    └── activity.md          # Activity groups and sessions domain (CORE)
```

## Purpose

### docs/
Contains detailed domain-driven design documentation for each bounded context. These files help AI agents (like Claude Code) understand:

- **Business rules and invariants** - What's allowed and what's not
- **Domain models** - Aggregates, entities, value objects
- **Database schemas** - Table structures and relationships
- **Domain events** - Event-driven communication between contexts
- **API contracts** - Request/response formats
- **Event subscriptions** - How contexts react to events from other contexts

### When to Update These Files

Update the domain documentation when:
- Adding new aggregates or domain models
- Changing business rules or validation logic
- Adding new domain events
- Modifying database schema
- Adding new API endpoints
- Changing event subscription patterns

### Best Practices

1. **Keep in sync with code** - Documentation should reflect actual implementation
2. **Focus on "why" not just "what"** - Explain business rules and design decisions
3. **Document cross-context interactions** - Event flows between bounded contexts
4. **Include examples** - Especially for complex flows (waitlists, recurring sessions, etc.)
5. **Reference actual files** - Point to migration files, domain code locations

## Usage by Claude Code

When working with Claude Code:
1. Start by reading `CLAUDE.md` in the project root for general context
2. Refer to specific domain docs in `.claude/docs/` when working on that domain
3. Claude Code will use these files to understand business logic and architecture

## Related Files

- `/CLAUDE.md` - Root-level guidance for Claude Code (general project info)
- `/migrations/` - Database migrations (source of truth for schema)
- `/internal/{domain}/domain/` - Actual domain model implementations
