import SwiftUI
import UIKit

/// The design system in one file. The app is the phone-sized sibling of the
/// terminal UI: the TUI's exact palette (internal/tui/model.go), monospaced
/// type for everything structural, a dark ground in every appearance. Colour
/// has jobs — purple titles, cyan interaction, yellow counts, green done,
/// pink/amber importance — and the confetti hues are for celebration only.
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

    // Ground
    static let bg = Color(hex: 0x0D0F14)
    static let surface = Color(hex: 0x161922)
    static let surfaceRaised = Color(hex: 0x1D2130)
    static let rule = Color(hex: 0x44475A)

    // Ink
    static let ink = Color(hex: 0xF2F3F7)
    static let inkSecondary = Color(hex: 0x9AA0B4)
    static let inkDim = Color(hex: 0x5E6478)

    // The TUI hues, verbatim
    static let purple = Color(hex: 0xA864FD) // titles, group headers
    static let cyan = Color(hex: 0x29CDFF)   // cursor, interaction, project names
    static let green = Color(hex: 0x78FF44)  // done
    static let yellow = Color(hex: 0xFDFF6A) // counts
    static let pink = Color(hex: 0xFF718D)   // the important `!`, destructive
    static let amber = Color(hex: 0xFFB86C)  // important text

    /// Confetti (internal/tui/confetti.go) — celebration only.
    static let confetti: [Color] = [
        Color(hex: 0xA864FD), Color(hex: 0x29CDFF), Color(hex: 0x78FF44),
        Color(hex: 0xFF718D), Color(hex: 0xFDFF6A), Color(hex: 0xFFA94D),
    ]

    /// The TUI banner, row by row, with its gradient.
    static let bannerArt: [String] = [
        "████████╗██╗  ██████╗██╗  ██╗",
        "╚══██╔══╝██║ ██╔════╝██║ ██╔╝",
        "   ██║   ██║ ██║     █████╔╝ ",
        "   ██║   ██║ ██║     ██╔═██╗ ",
        "   ██║   ██║ ╚██████╗██║  ██╗",
        "   ╚═╝   ╚═╝  ╚═════╝╚═╝  ╚═╝",
    ]
    static let bannerPalette: [Color] = [
        Color(hex: 0xA864FD), Color(hex: 0x8F7BFF), Color(hex: 0x29CDFF),
        Color(hex: 0x78FF44), Color(hex: 0xFDFF6A), Color(hex: 0xFF718D),
    ]
}

extension Color {
    init(hex: UInt32) {
        self.init(red: Double((hex >> 16) & 0xFF) / 255,
                  green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255)
    }
}

extension Font {
    static func mono(_ style: Font.TextStyle, _ weight: Font.Weight = .regular) -> Font {
        .system(style, design: .monospaced).weight(weight)
    }
    static let stepNumber = mono(.body, .medium)
    static let countLabel = mono(.caption)
    static let statusLine = mono(.footnote)
    static let projectName = mono(.body, .semibold)
    static let screenTitle = mono(.title, .bold)
    static let groupHeader = mono(.caption, .bold)
}

/// The ASCII banner from the terminal, as the app's masthead.
struct BannerView: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(Theme.bannerArt.enumerated()), id: \.offset) { i, line in
                Text(line)
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundStyle(Theme.bannerPalette[i])
            }
        }
        .lineSpacing(0)
        .accessibilityLabel("tick")
    }
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
