# tick

Next-steps-per-project tool: Go CLI/TUI + iOS app + self-hosted sync server
(one repo). Product: docs/product.md. Design: docs/design.md — read it before
any UI work, TUI included. Sync contract: docs/protocol.md — the Go and Swift
merge/rank implementations change only in lockstep with it.

## Stack conventions

- Go 1.26, stdlib-first. Charm (bubbletea/lipgloss) is TUI-only; the server
  (`cmd/tickd`) must stay dependency-free so the Docker build needs no module
  downloads. `rsc.io/qr` and `golang.org/x/term` are CLI-only.
- iOS 26 / Swift 6, XcodeGen: `ios/project.yml` is the source of truth;
  `Tick.xcodeproj` is generated and gitignored — rerun `xcodegen generate`
  after adding ANY file. Shared/ compiles into app + widget: Foundation-only.
- State is JSON documents everywhere (ADR 0001). No SQLite, no SwiftData.
- Timestamps: local-offset in the CLI's store.json (external readers derive
  local dates), canonical UTC-ms on the wire and in the iOS store.

## Migration discipline

store.json schema changes bump `schemaVersion`, migrate on load, persist
immediately, keep a one-time backup, refuse newer versions. The v1 shape
(projects map; text/done/done_at/created keys) is load-bearing for external
readers — see docs/review-checklist.md.

## Deploy

Dokploy app "tick" (project "fun") builds `deploy/Dockerfile` from GitHub on
push; domain tick.dply.ch; env TICK_TOKEN + TICK_LANDING_URL; volume
tick_data:/data. Self-host: `deploy/docker-compose.yml` + `.env`.

## Pre-merge gate (agentic, no CI)

1. `go vet ./... && go test -race ./...` — all green, no skips.
2. `cd ios && xcodegen generate && xcodebuild build` + `xcodebuild test`
   (destination `iPhone 17 Pro`) — zero errors.
3. `docker build -f deploy/Dockerfile .` — image builds.
4. Simulator smoke when app behavior changed: pair → steps → add → cross off
   → kill/relaunch → widget (docs/design.md QA list for UI changes).
5. Protocol-touching changes: both merge tables updated, golden vectors
   regenerated if rank changed (`RANK_GENVEC=… go test ./internal/rank -run
   TestGenerateVectors`), docs/protocol.md in the same commit.

Any functional FAIL → stop and fix before pushing.
