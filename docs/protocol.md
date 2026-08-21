# Sync protocol

The contract between the CLI (`internal/sync`), the server
(`internal/server`), and the iOS app (`ios/Tick/Sync`). Change any side only
in lockstep with the others and with this document.

## Shape

One endpoint. The client pushes its dirty entities and names a cursor; the
server answers with everything newer — including the current version of any
pushed entity it rejected, so a device with a slow clock converges instead of
diverging silently.

```
POST /v1/sync            Authorization: Bearer <TICK_TOKEN>
→ { "device": "<id>", "since": <int64>,
    "projects": [Project…], "steps": [Step…] }
← { "seq": <int64>, "projects": [Project…], "steps": [Step…] }

Project { id, name, path?, group?, deleted?, updated, seq }
Step    { id, project_id, text, done?, important?, rank,
          created, done_at?, deleted?, updated, seq }
```

- `id`: lowercase canonical UUIDv4.
- `updated`: canonical timestamp `YYYY-MM-DDTHH:MM:SS.mmmZ` — fixed-width UTC
  with milliseconds, so string order == time order and the server never
  parses times for conflict resolution. Clients must *send* canonical form;
  the server canonicalizes leniently (any RFC3339 accepted on ingest).
- `seq`: server-assigned, strictly increasing; the pull cursor. `since = 0`
  is a full resync.
- Tombstones are entities with `deleted: true` carrying only
  `id (+ project_id), updated`. The server keeps them forever.
- `path` is Mac-side metadata; other devices store and echo it opaquely.

## Conflict rule (last-writer-wins)

Server, per incoming entity: accept iff unknown, `in.updated > cur.updated`,
or equal-updated and `in.device > cur.device` (deterministic tie). Accepted
entities get `seq = ++counter`. An identical re-push from the same device is
a no-op (idempotent retries).

Client, per received entity, applied under the store lock:

1. **Ack**: a pushed entity whose local `updated` is unchanged since the push
   loses its dirty flag / its tombstone is dropped. An edit that raced the
   push stays dirty for the next round.
2. **Replace**: a newer remote record replaces the local one whole (deletion
   = removal). An *equal* stamp replaces too when the local copy is clean and
   the content differs — that is the server's device-id tie-break echoing the
   winner to the loser (a clean equal-stamp identical echo is a no-op). Older
   → keep local.
3. **Insert guard**: a remote live copy older than a pending local tombstone
   is skipped — local deletions don't resurrect.
4. **Path guard** (CLI only): a project's local path is never overwritten;
   a remote path is adopted only when local has none and the directory
   exists on this machine.
5. Cursor advances to `seq` in the same atomic write as the merged data.

Both merge implementations — internal/sync/merge.go and
ios/Tick/Sync/Merge.swift — are tested against the same case table.

Known tradeoff: LWW on device wall clocks (ms precision, monotonic 1ms bump
per entity per device). Editing the *same step on both devices within the
same millisecond* while offline resolves arbitrarily-but-deterministically.

## Ordering: fractional ranks

A step's position is a string over the base-62 alphabet
`0-9A-Za-z` (ASCII order), compared byte-wise. Moving a step re-keys only
that step: its new rank is the midpoint of its new neighbours' ranks.

Spec (implemented in internal/rank/rank.go and ios/Shared/Models/Rank.swift):

- Ranks are non-empty and never end in `0`.
- `Mid(a,b)`, a<b, "" = open end: strip the common prefix (a right-padded
  with `0`); on the first differing digit take the arithmetic midpoint if the
  gap > 1, else reuse b's head digit when b has more digits, else extend a.
- `After(last)`: increment the final digit; after `z`, append `V`.
- `Before(first)`: decrement the head digit (min `1`), else `Mid("", first)`.
- `Initial(i, n)`: evenly spread keys for bulk assignment (migration,
  rebalance), smallest width with a gap ≥ 2.
- Rebalance: when any rank in a project exceeds 24 chars, re-key the whole
  project with `Initial`.

Parity is pinned by ~400 golden vectors generated from the Go implementation
(`RANK_GENVEC=… go test ./internal/rank -run TestGenerateVectors`) and
asserted by both test suites (`ios/TickTests/Fixtures/rank_vectors.json`).

## Auth & limits

Single shared bearer token (`TICK_TOKEN`), constant-time compare, no
accounts. Body cap 4 MB; text ≤ 4096, name ≤ 256, rank ≤ 128 validated
server-side. `GET /healthz` unauthenticated; `GET /` → 302 to
`TICK_LANDING_URL`.
