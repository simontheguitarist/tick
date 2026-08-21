import Foundation

/// Wire/store timestamp handling. One canonical form everywhere: fixed-width
/// UTC with milliseconds — matching the Go side, where string order == time
/// order. All stamps are truncated to the millisecond so a value survives a
/// round-trip through JSON unchanged.
enum TickTime {
    private static let wire: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'"
        return f
    }()
    // ISO8601DateFormatter is documented thread-safe but not Sendable-annotated.
    nonisolated(unsafe) private static let isoFrac: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    nonisolated(unsafe) private static let iso: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    static func now() -> Date { truncate(Date()) }

    static func truncate(_ d: Date) -> Date {
        Date(timeIntervalSince1970: (d.timeIntervalSince1970 * 1000).rounded(.down) / 1000)
    }

    /// Strictly-after stamp: the monotonic guard that keeps last-writer-wins
    /// sane when the clock stalls or jumps backwards.
    static func bump(after prev: Date) -> Date {
        let n = now()
        return n > prev ? n : prev.addingTimeInterval(0.001)
    }

    static func format(_ d: Date) -> String { wire.string(from: truncate(d)) }

    static func parse(_ s: String) -> Date? {
        if let d = isoFrac.date(from: s) { return d }
        return iso.date(from: s)
    }

    /// JSON coders for the local store document (canonical strings, not epochs).
    static func encoder() -> JSONEncoder {
        let e = JSONEncoder()
        e.outputFormatting = [.prettyPrinted, .sortedKeys]
        e.dateEncodingStrategy = .custom { date, enc in
            var c = enc.singleValueContainer()
            try c.encode(format(date))
        }
        return e
    }

    static func decoder() -> JSONDecoder {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { dec in
            let c = try dec.singleValueContainer()
            let s = try c.decode(String.self)
            guard let date = parse(s) else {
                throw DecodingError.dataCorruptedError(in: c, debugDescription: "bad timestamp \(s)")
            }
            return date
        }
        return d
    }
}

/// Wire field limits (docs/protocol.md). The server rejects a whole batch
/// over these, so every input path clamps to them at the source.
enum Limits {
    static let text = 4096
    static let name = 256
    static let group = 128
}

extension String {
    /// At most `maxBytes` of UTF-8, cut on a character boundary.
    func clampedUTF8(_ maxBytes: Int) -> String {
        var s = self
        while s.utf8.count > maxBytes { s.removeLast() }
        return s
    }
}

/// A project. `path` is Mac-side metadata (where the repo lives); the phone
/// stores and echoes it opaquely so a push never strips it. An entity with
/// `deleted` is a tombstone: it stays in the document while `dirty`, and the
/// purge sweep drops it once the server has acknowledged it.
struct Project: Codable, Identifiable, Sendable, Equatable, Hashable {
    var id: String
    var name: String
    var path: String?
    var group: String?
    var deleted: Bool
    var updated: Date
    var dirty: Bool

    static func new(name: String, group: String? = nil) -> Project {
        Project(id: UUID().uuidString.lowercased(), name: name, path: nil,
                group: normalizedGroup(group), deleted: false, updated: TickTime.now(), dirty: true)
    }

    static func normalizedGroup(_ g: String?) -> String? {
        guard let g = g?.trimmingCharacters(in: .whitespaces), !g.isEmpty else { return nil }
        return g
    }
}

/// A step. Order is the meaning: `rank` sorts byte-wise, ties break on id —
/// identical to the CLI.
struct Step: Codable, Identifiable, Sendable, Equatable, Hashable {
    var id: String
    var projectId: String
    var text: String
    var done: Bool
    var important: Bool
    var rank: String
    var created: Date
    var doneAt: Date?
    var deleted: Bool
    var updated: Date
    var dirty: Bool

    static func new(text: String, projectId: String, rank: String) -> Step {
        let now = TickTime.now()
        return Step(id: UUID().uuidString.lowercased(), projectId: projectId, text: text,
                    done: false, important: false, rank: rank, created: now,
                    doneAt: nil, deleted: false, updated: now, dirty: true)
    }
}

/// The whole on-disk document in the App Group container — data, outbox state
/// (dirty flags + tombstones) and the pull cursor in one file, written
/// atomically, so a crash can never separate them.
struct StoreDocument: Codable, Sendable, Equatable {
    var version: Int
    var device: String
    var since: Int64
    var projects: [Project]
    var steps: [Step]

    static func empty() -> StoreDocument {
        StoreDocument(version: 1,
                      device: "iphone-" + String(UUID().uuidString.lowercased().prefix(8)),
                      since: 0, projects: [], steps: [])
    }
}

// MARK: - Queries (pure, shared with the widget)

extension StoreDocument {
    var liveProjects: [Project] {
        projects.filter { !$0.deleted }.sorted { a, b in
            a.name.localizedCaseInsensitiveCompare(b.name) == .orderedSame
                ? a.id < b.id
                : a.name.localizedCaseInsensitiveCompare(b.name) == .orderedAscending
        }
    }

    /// Grouped sections mirroring the TUI overview: named groups alphabetical,
    /// ungrouped last (unlabelled when it's the only section).
    var sections: [(group: String?, projects: [Project])] {
        let live = liveProjects
        let grouped = Dictionary(grouping: live.filter { $0.group != nil }, by: { $0.group! })
        var out: [(String?, [Project])] = grouped.keys.sorted().map { ($0, grouped[$0]!) }
        let ungrouped = live.filter { $0.group == nil }
        if !ungrouped.isEmpty { out.append((nil, ungrouped)) }
        return out
    }

    func project(_ id: String) -> Project? {
        projects.first { $0.id == id && !$0.deleted }
    }

    /// Steps of a project in rank order. Done steps stay in place when shown.
    func steps(of projectId: String, includeDone: Bool) -> [Step] {
        steps.filter { $0.projectId == projectId && !$0.deleted && (includeDone || !$0.done) }
            .sorted { $0.rank == $1.rank ? $0.id < $1.id : $0.rank < $1.rank }
    }

    func openCount(of projectId: String) -> Int {
        steps.count { $0.projectId == projectId && !$0.deleted && !$0.done }
    }

    /// The widget's default project: the one whose steps moved most recently.
    var mostRecentlyActiveProject: Project? {
        let activity = Dictionary(grouping: steps.filter { !$0.deleted }, by: \.projectId)
            .mapValues { $0.map(\.updated).max() ?? .distantPast }
        return liveProjects.max { a, b in
            (activity[a.id] ?? a.updated) < (activity[b.id] ?? b.updated)
        }
    }
}
