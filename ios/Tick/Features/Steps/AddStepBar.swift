import SwiftUI

/// The bottom capsule for adding steps. Stays out of the way, keeps the
/// keyboard up for rapid entry.
struct AddStepBar: View {
    let add: (String) -> Void
    @State private var text = ""
    @FocusState private var focused: Bool

    var body: some View {
        HStack(spacing: Theme.Space.snug) {
            TextField("Add a step", text: $text, axis: .vertical)
                .lineLimit(1...3)
                .focused($focused)
                .onSubmit(submit)
                .submitLabel(.done)
            if !text.trimmingCharacters(in: .whitespaces).isEmpty {
                Button(action: submit) {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title2)
                }
                .transition(.scale.combined(with: .opacity))
            }
        }
        .padding(.horizontal, Theme.Space.base)
        .padding(.vertical, Theme.Space.snug)
        .background(.regularMaterial, in: Capsule())
        .padding(.horizontal, Theme.Space.base)
        .padding(.bottom, Theme.Space.tight)
        .animation(.snappy(duration: Theme.Motion.quick), value: text.isEmpty)
    }

    private func submit() {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        add(t)
        text = ""
        focused = true // keep going — adding steps comes in bursts
    }
}
