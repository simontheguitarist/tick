# 0001 — Per-entity last-writer-wins sync over JSON document stores

## Decision

Two-way sync between CLI and iPhone converges by per-entity last-writer-wins
on millisecond UTC timestamps (monotonic 1ms bump per device), with UUID
identity, per-step fractional ranks for ordering, and permanent tombstones on
a self-hosted single-binary server. Every store — CLI, server, phone — is one
JSON document written atomically; the server is plain `net/http` with a
mutex, no database.

## Why

One user, two-ish devices, tiny data (~16 KB). LWW is the simplest rule two
codebases (Go + Swift) can provably implement identically — the merge is a
pure function tested against a shared case table, and ranks make reordering
(the product's core gesture) a single-entity change instead of a list-wide
conflict. JSON documents reuse the CLI's existing atomic-rename pattern,
keep every store greppable, and mean cursor + data + outbox commit in one
rename — crash-safe by construction.

## Alternatives considered

- **Operation log / CRDT**: strictly more correct under concurrency, and
  strictly more machinery on three sides. The conflict LWW gets "wrong"
  (same entity edited on both devices in the same instant) is vanishingly
  rare for one person and resolves deterministically.
- **SQLite everywhere**: adds a dependency and a schema-migration story to
  each side, buys query power nobody needs at this size.
- **iOS SwiftData** (house convention): would split state across a DB, a
  cursor store, and an outbox — three things to keep consistent across
  crashes where the JSON document needs zero. The widget also reads the
  same file directly.

## Consequences

Wall-clock dependence (mitigated, not eliminated: ms precision + monotonic
bump + server echo of rejected pushes). Whole-record conflict granularity —
a text edit and a reorder of the *same step* race whole-record. Server
storage rewrites the full document per accepted batch — fine at kilobytes,
revisit past ~1 MB. If a second Mac joins with its own pre-sync v1 store, it
must start empty or duplicate projects appear (documented limitation).
