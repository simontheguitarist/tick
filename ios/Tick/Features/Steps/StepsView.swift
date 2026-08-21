import SwiftUI

/// One project's next steps — the screen the app exists for. Numbered open
/// steps in priority order, nothing from any other project in sight.
/// Tap crosses off, long-press opens the menu, press-and-drag reorders.
struct StepsView: View {
    @Environment(AppModel.self) private var app
    let projectID: String

    @State private var showDone = false
    @State private var celebrating: Set<String> = []
    @State private var editingStep: Step?
    @State private var editText = ""
    @State private var lastDeleted: Step?
    @State private var undoDismiss: Task<Void, Never>?

    private var project: Project? { app.store.doc.project(projectID) }

    /// Rows on screen with their open-number, computed once per render: open
    /// steps, done steps when shown, plus freshly crossed-off ones still
    /// celebrating (persist first, animate after).
    private var rows: [(step: Step, number: Int?)] {
        var open = 0
        return app.store.doc.steps(of: projectID, includeDone: true).compactMap { step in
            guard !step.done || showDone || celebrating.contains(step.id) else { return nil }
            if step.done { return (step, nil) }
            open += 1
            return (step, open)
        }
    }

    var body: some View {
        let rows = rows
        List {
            header
                .listRowInsets(EdgeInsets(top: 0, leading: Theme.Space.base, bottom: Theme.Space.snug, trailing: Theme.Space.base))
                .listRowSeparator(.hidden)
                .listRowBackground(Theme.bg)

            ForEach(rows, id: \.step.id) { row in
                StepRow(step: row.step, number: row.number, celebrating: celebrating.contains(row.step.id))
                    .contentShape(Rectangle())
                    .onTapGesture { toggle(row.step) }
                    .listRowBackground(Theme.bg)
                    .listRowSeparatorTint(Theme.rule.opacity(0.35))
                    .listRowInsets(EdgeInsets(top: 0, leading: Theme.Space.base, bottom: 0, trailing: Theme.Space.base))
                    .contextMenu { menu(for: row.step, isFirst: rows.first?.step.id == row.step.id) }
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        Button {
                            Haptics.important()
                            app.store.toggleImportant(row.step.id)
                        } label: {
                            Label(row.step.important ? "Normal" : "Important", systemImage: "exclamationmark")
                        }
                        .tint(Theme.pink)
                    }
                    .swipeActions(edge: .trailing) {
                        Button(role: .destructive) { delete(row.step) } label: {
                            Label("Delete", systemImage: "trash")
                        }
                        Button {
                            editText = row.step.text
                            editingStep = row.step
                        } label: {
                            Label("Edit", systemImage: "pencil")
                        }
                        .tint(Theme.inkDim)
                    }
            }
            .onMove { source, destination in
                Haptics.reorder()
                app.store.move(in: projectID, visible: rows.map(\.step), from: source, to: destination)
            }

            if rows.isEmpty {
                HStack(spacing: Theme.Space.tight) {
                    Text("✓").foregroundStyle(Theme.green)
                    Text("all clear — add the next step below")
                        .foregroundStyle(Theme.inkDim)
                }
                .font(.statusLine)
                .listRowBackground(Theme.bg)
                .listRowSeparator(.hidden)
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .scrollDismissesKeyboard(.interactively)
        .background(Theme.bg)
        .navigationBarTitleDisplayMode(.inline)
        .toolbarBackground(.hidden, for: .navigationBar)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    withAnimation(.snappy) { showDone.toggle() }
                } label: {
                    Image(systemName: showDone ? "eye" : "eye.slash")
                }
                .accessibilityLabel(showDone ? "Hide done steps" : "Show done steps")
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
            .background(Theme.bg.opacity(0.92))
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

    /// Project name and its numbers, in the banner's voice: "› name" in
    /// cyan like the TUI header, the open count in count-yellow underneath.
    private var header: some View {
        VStack(alignment: .leading, spacing: Theme.Space.hair) {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Space.tight) {
                Text("›")
                    .font(.screenTitle)
                    .foregroundStyle(Theme.inkDim)
                Text(project?.name ?? "")
                    .font(.screenTitle)
                    .foregroundStyle(Theme.cyan)
                    .lineLimit(2)
                    .minimumScaleFactor(0.7)
            }
            HStack(spacing: Theme.Space.snug) {
                let open = app.store.doc.openCount(of: projectID)
                Text(open == 0 ? "nothing open" : "\(open) open")
                    .foregroundStyle(open == 0 ? Theme.inkDim : Theme.yellow)
                if let g = project?.group {
                    Text("· \(g)")
                        .foregroundStyle(Theme.purple)
                }
            }
            .font(.statusLine)
        }
        .padding(.top, Theme.Space.tight)
    }

    @ViewBuilder
    private func menu(for step: Step, isFirst: Bool) -> some View {
        Button {
            editText = step.text
            editingStep = step
        } label: {
            Label("Edit", systemImage: "pencil")
        }
        Button {
            Haptics.important()
            app.store.toggleImportant(step.id)
        } label: {
            Label(step.important ? "Not important" : "Important", systemImage: "exclamationmark")
        }
        if !isFirst && !step.done {
            Button {
                Haptics.reorder()
                app.store.moveToTop(step.id, in: projectID)
            } label: {
                Label("Move to top", systemImage: "arrow.up.to.line")
            }
        }
        if step.done {
            Button {
                Haptics.reopen()
                app.store.toggleDone(step.id)
            } label: {
                Label("Reopen", systemImage: "arrow.uturn.backward")
            }
        }
        Divider()
        Button(role: .destructive) { delete(step) } label: {
            Label("Delete", systemImage: "trash")
        }
    }

    private func toggle(_ step: Step) {
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
        guard let deleted = app.store.deleteStep(step.id) else { return }
        lastDeleted = deleted
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
            Text("deleted \u{201C}\(text.prefix(28))\(text.count > 28 ? "…" : "")\u{201D}")
                .font(.statusLine)
                .foregroundStyle(Theme.inkSecondary)
                .lineLimit(1)
            Spacer()
            Button("undo", action: undo)
                .font(.mono(.footnote, .semibold))
                .foregroundStyle(Theme.cyan)
        }
        .padding(.horizontal, Theme.Space.base)
        .padding(.vertical, Theme.Space.snug)
        .background(Theme.surfaceRaised, in: RoundedRectangle(cornerRadius: 12))
        .padding(.horizontal, Theme.Space.base)
        .transition(.move(edge: .bottom).combined(with: .opacity))
    }
}
