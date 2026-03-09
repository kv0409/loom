# Memory System

## Overview

Shared knowledge base where agents record key decisions, discoveries, and conventions. Other agents (and future sessions) can query this to understand *why* things are the way they are. Prevents repeated exploration and conflicting decisions.

## Memory Types

### Decisions
"We chose X because Y." Records architectural and implementation choices with rationale and alternatives considered.

### Discoveries
"We found that X works like Y." Records findings from exploration — how existing code works, gotchas, undocumented behavior.

### Conventions
"In this project, we always do X." Records patterns and standards established during the session.

## Directory Structure

```
.loom/memory/
├── decisions/
│   ├── DEC-001.yaml
│   └── DEC-002.yaml
├── discoveries/
│   ├── DISC-001.yaml
│   └── DISC-002.yaml
└── conventions/
    ├── CONV-001.yaml
    └── CONV-002.yaml
```

## Schemas

### Decision

```yaml
# .loom/memory/decisions/DEC-001.yaml
id: DEC-001
title: "Use JWT for auth tokens instead of sessions"
type: decision
decided_by: lead-auth
timestamp: 2026-03-09T18:36:00-04:00
context: |
  Need stateless auth for the API layer. The app is deployed
  across multiple instances behind a load balancer.
decision: "JWT with RS256 signing, 15min access tokens, 7-day refresh tokens"
rationale: |
  Stateless — no shared session store needed across instances.
  RS256 allows public key verification without sharing secrets.
  Short access tokens limit exposure window.
alternatives:
  - option: "Session-based auth with Redis"
    rejected_because: "Adds Redis as infrastructure dependency"
  - option: "JWT with HS256"
    rejected_because: "Requires sharing secret key across services"
affects:
  - LOOM-001
  - LOOM-001-02
  - LOOM-001-03
tags:
  - auth
  - architecture
```

### Discovery

```yaml
# .loom/memory/discoveries/DISC-001.yaml
id: DISC-001
title: "Auth module uses singleton pattern with lazy initialization"
type: discovery
discovered_by: explorer-001
timestamp: 2026-03-09T18:38:00-04:00
location: "src/auth/provider.ts:15-42"
finding: |
  The AuthProvider class uses a private constructor with a static
  getInstance() method. It lazily initializes on first call.
  All route handlers share the same instance.
implications: |
  New auth middleware must use AuthProvider.getInstance() rather
  than creating a new instance. State is shared across requests.
affects:
  - LOOM-001-02
tags:
  - auth
  - patterns
```

### Convention

```yaml
# .loom/memory/conventions/CONV-001.yaml
id: CONV-001
title: "All API handlers return { data, error } shape"
type: convention
established_by: lead-api
timestamp: 2026-03-09T18:40:00-04:00
rule: |
  Every API endpoint handler must return:
  - Success: { data: <payload>, error: null }
  - Failure: { data: null, error: { code: string, message: string } }
  
  HTTP status codes are set separately. The response body shape is always consistent.
examples:
  - "src/api/users.ts:getUser — returns { data: User, error: null }"
  - "src/api/auth.ts:login — returns { data: { token }, error: null }"
applies_to: "All files in src/api/"
tags:
  - api
  - patterns
```

## Search

Built-in keyword search with BM25-style scoring. No external dependencies.

### How It Works
1. Load all memory YAML files
2. Tokenize search query into terms
3. Score each memory entry by term frequency across: title, context, decision, rationale, finding, rule fields
4. Return ranked results

### CLI

```bash
$ loom memory search "auth middleware"

Results (3 matches):

1. [DEC-001] Use JWT for auth tokens instead of sessions
   Score: 0.87 | By: lead-auth | Affects: LOOM-001, LOOM-001-02

2. [DISC-001] Auth module uses singleton pattern with lazy initialization
   Score: 0.72 | By: explorer-001 | Location: src/auth/provider.ts

3. [CONV-002] Auth middleware must call AuthProvider.getInstance()
   Score: 0.65 | By: lead-auth | Applies to: src/middleware/
```

### MCP Tool

```
loom_memory_search(query: "auth middleware", limit: 5)
loom_memory_add(type: "decision", title: "...", context: "...", rationale: "...")
```

## When Agents Should Record Memory

Agents are instructed via prompt templates:

### Record a Decision when:
- Choosing between two or more approaches
- Selecting a library, pattern, or architecture
- Deciding NOT to do something (and why)

### Record a Discovery when:
- Finding undocumented behavior in existing code
- Discovering how a module/system works
- Finding a gotcha or edge case

### Record a Convention when:
- Establishing a pattern that other agents should follow
- Noticing a consistent pattern in the existing codebase
- Defining a standard for new code

## Persistence Across Sessions

Memory files persist in `.loom/` across `loom stop` / `loom start` cycles. When the orchestrator starts, it can be instructed to review recent memory entries to maintain continuity.

The `loom export` command includes memory entries in its output, making decisions traceable in PR descriptions.

## ID Generation

- Decisions: `DEC-{counter}`
- Discoveries: `DISC-{counter}`
- Conventions: `CONV-{counter}`
- Counters stored in each subdirectory as `counter.txt`
