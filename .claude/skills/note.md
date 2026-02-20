---
name: note
description: Add an observation, antipattern, or design note to the appropriate ANTIPATTERNS.md file
user_invocable: true
---

# /note — Add a development note

The user wants to record an observation about the codebase. This could be an antipattern, a design concern, a TODO, or any development note.

## Instructions

1. Parse the user's note. Determine:
   - Which package/directory it relates to (e.g., `pkg/wal/`, `pkg/lifecycle/`, `cmd/nile/`)
   - Whether it's a new antipattern, a resolution of an existing one, or a general observation
   - A severity level if it's an antipattern: High, Medium, or Low

2. Read the relevant `ANTIPATTERNS.md` file in that package directory.

3. If it's a **new antipattern**: Add it under the `## Active` section with a title, severity, description, and suggested fix.

4. If it **resolves an existing antipattern**: Move the entry from `## Active` to `## Resolved` with a brief note on what was done.

5. If the package doesn't have an `ANTIPATTERNS.md` yet, create one following this format:

```markdown
# Antipatterns: <package name>

## Active

### <Title> [Severity: High/Medium/Low]
<Description>

**Fix**: <Suggested approach>

## Resolved

*(none yet)*
```

6. Confirm what was added and where.

If the user's note is ambiguous about which package it belongs to, ask them. If it's a cross-cutting concern, add it to the most relevant package and cross-reference from others.
