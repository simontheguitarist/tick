import Foundation

/// Where the document lives and how it is read/written. The App Group
/// container makes the same file visible to the app and the widget; writes are
/// atomic (temp file + rename under the hood) like every other tick store.
struct StoreFile: Sendable {
    static let appGroupID = "group.ch.simk.tick"

    let url: URL

    static func appGroup() -> StoreFile {
        let base = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID)
            ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        return StoreFile(url: base.appendingPathComponent("store.json"))
    }

    /// nil when the file doesn't exist yet or can't be parsed — the caller
    /// starts fresh rather than crashing on a corrupt document.
    func read() -> StoreDocument? {
        guard let data = try? Data(contentsOf: url) else { return nil }
        return try? TickTime.decoder().decode(StoreDocument.self, from: data)
    }

    func write(_ doc: StoreDocument) throws {
        let data = try TickTime.encoder().encode(doc)
        try data.write(to: url, options: .atomic)
    }
}
