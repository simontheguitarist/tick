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
        do {
            if let existing = try file.read() {
                doc = existing
                return
            }
        } catch {
            // Keep the unreadable document for forensics instead of
            // overwriting it; start fresh beside it.
            let aside = file.url.deletingLastPathComponent()
                .appendingPathComponent("store.corrupt-\(Int(Date().timeIntervalSince1970)).json")
            try? FileManager.default.moveItem(at: file.url, to: aside)
        }
        doc = .empty()
        persist(notify: false)
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
        doc.projects[i].name = name.clampedUTF8(Limits.name)
        doc.projects[i].group = Project.normalizedGroup(group)?.clampedUTF8(Limits.group)
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
        doc.steps.append(Step.new(text: text.clampedUTF8(Limits.text), projectId: projectId, rank: Rank.after(last)))
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
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines).clampedUTF8(Limits.text)
        guard !t.isEmpty else { return }
        mutateStep(stepId) { $0.text = t }
    }

    /// Reorder within the currently visible list (SwiftUI onMove semantics):
    /// only the moved step is re-keyed, so a concurrent edit of any other step
    /// merges cleanly. Duplicate ranks (two devices appending offline) leave
    /// no gap to move into, so the project is rebalanced first.
    func move(in projectId: String, visible: [Step], from source: IndexSet, to destination: Int) {
        guard let src = source.first, visible.indices.contains(src) else { return }
        rebalanceIfNeeded(projectId)
        let ids = visible.map(\.id)
        var rest = doc.steps(of: projectId, includeDone: true).filter { ids.contains($0.id) }
        guard rest.indices.contains(src) else { return }
        let moved = rest.remove(at: src)
        let dest = destination > src ? destination - 1 : destination
        let lo = dest > 0 ? rest[dest - 1].rank : ""
        let hi = dest < rest.count ? rest[dest].rank : ""
        mutateStep(moved.id, persist: false) { $0.rank = Rank.mid(lo, hi) }
        rebalanceIfNeeded(projectId)
        persist()
    }

    func moveToTop(_ stepId: String, in projectId: String) {
        rebalanceIfNeeded(projectId)
        let all = doc.steps(of: projectId, includeDone: true)
        guard let first = all.first, first.id != stepId else { return }
        mutateStep(stepId, persist: false) { $0.rank = Rank.before(first.rank) }
        rebalanceIfNeeded(projectId)
        persist()
    }

    /// Delete with undo. The returned copy carries the tombstone's stamp, so
    /// `undoDelete` always lands strictly after the deletion even when the
    /// device clock lags behind the step's own stamp.
    @discardableResult
    func deleteStep(_ stepId: String) -> Step? {
        guard let i = doc.steps.firstIndex(where: { $0.id == stepId }) else { return nil }
        var before = doc.steps[i]
        doc.steps[i].deleted = true
        stampStep(&doc.steps[i])
        before.updated = doc.steps[i].updated
        persist()
        return before
    }

    /// Undo re-activates the same id with a stamp after the tombstone's, so
    /// it wins everywhere even if the deletion already synced. If the step is
    /// live again already (a newer edit elsewhere outvoted the deletion),
    /// that newer version stays — undo must not overwrite it.
    func undoDelete(_ step: Step) {
        if let i = doc.steps.firstIndex(where: { $0.id == step.id }) {
            guard doc.steps[i].deleted else { return }
            var s = step
            s.updated = max(step.updated, doc.steps[i].updated)
            s.deleted = false
            stampStep(&s)
            doc.steps[i] = s
        } else {
            var s = step
            s.deleted = false
            stampStep(&s)
            doc.steps.append(s)
        }
        persist()
    }

    // MARK: sync integration

    /// Replaces the document with a merge result. Persists but does NOT fire
    /// onMutate — a merge must not retrigger sync — and only wakes the widget
    /// when something it can show actually changed (acks flip dirty flags,
    /// which the widget never displays).
    func applyMerged(_ merged: StoreDocument, pulled: Int) {
        guard merged != doc else { return }
        doc = merged
        persist(notify: false, widget: pulled > 0)
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

    private func mutateStep(_ id: String, persist: Bool = true, _ change: (inout Step) -> Void) {
        guard let i = doc.steps.firstIndex(where: { $0.id == id }) else { return }
        change(&doc.steps[i])
        stampStep(&doc.steps[i])
        if persist { self.persist() }
    }

    private func stampStep(_ s: inout Step) {
        s.updated = TickTime.bump(after: s.updated)
        s.dirty = true
    }

    private func stampProject(_ p: inout Project) {
        p.updated = TickTime.bump(after: p.updated)
        p.dirty = true
    }

    /// Re-keys a project's steps when a rank grew past the limit or two steps
    /// share a rank. Pure: the caller persists. Mirrors Project.Rebalance in Go.
    private func rebalanceIfNeeded(_ projectId: String) {
        let all = doc.steps(of: projectId, includeDone: true)
        let tooLong = all.contains { $0.rank.count > Rank.maxLength }
        let duplicates = zip(all, all.dropFirst()).contains { $0.rank == $1.rank }
        guard tooLong || duplicates else { return }
        for (i, step) in all.enumerated() {
            mutateStep(step.id, persist: false) { $0.rank = Rank.initial(i, all.count) }
        }
    }

    private func persist(notify: Bool = true, widget: Bool = true) {
        try? file.write(doc)
        if widget { WidgetCenter.shared.reloadAllTimelines() }
        if notify { onMutate?() }
    }
}
