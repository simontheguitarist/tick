# Review checklist

## Correctness
- [ ] Every mutation goes through `store.Update` (CLI/TUI) or `Store.persist`
      (iOS) — no direct writes.
- [ ] Every mutation stamps `updated` (monotonic bump) and `dirty`.
- [ ] Deletions leave tombstones; nothing splices without one.
- [ ] Rank invariant: steps sorted by (rank, id); only the moved step re-keys.

## Data & migrations
- [ ] store.json stays readable by external consumers: `projects` map,
      steps keep `text/done/done_at/created` with local-offset timestamps.
- [ ] Schema bumps: migrate on load, persist immediately, keep a one-time
      backup, refuse newer-versioned files.
- [ ] Wire/format changes update docs/protocol.md + BOTH merge
      implementations + both test tables in the same PR.

## Security
- [ ] Token only in sync.json (0600) / iOS Keychain — never in store.json,
      argv, logs, or URLs (the QR/deep link is the sanctioned exception).
- [ ] Server: constant-time token compare, body/field limits, no default
      token, fails to start without TICK_TOKEN.

## UI
- [ ] docs/design.md QA checklist (both appearances, Reduce Motion,
      VoiceOver, ≥44pt targets).
- [ ] Sync failures stay quiet (status line), never block input.
