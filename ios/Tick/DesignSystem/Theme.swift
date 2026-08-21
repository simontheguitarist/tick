import SwiftUI
import UIKit

/// The design system in one file. Colour lives in exactly two places: the
/// amber accent (interactive + important — the TUI's `!` colour) and the
/// confetti palette (celebration only, the TUI's six hues, verbatim). Mono is
/// the signature — step numbers, counts, hints — everything else is plain SF.
enum Theme {
    enum Space {
        static let hair: CGFloat = 4
        static let tight: CGFloat = 8
        static let snug: CGFloat = 12
        static let base: CGFloat = 16
        static let roomy: CGFloat = 24
    }

    enum Motion {
        static let quick: Double = 0.18  // number -> check morph
        static let strike: Double = 0.28 // strike-through draw
        static let hold: Double = 0.60   // struck row lingers before it leaves
        static let burst: Double = 0.70  // confetti lifetime
    }

    /// The TUI confetti hues (internal/tui/confetti.go), used nowhere else.
    static let confetti: [Color] = [
        Color(red: 0.66, green: 0.39, blue: 0.99), // #a864fd
        Color(red: 0.16, green: 0.80, blue: 1.00), // #29cdff
        Color(red: 0.47, green: 1.00, blue: 0.27), // #78ff44
        Color(red: 1.00, green: 0.44, blue: 0.55), // #ff718d
        Color(red: 0.99, green: 1.00, blue: 0.42), // #fdff6a
        Color(red: 1.00, green: 0.66, blue: 0.30), // #ffa94d
    ]
}

extension Font {
    /// The step number / count voice — the CLI nod.
    static let stepNumber = Font.system(.body, design: .monospaced).weight(.medium)
    static let countLabel = Font.system(.caption, design: .monospaced)
    static let statusLine = Font.system(.footnote, design: .monospaced)
}

/// Haptics map: success for the win, light touches for everything reversible,
/// silence for sync (background noise stays background).
@MainActor
enum Haptics {
    static func crossOff() { UINotificationFeedbackGenerator().notificationOccurred(.success) }
    static func reopen() { UIImpactFeedbackGenerator(style: .light).impactOccurred() }
    static func important() { UIImpactFeedbackGenerator(style: .medium).impactOccurred() }
    static func reorder() { UIImpactFeedbackGenerator(style: .rigid).impactOccurred() }
    static func delete() { UINotificationFeedbackGenerator().notificationOccurred(.warning) }
    static func paired() { UINotificationFeedbackGenerator().notificationOccurred(.success) }
}
