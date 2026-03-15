# RFC: Research Phase in Agent Workflow

**Issue:** LOOM-121  
**Status:** Proposed  
**Author:** builder-002  
**Date:** 2026-03-15

## Summary

Make research-before-build a first-class part of the Loom workflow by adding a `research_needed` flag to issues. When set, leads spawn a researcher agent before any builders and wait for research completion before proceeding to implementation.

## Problem

Researcher agents exist in Loom but are spawned ad-hoc at a lead's discretion. There is no structured signal that an issue requires research, no visibility in the issue tracker, and no enforcement. This leads to:

- Builders starting work without understanding libraries, patterns, or trade-offs
- Inconsistent use of researchers across different leads
- No audit trail showing that research happened for a given issue

## Options Evaluated

### Option 1: Pre-implement hook spawns researcher before builder

The lead spawns a researcher agent before any builder for flagged issues, using existing conventions.

| Pros | Cons |
|------|------|
| No state machine changes | Invisible in issue tracker |
| Works with existing researcher role | Relies on lead's judgment |
| Lead controls when research is needed | No daemon-level enforcement — easy to skip |

### Option 2: `research_needed` flag on issues (Recommended)

Add a boolean `ResearchNeeded` field to the Issue struct. When a lead sees this flag, it spawns a researcher first and waits for completion before spawning builders.

| Pros | Cons |
|------|------|
| Explicit signal visible in `loom issue show` | Still relies on lead template to enforce |
| Minimal code change | No daemon-level enforcement |
| Humans can set it at creation time | Flag is binary — no "research in progress" state |
| No state machine or dashboard changes | Adds a field most issues won't use |

### Option 3: New `researching` issue status

Add `researching` as a state between `assigned` and `in-progress`: `open → assigned → researching → in-progress → review → done`.

| Pros | Cons |
|------|------|
| First-class visibility in issue tracker | Most issues don't need research — adds noise |
| Daemon can enforce the transition | Requires state machine + dashboard changes |
| History log shows when research happened | More code changes than other options |
| Dashboard kanban shows research in progress | Transient state may confuse kanban view |

## Recommendation: Option 2

Option 2 hits the sweet spot between visibility and simplicity:

- **Explicit signal** on the issue itself, visible in `loom issue show` and `loom issue list`
- **Minimal code change** — one bool field, one CLI flag, one template update
- **No state machine changes** — the `validTransitions` map in `internal/issue/issue.go` stays unchanged
- **No dashboard changes** — no new kanban column or status color needed
- **Humans and leads can both set it** — at issue creation or during decomposition

The lead's existing mail-wait pattern (waiting for builder completion mail) applies identically for researcher completion. No new coordination mechanism is needed.

## Implementation Plan

### Phase 1: Issue struct + CLI (minimal, ~30 lines of Go)

**File: `internal/issue/issue.go`**

Add `ResearchNeeded` field to the `Issue` struct:

```go
type Issue struct {
    // ... existing fields ...
    ResearchNeeded bool `yaml:"research_needed,omitempty"`
}
```

Add `ResearchNeeded` to `CreateOpts`:

```go
type CreateOpts struct {
    // ... existing fields ...
    ResearchNeeded bool
}
```

Wire it through in the `Create()` function where the Issue is constructed from opts.

**File: `cmd/loom/main.go`**

Add `--research-needed` flag to the `issue create` command:

```go
issueCreateCmd.Flags().Bool("research-needed", false, "Flag issue as needing research before implementation")
```

Pass the flag value into `CreateOpts.ResearchNeeded`.

**File: `internal/issue/issue.go` (Show output)**

Display the flag in `Show()` output when set:

```
Research:    yes
```

### Phase 2: Lead template update

**File: `templates/lead.md`**

Add a section to the lead prompt instructing it to check `research_needed`:

```markdown
## Research Phase

Before spawning builders for a task, check if the issue has `research_needed: true`
(visible in `loom issue show` output as `Research: yes`).

If research is needed:
1. Spawn a researcher agent for the issue:
   ```
   loom spawn --role researcher --issues <TASK-ID>
   ```
2. Wait for the researcher's completion mail before spawning builders.
3. After research completes, check memory for findings:
   ```
   loom memory search "<issue topic>"
   ```
4. Use research findings to inform builder task decomposition.

If research is NOT needed, proceed directly to spawning builders as usual.
```

### Phase 3: Researcher template update

**File: `templates/researcher.md`**

Ensure the researcher template instructs the agent to:
1. Record findings in memory using `loom memory add discovery`
2. Send completion mail to parent when done
3. Reference the issue ID in all memory entries for traceability

This is largely already the case — verify and add explicit instructions if missing.

### Phase 4: Documentation

**File: `docs/ISSUES.md`**

Add a section documenting the `research_needed` flag, when to use it, and how it affects the workflow.

**File: `docs/AGENTS.md`**

Update the lead agent section to mention the research phase protocol.

## Files Changed

| File | Change | Size |
|------|--------|------|
| `internal/issue/issue.go` | Add `ResearchNeeded` field to `Issue` and `CreateOpts`, wire through `Create()` and `Show()` | ~15 lines |
| `cmd/loom/main.go` | Add `--research-needed` CLI flag to `issue create` | ~5 lines |
| `templates/lead.md` | Add research phase instructions | ~20 lines |
| `templates/researcher.md` | Verify/add completion protocol | ~5 lines |
| `docs/ISSUES.md` | Document the flag | ~15 lines |
| `docs/AGENTS.md` | Update lead section | ~10 lines |

**Total estimated change: ~70 lines across 6 files.**

## What This Does NOT Change

- **Issue state machine** — `validTransitions` is untouched
- **Dashboard** — no new columns, statuses, or colors
- **Daemon** — no new polling or enforcement logic
- **Mail system** — uses existing completion mail pattern
- **Worktree management** — researchers don't use worktrees (they use memory)

## Future Considerations

If the flag-based approach proves insufficient (e.g., leads consistently ignore it), Option 3 (new `researching` status) can be layered on top. The `ResearchNeeded` field would remain as the trigger, and the new status would add daemon-level enforcement and dashboard visibility. The two approaches are complementary, not mutually exclusive.

Another future enhancement: auto-setting `research_needed` based on issue type (e.g., `spike` issues always need research). This would be a one-line check in the lead template or a default in `CreateOpts` for certain issue types.
