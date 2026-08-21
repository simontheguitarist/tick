import Foundation
import Testing
@testable import Tick

/// The merge table — same cases the Go suite pins, so both clients implement
/// one rule.
@Suite struct MergeTests {
    func date(_ s: String) -> Date { TickTime.parse(s)! }

    func baseDoc() -> StoreDocument {
        var doc = StoreDocument.empty()
        doc.projects = [Project(id: "p1", name: "proj", path: "/mac/proj", group: nil,
                                deleted: false, updated: date("2026-01-01T00:00:00.000Z"), dirty: false)]
        doc.steps = [Step(id: "s1", projectId: "p1", text: "hello", done: false, important: false,
                          rank: "V", created: date("2026-01-01T00:00:00.000Z"), doneAt: nil,
                          deleted: false, updated: date("2026-01-01T00:00:00.000Z"), dirty: false)]
        return doc
    }

    @Test func remoteNewerWins() {
        let resp = WireSyncResponse(seq: 7, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "edited elsewhere", done: true, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: "2026-01-02T00:00:00.000Z",
                     deleted: nil, updated: "2026-01-02T00:00:00.000Z", seq: 7)])
        let (doc, pulled) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: baseDoc())
        #expect(pulled == 1)
        #expect(doc.steps[0].text == "edited elsewhere")
        #expect(doc.steps[0].done)
        #expect(!doc.steps[0].dirty)
        #expect(doc.since == 7)
    }

    @Test func localNewerKept() {
        var input = baseDoc()
        input.steps[0].text = "local newer"
        input.steps[0].updated = date("2026-01-03T00:00:00.000Z")
        input.steps[0].dirty = true
        let resp = WireSyncResponse(seq: 3, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "stale", done: nil, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: nil,
                     deleted: nil, updated: "2026-01-02T00:00:00.000Z", seq: 3)])
        let (doc, _) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: input)
        #expect(doc.steps[0].text == "local newer")
        #expect(doc.steps[0].dirty)
    }

    @Test func midFlightEditStaysDirty() {
        var input = baseDoc()
        // Pushed with T1; edited again to T2 before the response landed.
        let req = WireSyncRequest(device: "d", since: 0, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "hello", done: nil, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: nil,
                     deleted: nil, updated: "2026-01-01T00:00:00.000Z", seq: nil)])
        input.steps[0].updated = date("2026-01-05T00:00:00.000Z")
        input.steps[0].dirty = true
        var (doc, _) = Merge.apply(response: WireSyncResponse(seq: 1, projects: nil, steps: nil), request: req, to: input)
        #expect(doc.steps[0].dirty, "mid-flight edit lost its dirty flag")

        // An unchanged push is acked.
        let req2 = WireSyncRequest(device: "d", since: 0, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "x", done: nil, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: nil,
                     deleted: nil, updated: "2026-01-05T00:00:00.000Z", seq: nil)])
        (doc, _) = Merge.apply(response: WireSyncResponse(seq: 2, projects: nil, steps: nil), request: req2, to: doc)
        #expect(!doc.steps[0].dirty)
    }

    @Test func tombstoneAckPurgesAndGuards() {
        var input = baseDoc()
        // Local deletion, pushed and acked -> entity purged.
        input.steps[0].deleted = true
        input.steps[0].updated = date("2026-01-02T00:00:00.000Z")
        input.steps[0].dirty = true
        let req = WireSyncRequest(device: "d", since: 0, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: nil, done: nil, important: nil,
                     rank: nil, created: nil, doneAt: nil, deleted: true,
                     updated: "2026-01-02T00:00:00.000Z", seq: nil)])
        var (doc, _) = Merge.apply(response: WireSyncResponse(seq: 1, projects: nil, steps: nil), request: req, to: input)
        #expect(doc.steps.isEmpty, "acked tombstone must purge")

        // A pending (dirty) tombstone must not be resurrected by an older live copy.
        var input2 = baseDoc()
        input2.steps[0].deleted = true
        input2.steps[0].updated = date("2026-01-02T00:00:00.000Z")
        input2.steps[0].dirty = true
        let resp = WireSyncResponse(seq: 2, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "zombie", done: nil, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: nil,
                     deleted: nil, updated: "2026-01-01T00:00:00.000Z", seq: 2)])
        (doc, _) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: input2)
        #expect(doc.steps.count == 1 && doc.steps[0].deleted, "older live copy resurrected a deletion")

        // A newer live copy (done later elsewhere) overrides the local deletion.
        let resp2 = WireSyncResponse(seq: 3, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "done later", done: true, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: "2026-01-03T00:00:00.000Z",
                     deleted: nil, updated: "2026-01-03T00:00:00.000Z", seq: 3)])
        (doc, _) = Merge.apply(response: resp2, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: doc)
        #expect(doc.steps.count == 1 && !doc.steps[0].deleted && doc.steps[0].done)
    }

    @Test func projectTombstoneCascades() {
        let resp = WireSyncResponse(seq: 9, projects: [
            WireProject(id: "p1", name: nil, path: nil, group: nil, deleted: true,
                        updated: "2026-02-01T00:00:00.000Z", seq: 9)], steps: nil)
        let (doc, _) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: baseDoc())
        #expect(doc.projects.isEmpty)
        #expect(doc.steps.isEmpty, "steps must not outlive their project")
    }

    @Test func orphanStepSkipped() {
        let resp = WireSyncResponse(seq: 1, projects: nil, steps: [
            WireStep(id: "sX", projectId: "ghost", text: "orphan", done: nil, important: nil,
                     rank: "V", created: "2026-01-01T00:00:00.000Z", doneAt: nil,
                     deleted: nil, updated: "2026-01-01T00:00:00.000Z", seq: 1)])
        let (doc, pulled) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: baseDoc())
        #expect(pulled == 0)
        #expect(doc.steps.count == 1)
    }

    @Test func buildRequestCarriesOnlyDirty() {
        var input = baseDoc()
        input.steps[0].dirty = true
        input.projects[0].dirty = false
        let req = Merge.buildRequest(input)
        #expect(req.projects == nil)
        #expect(req.steps?.count == 1)
        // The path must ride along on project pushes so it never gets stripped.
        input.projects[0].dirty = true
        let req2 = Merge.buildRequest(input)
        #expect(req2.projects?.first?.path == "/mac/proj")
    }
}

@Suite struct MergeTieTests {
    @Test func equalStampCleanLocalAdoptsServerWinner() {
        var doc = StoreDocument.empty()
        let t = TickTime.parse("2026-01-01T00:00:00.000Z")!
        doc.projects = [Project(id: "p1", name: "proj", path: nil, group: nil, deleted: false, updated: t, dirty: false)]
        doc.steps = [Step(id: "s1", projectId: "p1", text: "ours", done: false, important: false, rank: "V",
                          created: t, doneAt: nil, deleted: false, updated: t, dirty: false)]
        let resp = WireSyncResponse(seq: 2, projects: nil, steps: [
            WireStep(id: "s1", projectId: "p1", text: "theirs", done: nil, important: nil, rank: "V",
                     created: "2026-01-01T00:00:00.000Z", doneAt: nil, deleted: nil,
                     updated: "2026-01-01T00:00:00.000Z", seq: 2)])
        let (out, pulled) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: doc)
        #expect(pulled == 1)
        #expect(out.steps[0].text == "theirs")

        // An identical echo is a no-op.
        let (again, pulled2) = Merge.apply(response: resp, request: WireSyncRequest(device: "d", since: 0, projects: nil, steps: nil), to: out)
        #expect(pulled2 == 0)
        #expect(again.steps == out.steps)
    }
}
