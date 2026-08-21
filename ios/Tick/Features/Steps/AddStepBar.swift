import SwiftUI

/// The bottom prompt for adding steps — the TUI's `add:` line, phone-sized.
/// Keeps the keyboard up for rapid entry; the trailing slot is the way out:
/// a dismiss chevron while the field is empty, the return arrow once you type.
struct AddStepBar: View {
    let add: (String) -> Void
    @State private var text = ""
    @FocusState private var focused: Bool

    var body: some View {
        HStack(spacing: Theme.Space.tight) {
            Text("add:")
                .font(.mono(.body, .semibold))
                .foregroundStyle(Theme.cyan)
            TextField("", text: $text, prompt: Text("next step").foregroundStyle(Theme.inkDim), axis: .vertical)
                .font(.body)
                .foregroundStyle(Theme.ink)
                .lineLimit(1...3)
                .focused($focused)
                .onSubmit(submit)
                .submitLabel(.done)
            if !text.trimmingCharacters(in: .whitespaces).isEmpty {
                Button(action: submit) {
                    Image(systemName: "return")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(Theme.cyan)
                }
                .accessibilityLabel("Add step")
                .transition(.scale.combined(with: .opacity))
            } else if focused {
                Button { focused = false } label: {
                    Image(systemName: "keyboard.chevron.compact.down")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(Theme.inkSecondary)
                }
                .accessibilityLabel("Hide keyboard")
                .transition(.scale.combined(with: .opacity))
            }
        }
        .padding(.horizontal, Theme.Space.base)
        .padding(.vertical, Theme.Space.snug)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(Theme.rule.opacity(0.5), lineWidth: 1))
        .padding(.horizontal, Theme.Space.base)
        .padding(.bottom, Theme.Space.tight)
        .animation(.snappy(duration: Theme.Motion.quick), value: text.isEmpty)
        .animation(.snappy(duration: Theme.Motion.quick), value: focused)
    }

    private func submit() {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        add(t)
        text = ""
        focused = true // keep going — adding steps comes in bursts
    }
}
