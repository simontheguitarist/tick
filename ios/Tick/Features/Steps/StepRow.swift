import SwiftUI

/// One step, in the TUI's grammar: a fixed leading slot holds the priority
/// number (dim mono), a pink `!` for important, or a green check once done;
/// important text goes amber and semibold, done text strikes through and
/// dims. The celebration overlay rides on top.
struct StepRow: View {
    let step: Step
    let number: Int?
    let celebrating: Bool

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    private var struck: Bool { step.done }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Space.snug) {
            leadingSlot
                .frame(width: 34, alignment: .trailing)
            Text(step.text)
                .font(.body)
                .fontWeight(step.important && !struck ? .semibold : .regular)
                .foregroundStyle(textColor)
                .strikethrough(struck, color: Theme.inkDim)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, Theme.Space.snug)
        .frame(minHeight: 44)
        .scaleEffect(celebrating && !reduceMotion ? 0.985 : 1)
        .overlay(alignment: .leading) {
            if celebrating && !reduceMotion {
                ConfettiBurst()
                    .frame(width: 140, height: 60)
                    .offset(x: 60)
                    .allowsHitTesting(false)
            }
        }
        .animation(.snappy(duration: Theme.Motion.strike), value: struck)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(accessibilityText)
    }

    private var leadingSlot: some View {
        ZStack {
            if struck {
                Text("✓")
                    .font(.stepNumber)
                    .foregroundStyle(Theme.green)
                    .transition(.scale.combined(with: .opacity))
            } else {
                HStack(spacing: 2) {
                    if step.important {
                        Text("!")
                            .font(.mono(.body, .bold))
                            .foregroundStyle(Theme.pink)
                    }
                    Text(number.map { "\($0)." } ?? "")
                        .font(.stepNumber)
                        .foregroundStyle(step.important ? Theme.amber : Theme.inkDim)
                }
                .transition(.scale.combined(with: .opacity))
            }
        }
    }

    private var textColor: Color {
        if struck { return Theme.inkDim }
        if step.important { return Theme.amber }
        return Theme.ink
    }

    private var accessibilityText: String {
        var parts: [String] = []
        if let n = number { parts.append("Step \(n)") }
        parts.append(step.text)
        if step.important { parts.append("important") }
        if step.done { parts.append("done") }
        return parts.joined(separator: ", ")
    }
}
