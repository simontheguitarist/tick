# Tick Design System

Two faces, one identity: a terminal tool and a native iOS app that clearly
belong together without the app cosplaying as a terminal.

## Aesthetic direction

Calm list, loud celebration. Ninety-nine percent of the surface is quiet —
system backgrounds, plain type, generous space. All personality is
concentrated in two moments: the amber accent on interactive/important
things, and the confetti burst when a step is crossed off.

## Typography

- iOS: SF only. No serif, no rounded. Large title for the project name.
- Monospaced is the signature voice — step numbers, counts, sync status,
  server hostnames — always via `Font.stepNumber` / `.countLabel` /
  `.statusLine` (Theme.swift). If it smells like the terminal, it's mono.
- TUI: the ANSI banner + Lip Gloss styles in internal/tui/model.go.

## Color

- Accent "amber phosphor": `#B45309` light / `#FFB86C` dark (the TUI's
  important-`!` colour). Used for: interactive controls, important steps,
  checkmarks. Nothing else.
- Confetti palette (celebration ONLY, never UI chrome):
  `#a864fd #29cdff #78ff44 #ff718d #fdff6a #ffa94d` — verbatim from
  internal/tui/confetti.go, mirrored in Theme.confetti.
- Everything else: system semantic colors, both appearances first-class.

## Spacing & layout

Theme.Space: 4/8/12/16/24. Step rows: 28pt leading number slot, ≥44pt touch
target. One project per screen — never show another project's steps.

## Components

- **Step row**: number ↔ checkmark morph in a fixed slot; strike-through +
  text fades to secondary on done; important = semibold + accent, no badges.
- **Add bar**: capsule on material, pinned to the bottom, keeps the keyboard
  up (capture comes in bursts).
- **Status line**: one mono footnote ("Synced 2 min ago" / "Offline — saved
  on this phone"). Sync issues are never alerts.
- **Cross-off**: persist first, then celebrate — haptic ✓, strike draw
  (0.28s), 0.7s confetti, row leaves after 0.6s. Reduce Motion: strike +
  check only.

## Voice & microcopy

Lowercase-calm, concrete, no exclamation marks outside the `!` glyph. "Add a
step", "All clear", "Offline — saved on this phone". The CLI speaks the same
way ("all done — nice.").

## Design QA checklist

- [ ] Light + dark screenshots: Projects, Steps, Pairing, widget S/M
- [ ] Cross-off: haptic → strike → burst → row gone in ≤1s; Reduce Motion path
- [ ] Accent only on interactive/important; confetti hues nowhere in chrome
- [ ] Numbers/counts/status in mono; touch targets ≥44pt
- [ ] VoiceOver reads "Step n, …, important/done"
