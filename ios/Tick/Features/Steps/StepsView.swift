import SwiftUI

/// One project's next steps — the screen the app exists for. Numbered open
/// steps in priority order, nothing from any other project in sight.
struct StepsView: View {
    @Environment(AppModel.self) private var app
    let projectID: String

    @State private var showDone = false
    @State private var celebrating: Set<String> = []
    @State private var editingStep: Step?
    @State private var editText = ""
    @State private var lastDeleted: Step?
    @State private var undoDismiss: Task<Void, Never>?
    @State private var editMode: EditMode = .inactive

    private var project: Project? { app.store.doc.project(projectID) }

    /// Rows on screen: open steps, done steps when shown, plus freshly
    /// crossed-off ones still celebrating (persist first, animate after).
    private var visibleSteps: [Step] {
        app.store.doc.steps(of: projectID, includeDone: true).filter {
            !$0.done || showDone || celebrating.contains($0.id)
        }
    }

    var body: some View {
        List {
            let steps = visibleSteps
            ForEach(Array(steps.enumerated()), id: \.element.id) { index, step in
                StepRow(
                    step: step,
                    number: openNumber(step, in: steps),
                    celebrating: celebrating.contains(step.id)
                )
                .contentShape(Rectangle())
                .onTapGesture { toggle(step) }
                .listRowSeparator(.hidden)
                .swipeActions(edge: .leading, allowsFullSwipe: true) {
                    Button {
                        Haptics.important()
                        app.store.toggleImportant(step.id)
                    } label: {
                        Label("Important", systemImage: "exclamationmark")
                    }
                    .tint(.accentColor)
                    if index > 0 {
                        Button {
                            Haptics.reorder()
                            app.store.moveToTop(step.id, in: projectID)
                        } label: {
                            Label("Top", systemImage: "arrow.up.to.line")
                        }
                        .tint(.secondary)
                    }
                }
                .swipeActions(edge: .trailing) {
                    Button(role: .destructive) { delete(step) } label: {
                        Label("Delete", systemImage: "trash")
                    }
                    Button {
                        editText = step.text
                        editingStep = step
                    } label: {
                        Label("Edit", systemImage: "pencil")
                    }
                    .tint(.secondary)
                }
            }
            .onMove { source, destination in
                Haptics.reorder()
                app.store.move(in: projectID, visible: visibleSteps, from: source, to: destination)
            }

            if visibleSteps.isEmpty {
                ContentUnavailableView {
                    Label("All clear", systemImage: "checkmark.circle")
                } description: {
                    Text("Nothing open here. Add the next step below.")
                }
                .listRowSeparator(.hidden)
            }
        }
        .listStyle(.plain)
        .environment(\.editMode, $editMode)
        .navigationTitle(project?.name ?? "")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Toggle(isOn: $showDone) {
                        Label("Show done", systemImage: "eye")
                    }
                    Button {
                        withAnimation { editMode = editMode == .active ? .inactive : .active }
                    } label: {
                        Label(editMode == .active ? "Done reordering" : "Reorder", systemImage: "arrow.up.arrow.down")
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
            }
            ToolbarItem(placement: .topBarTrailing) {
                let open = app.store.doc.openCount(of: projectID)
                Text(open == 0 ? "—" : "\(open) open")
                    .font(.countLabel)
                    .foregroundStyle(.secondary)
            }
        }
        .refreshable { await app.engine.syncNow() }
        .safeAreaInset(edge: .bottom) {
            VStack(spacing: Theme.Space.tight) {
                if let deleted = lastDeleted {
                    UndoToast(text: deleted.text) {
                        app.store.undoDelete(deleted)
                        lastDeleted = nil
                        undoDismiss?.cancel()
                    }
                }
                AddStepBar { app.store.addStep($0, to: projectID) }
            }
        }
        .alert("Edit step", isPresented: Binding(
            get: { editingStep != nil },
            set: { if !$0 { editingStep = nil } }
        )) {
            TextField("Step", text: $editText)
            Button("Save") {
                if let s = editingStep {
                    app.store.editText(s.id, editText)
                }
                editingStep = nil
            }
            Button("Cancel", role: .cancel) { editingStep = nil }
        }
        .onAppear {
            showDone = app.settings.showDoneByDefault
            app.settings.lastOpenedProjectID = projectID
        }
    }

    /// 1-based position among open steps (done rows carry no number).
    private func openNumber(_ step: Step, in steps: [Step]) -> Int? {
        if step.done { return nil }
        var n = 0
        for s in steps where !s.done {
            n += 1
            if s.id == step.id { return n }
        }
        return nil
    }

    private func toggle(_ step: Step) {
        guard editMode == .inactive else { return }
        if step.done && !celebrating.contains(step.id) {
            Haptics.reopen()
            app.store.toggleDone(step.id)
            return
        }
        guard !step.done else { return }
        // Persist first, celebrate after — the TUI's animate-then-remove.
        Haptics.crossOff()
        app.store.toggleDone(step.id)
        _ = withAnimation(.snappy(duration: Theme.Motion.quick)) {
            celebrating.insert(step.id)
        }
        Task {
            try? await Task.sleep(for: .seconds(Theme.Motion.hold + Theme.Motion.burst * 0.4))
            _ = withAnimation(.snappy) { celebrating.remove(step.id) }
        }
    }

    private func delete(_ step: Step) {
        Haptics.delete()
        app.store.deleteStep(step.id)
        lastDeleted = step
        undoDismiss?.cancel()
        undoDismiss = Task {
            try? await Task.sleep(for: .seconds(5))
            guard !Task.isCancelled else { return }
            withAnimation { lastDeleted = nil }
        }
    }
}

struct UndoToast: View {
    let text: String
    let undo: () -> Void

    var body: some View {
        HStack {
            Text("Deleted \u{201C}\(text.prefix(28))\(text.count > 28 ? "…" : "")\u{201D}")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            Spacer()
            Button("Undo", action: undo)
                .font(.footnote.weight(.semibold))
        }
        .padding(.horizontal, Theme.Space.base)
        .padding(.vertical, Theme.Space.snug)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, Theme.Space.base)
        .transition(.move(edge: .bottom).combined(with: .opacity))
    }
}
