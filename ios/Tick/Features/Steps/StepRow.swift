import SwiftUI

/// One step: a fixed 28pt leading slot (number ↔ checkmark), the text, and the
/// celebration overlay. Important steps speak with weight and the accent, not
/// with badges.
struct StepRow: View {
    let step: Step
    let number: Int?
    let celebrating: Bool

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    private var struck: Bool { step.done }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Space.snug) {
            leadingSlot
                .frame(width: 28, alignment: .trailing)
            Text(step.text)
                .font(.body)
                .fontWeight(step.important && !struck ? .semibold : .regular)
                .foregroundStyle(textColor)
                .strikethrough(struck, color: .secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, Theme.Space.tight)
        .frame(minHeight: 44)
        .scaleEffect(celebrating && !reduceMotion ? 0.985 : 1)
        .overlay(alignment: .leading) {
            if celebrating && !reduceMotion {
                ConfettiBurst()
                    .frame(width: 120, height: 60)
                    .offset(x: 56)
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
                Image(systemName: "checkmark")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(.tint)
                    .transition(.scale.combined(with: .opacity))
            } else {
                Text(number.map { "\($0)." } ?? "")
                    .font(.stepNumber)
                    .foregroundStyle(step.important ? AnyShapeStyle(.tint) : AnyShapeStyle(.secondary))
                    .transition(.scale.combined(with: .opacity))
            }
        }
    }

    private var textColor: Color {
        if struck { return .secondary }
        if step.important { return .accentColor }
        return .primary
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
