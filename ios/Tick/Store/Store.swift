import Foundation
import WidgetKit

/// The app's single mutable state: an @Observable wrapper around the shared
/// document. Every mutation stamps the entity (monotonic `updated`, `dirty`),
/// persists atomically, refreshes the widget, and pokes the sync engine via
/// `onMutate`. There is no other writer in the app process.
@MainActor
@Observable
final class Store {
    private(set) var doc: StoreDocument
    let file: StoreFile
    var onMutate: (() -> Void)?

    init(file: StoreFile = .appGroup()) {
        self.file = file
        if let existing = file.read() {
            doc = existing
        } else {
            doc = .empty()
            try? file.write(doc)
        }
    }

    // MARK: mutations — projects

    func addProject(name: String, group: String?) -> Project {
        let p = Project.new(name: name, group: group)
        doc.projects.append(p)
        persist()
        return p
    }

    func editProject(_ id: String, name: String, group: String?) {
        guard let i = doc.projects.firstIndex(where: { $0.id == id }) else { return }
        doc.projects[i].name = name
        doc.projects[i].group = Project.normalizedGroup(group)
        stampProject(&doc.projects[i])
        persist()
    }

    /// Untrack: tombstone the project and every step so the deletion syncs.
    func deleteProject(_ id: String) {
        guard let i = doc.projects.firstIndex(where: { $0.id == id }) else { return }
        doc.projects[i].deleted = true
        stampProject(&doc.projects[i])
        for j in doc.steps.indices where doc.steps[j].projectId == id && !doc.steps[j].deleted {
            doc.steps[j].deleted = true
            stampStep(&doc.steps[j])
        }
        persist()
    }

    // MARK: mutations — steps

    func addStep(_ text: String, to projectId: String) {
        let last = doc.steps(of: projectId, includeDone: true).last?.rank ?? ""
        doc.steps.append(Step.new(text: text, projectId: projectId, rank: Rank.after(last)))
        persist()
    }

    func toggleDone(_ stepId: String) {
        mutateStep(stepId) { s in
            s.done.toggle()
            s.doneAt = s.done ? TickTime.now() : nil
        }
    }

    func toggleImportant(_ stepId: String) {
        mutateStep(stepId) { $0.important.toggle() }
    }

    func editText(_ stepId: String, _ text: String) {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else { return }
        mutateStep(stepId) { $0.text = t }
    }

    /// Reorder within the currently visible list (SwiftUI onMove semantics):
    /// only the moved step is re-keyed, so a concurrent edit of any other step
    /// merges cleanly.
    func move(in projectId: String, visible: [Step], from source: IndexSet, to destination: Int) {
        guard let src = source.first, visible.indices.contains(src) else { return }
        var rest = visible
        let moved = rest.remove(at: src)
        let dest = destination > src ? destination - 1 : destination
        let lo = dest > 0 ? rest[dest - 1].rank : ""
        let hi = dest < rest.count ? rest[dest].rank : ""
        mutateStep(moved.id) { $0.rank = Rank.mid(lo, hi) }
        rebalanceIfNeeded(projectId)
    }

    func moveToTop(_ stepId: String, in projectId: String) {
        let all = doc.steps(of: projectId, includeDone: true)
        guard let first = all.first, first.id != stepId else { return }
        mutateStep(stepId) { $0.rank = Rank.before(first.rank) }
        rebalanceIfNeeded(projectId)
    }

    /// Delete with undo: returns the step as it was, for `undoDelete`.
    @discardableResult
    func deleteStep(_ stepId: String) -> Step? {
        guard let i = doc.steps.firstIndex(where: { $0.id == stepId }) else { return nil }
        let before = doc.steps[i]
        doc.steps[i].deleted = true
        stampStep(&doc.steps[i])
        persist()
        return before
    }

    /// Undo re-activates the same id with a fresh stamp — newer than the
    /// tombstone, so it wins everywhere even if the deletion already synced.
    func undoDelete(_ step: Step) {
        if let i = doc.steps.firstIndex(where: { $0.id == step.id }) {
            doc.steps[i] = step
            doc.steps[i].deleted = false
            stampStep(&doc.steps[i])
        } else {
            var s = step
            s.deleted = false
            s.dirty = true
            s.updated = TickTime.bump(after: s.updated)
            doc.steps.append(s)
        }
        persist()
    }

    // MARK: sync integration

    /// Replaces the document with a merge result. Persists and refreshes the
    /// widget but does NOT fire onMutate — a merge must not retrigger sync.
    func applyMerged(_ merged: StoreDocument) {
        doc = merged
        persist(notify: false)
    }

    /// Queue the entire document for push (pairing, full resync).
    func markAllDirtyAndResetCursor() {
        for i in doc.projects.indices { doc.projects[i].dirty = true }
        for i in doc.steps.indices { doc.steps[i].dirty = true }
        doc.since = 0
        persist(notify: false)
    }

    var pendingPushCount: Int {
        doc.projects.count(where: \.dirty) + doc.steps.count(where: \.dirty)
    }

    // MARK: internals

    private func mutateStep(_ id: String, _ change: (inout Step) -> Void) {
        guard let i = doc.steps.firstIndex(where: { $0.id == id }) else { return }
        change(&doc.steps[i])
        stampStep(&doc.steps[i])
        persist()
    }

    private func stampStep(_ s: inout Step) {
        s.updated = TickTime.bump(after: s.updated)
        s.dirty = true
    }

    private func stampProject(_ p: inout Project) {
        p.updated = TickTime.bump(after: p.updated)
        p.dirty = true
    }

    private func rebalanceIfNeeded(_ projectId: String) {
        let all = doc.steps(of: projectId, includeDone: true)
        guard all.contains(where: { $0.rank.count > Rank.maxLength }) else { return }
        for (i, step) in all.enumerated() {
            mutateStepQuiet(step.id) { $0.rank = Rank.initial(i, all.count) }
        }
        persist()
    }

    private func mutateStepQuiet(_ id: String, _ change: (inout Step) -> Void) {
        guard let i = doc.steps.firstIndex(where: { $0.id == id }) else { return }
        change(&doc.steps[i])
        stampStep(&doc.steps[i])
    }

    private func persist(notify: Bool = true) {
        try? file.write(doc)
        WidgetCenter.shared.reloadAllTimelines()
        if notify { onMutate?() }
    }
}
