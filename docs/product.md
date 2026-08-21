# tick

Jot down the next open steps per project, cross them off with a small
celebration, and see the same list on the Mac (CLI/TUI) and the iPhone.

## Users & jobs

One user: Simon. The job: when sitting down to work — or thinking about work
away from the desk — see *one* project's next steps without the mental
context-switch tax of every other project's open list. The phone exists so an
idea captured on the couch lands in the right project's queue on the Mac, and
a step crossed off at the desk disappears from the pocket.

## Scope

- **CLI/TUI** (`tick`): capture and prioritize inside the repo you're in.
  Order is the priority — a list you sort by hand, not a checklist.
- **iPhone app** (`ios/`): projects → one project's steps; add, cross off,
  reorder, mark important; widget for the most-active project.
- **Sync server** (`tickd`): one binary, one JSON document, one bearer token,
  self-hosted at https://tick.dply.ch. `/` redirects to this repo (App Store
  page later).
- Offline-first everywhere; devices converge via per-entity last-writer-wins
  (see docs/protocol.md).

Out of scope: multi-user, accounts, push notifications, real-time channels,
iPad/macOS app targets, recurring tasks, due dates.

## Key flows

1. `tick` in a repo → that project's steps; `a` to add, `x` to cross off
   (confetti), `⇧↑/↓` to reorder. Everything syncs automatically.
2. Phone: open Tick → it restores the last project → the next steps, numbered.
   Tap to cross off (haptic + confetti), swipe for important/delete, drag to
   reorder, add from the bottom bar.
3. Pairing: `tick sync setup <url>` once on the Mac, then `tick sync qr` and
   scan with the app.
4. Offline: edit anywhere; the outbox drains on reconnect; deletions carry
   tombstones so nothing resurrects.

## Business context

Personal tool, public repo. No revenue intent; App Store release is for
convenience (own devices) and as a portfolio artifact.

## Pointers

- docs/design.md — design system (TUI + iOS)
- docs/protocol.md — the sync contract both clients implement
- docs/decisions/ — ADRs
- deploy/ — Dockerfile + compose for self-hosting
