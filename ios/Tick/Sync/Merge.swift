import Foundation

/// The merge — a pure function mirroring the Go client rule for rule
/// (internal/sync/merge.go, docs/protocol.md). If you change one side,
/// change the other.
enum Merge {
    /// The outbox: every dirty entity, tombstones included.
    static func buildRequest(_ doc: StoreDocument) -> WireSyncRequest {
        var req = WireSyncRequest(device: doc.device, since: doc.since, projects: nil, steps: nil)
        let projects = doc.projects.filter(\.dirty).map(wire(_:))
        let steps = doc.steps.filter(\.dirty).map(wire(_:))
        if !projects.isEmpty { req.projects = projects }
        if !steps.isEmpty { req.steps = steps }
        return req
    }

    /// Applies a response to the document:
    ///  1. ack — pushed entities whose local stamp is unchanged lose `dirty`
    ///     (an edit that raced the push stays queued);
    ///  2. last-writer-wins — a strictly newer remote record replaces the
    ///     local one wholesale; an absent local inserts unless it's a
    ///     tombstone we never knew;
    ///  3. purge — acked tombstones and steps of vanished projects drop out;
    ///  4. the cursor advances to resp.seq.
    static func apply(response: WireSyncResponse, request: WireSyncRequest, to input: StoreDocument) -> (doc: StoreDocument, pulled: Int) {
        var doc = input
        var pulled = 0

        // 1. Ack.
        let pushedProjects = Dictionary(uniqueKeysWithValues: (request.projects ?? []).map { ($0.id, $0.updated) })
        let pushedSteps = Dictionary(uniqueKeysWithValues: (request.steps ?? []).map { ($0.id, $0.updated) })
        for i in doc.projects.indices where pushedProjects[doc.projects[i].id] == TickTime.format(doc.projects[i].updated) {
            doc.projects[i].dirty = false
        }
        for i in doc.steps.indices where pushedSteps[doc.steps[i].id] == TickTime.format(doc.steps[i].updated) {
            doc.steps[i].dirty = false
        }

        // 2. LWW upserts — projects first so steps can find their project.
        for rp in response.projects ?? [] {
            guard let rt = TickTime.parse(rp.updated) else { continue }
            let incoming = Project(id: rp.id, name: rp.name ?? "", path: rp.path,
                                   group: rp.group, deleted: rp.deleted ?? false,
                                   updated: rt, dirty: false)
            if let i = doc.projects.firstIndex(where: { $0.id == rp.id }) {
                if rt > doc.projects[i].updated {
                    doc.projects[i] = incoming
                    pulled += 1
                }
            } else if !incoming.deleted {
                doc.projects.append(incoming)
                pulled += 1
            }
        }
        let knownProjects = Set(doc.projects.filter { !$0.deleted }.map(\.id))
        for rs in response.steps ?? [] {
            guard let rt = TickTime.parse(rs.updated) else { continue }
            let deleted = rs.deleted ?? false
            if let i = doc.steps.firstIndex(where: { $0.id == rs.id }) {
                if rt > doc.steps[i].updated {
                    if deleted {
                        doc.steps[i].deleted = true
                        doc.steps[i].updated = rt
                        doc.steps[i].dirty = false
                    } else if let st = localStep(rs, rt) {
                        doc.steps[i] = st
                    }
                    pulled += 1
                }
            } else if !deleted, knownProjects.contains(rs.projectId), let st = localStep(rs, rt) {
                doc.steps.append(st)
                pulled += 1
            }
        }

        // 3. Purge: acked tombstones, then orphaned steps.
        doc.projects.removeAll { $0.deleted && !$0.dirty }
        let live = Set(doc.projects.map(\.id))
        doc.steps.removeAll { ($0.deleted && !$0.dirty) || !live.contains($0.projectId) }

        // 4. Cursor.
        doc.since = response.seq
        return (doc, pulled)
    }

    // MARK: converters

    private static func wire(_ p: Project) -> WireProject {
        WireProject(id: p.id, name: p.deleted ? nil : p.name, path: p.path, group: p.group,
                    deleted: p.deleted ? true : nil, updated: TickTime.format(p.updated), seq: nil)
    }

    private static func wire(_ s: Step) -> WireStep {
        if s.deleted {
            return WireStep(id: s.id, projectId: s.projectId, text: nil, done: nil, important: nil,
                            rank: nil, created: nil, doneAt: nil, deleted: true,
                            updated: TickTime.format(s.updated), seq: nil)
        }
        return WireStep(id: s.id, projectId: s.projectId, text: s.text,
                        done: s.done ? true : nil, important: s.important ? true : nil,
                        rank: s.rank, created: TickTime.format(s.created),
                        doneAt: s.doneAt.map(TickTime.format), deleted: nil,
                        updated: TickTime.format(s.updated), seq: nil)
    }

    private static func localStep(_ rs: WireStep, _ updated: Date) -> Step? {
        guard let text = rs.text, let rank = rs.rank,
              let created = rs.created.flatMap(TickTime.parse) else { return nil }
        return Step(id: rs.id, projectId: rs.projectId, text: text,
                    done: rs.done ?? false, important: rs.important ?? false,
                    rank: rank, created: created,
                    doneAt: rs.doneAt.flatMap(TickTime.parse),
                    deleted: false, updated: updated, dirty: false)
    }
}
