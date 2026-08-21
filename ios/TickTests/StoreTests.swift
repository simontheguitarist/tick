import Foundation
import Testing
@testable import Tick

@Suite struct StoreFileTests {
    func tempFile() -> StoreFile {
        StoreFile(url: FileManager.default.temporaryDirectory
            .appendingPathComponent("tick-test-\(UUID().uuidString).json"))
    }

    @Test func roundTripAndTimestampFormat() throws {
        let file = tempFile()
        var doc = StoreDocument.empty()
        doc.projects = [Project.new(name: "proj")]
        doc.steps = [Step.new(text: "hello", projectId: doc.projects[0].id, rank: "V")]
        try file.write(doc)
        let loaded = try #require(try file.read())
        #expect(loaded == doc)

        // The document stores canonical fixed-width UTC-ms strings.
        let raw = try String(contentsOf: file.url, encoding: .utf8)
        let match = try #require(raw.firstMatch(of: /"updated" : "([^"]+)"/))
        let stamp = String(match.1)
        #expect(stamp.count == 24 && stamp.hasSuffix("Z"), "not canonical: \(stamp)")
    }

    @Test func missingFileMeansNil() throws {
        #expect(try tempFile().read() == nil)
    }

    @Test func corruptFileThrowsInsteadOfVanishing() throws {
        let file = tempFile()
        try Data("{not json".utf8).write(to: file.url)
        #expect(throws: (any Error).self) { try file.read() }
    }

    @Test func bumpIsMonotonic() {
        var t = TickTime.now()
        for _ in 0..<100 {
            let n = TickTime.bump(after: t)
            #expect(n > t)
            t = n
        }
    }
}

@MainActor
@Suite struct StoreMutationTests {
    func freshStore() -> Store {
        Store(file: StoreFile(url: FileManager.default.temporaryDirectory
            .appendingPathComponent("tick-store-\(UUID().uuidString).json")))
    }

    @Test func addStepsAppendInRankOrder() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("one", to: p.id)
        store.addStep("two", to: p.id)
        store.addStep("three", to: p.id)
        let steps = store.doc.steps(of: p.id, includeDone: true)
        #expect(steps.map(\.text) == ["one", "two", "three"])
        #expect(steps.allSatisfy { Rank.isValid($0.rank) && $0.dirty })
    }

    @Test func moveReRanksOnlyTheMovedStep() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        for t in ["a", "b", "c"] { store.addStep(t, to: p.id) }
        let before = store.doc.steps(of: p.id, includeDone: true)
        // Move "c" to the front (SwiftUI onMove semantics).
        store.move(in: p.id, visible: before, from: IndexSet(integer: 2), to: 0)
        let after = store.doc.steps(of: p.id, includeDone: true)
        #expect(after.map(\.text) == ["c", "a", "b"])
        let changed = zip(before.sorted { $0.id < $1.id }, after.sorted { $0.id < $1.id })
            .filter { $0.rank != $1.rank }
        #expect(changed.count == 1, "a move must re-key exactly one step")
    }

    @Test func deleteThenUndoRestoresWithNewerStamp() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("keep me", to: p.id)
        let step = store.doc.steps(of: p.id, includeDone: true)[0]
        let deleted = store.deleteStep(step.id)!
        #expect(store.doc.steps(of: p.id, includeDone: true).isEmpty)
        store.undoDelete(deleted)
        let restored = store.doc.steps(of: p.id, includeDone: true)
        #expect(restored.count == 1 && restored[0].id == step.id)
        #expect(restored[0].updated > step.updated, "undo must outrank the tombstone")
    }

    @Test func deleteProjectTombstonesItsSteps() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("a", to: p.id)
        store.deleteProject(p.id)
        #expect(store.doc.project(p.id) == nil)
        #expect(store.doc.steps.allSatisfy { $0.deleted && $0.dirty })
        #expect(store.doc.projects.first?.deleted == true)
    }

    @Test func sectionsGroupLikeTheTUI() {
        let store = freshStore()
        _ = store.addProject(name: "zeta", group: nil)
        _ = store.addProject(name: "alpha", group: "work")
        _ = store.addProject(name: "beta", group: "fun")
        let sections = store.doc.sections
        #expect(sections.map(\.group) == ["fun", "work", nil])
        #expect(sections.last?.projects.map(\.name) == ["zeta"])
    }
}

@MainActor
@Suite struct StoreUndoTests {
    func freshStore() -> Store {
        Store(file: StoreFile(url: FileManager.default.temporaryDirectory
            .appendingPathComponent("tick-undo-\(UUID().uuidString).json")))
    }

    @Test func undoOutranksTombstoneEvenWithLaggingClock() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("x", to: p.id)
        // Pretend the step came from a device whose clock is 5s ahead.
        var doc = store.doc
        doc.steps[0].updated = TickTime.now().addingTimeInterval(5)
        store.applyMerged(doc, pulled: 1)
        let step = store.doc.steps[0]
        let deleted = store.deleteStep(step.id)!
        let tombstone = store.doc.steps[0].updated
        #expect(deleted.updated == tombstone)
        store.undoDelete(deleted)
        #expect(store.doc.steps[0].updated > tombstone)
        #expect(!store.doc.steps[0].deleted)
    }

    @Test func undoKeepsNewerLiveCopy() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("original", to: p.id)
        let step = store.doc.steps[0]
        let deleted = store.deleteStep(step.id)!
        // A newer edit from elsewhere resurrected it.
        var doc = store.doc
        doc.steps[0].deleted = false
        doc.steps[0].text = "edited elsewhere"
        doc.steps[0].updated = deleted.updated.addingTimeInterval(60)
        doc.steps[0].dirty = false
        store.applyMerged(doc, pulled: 1)
        store.undoDelete(deleted)
        #expect(store.doc.steps.count == 1)
        #expect(store.doc.steps[0].text == "edited elsewhere")
    }

    @Test func moveWithDuplicateRanksStillMoves() {
        let store = freshStore()
        let p = store.addProject(name: "proj", group: nil)
        for t in ["a", "b", "c"] { store.addStep(t, to: p.id) }
        var doc = store.doc
        doc.steps[2].rank = doc.steps[1].rank
        store.applyMerged(doc, pulled: 1)
        let visible = store.doc.steps(of: p.id, includeDone: true)
        store.move(in: p.id, visible: visible, from: IndexSet(integer: 2), to: 0)
        let after = store.doc.steps(of: p.id, includeDone: true)
        #expect(after[0].id == visible[2].id)
        #expect(Set(after.map(\.rank)).count == 3)
    }
}
