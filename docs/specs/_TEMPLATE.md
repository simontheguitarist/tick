---
status: draft            # draft | ready | in-progress | shipped | abandoned
created: YYYY-MM-DD
branch: <git branch>
---

# <Title>

## Context

[2–3 sentences: what exists today, why it's insufficient, why now — which
user/customer pain pulls this.]

## Current State

[Verified, not assumed — file paths with line numbers.]

## Scope

[The smallest version that delivers the value. What changes, concretely — files,
schemas, API shapes. Actual SQL / actual interfaces, not descriptions: zero design
decisions left for the implementer.]

## Out of Scope

- [Near-miss that seems related but is NOT part of this spec]
- [Deferred cut line, with a one-line why]

## Acceptance Criteria

Numbered. Pass/fail. Concrete input → observable output. No adjectives ("fast",
"clean", "works correctly") — numbers and behaviors; missing a number → "Unknown —
measure by <method>".

1. Given <concrete input>, <system> does <concrete observable output>
2. Given <edge case: empty / huge / duplicate / wrong tenant / called twice>, <output>
3. Tests written and passing; no degradation of existing functionality

## Files Reference

| File | Change |
|------|--------|
| `path/to/file:line` | What changes here |

## Rollback

[How to undo. "Revert the PR" is acceptable — but state it.]
