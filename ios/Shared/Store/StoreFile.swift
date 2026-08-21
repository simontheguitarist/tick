import Foundation

/// Where the document lives and how it is read/written. The App Group
/// container makes the same file visible to the app and the widget; writes are
/// atomic (temp file + rename under the hood) like every other tick store.
struct StoreFile: Sendable {
    static let appGroupID = "group.ch.simk.tick"

    let url: URL

    static func appGroup() -> StoreFile {
        // No silent fallback: a build without the App Group entitlement would
        // give the app and the widget two different stores and nobody would
        // notice until steps "vanished". Fail where the misconfiguration is.
        guard let base = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) else {
            preconditionFailure("App Group \(appGroupID) is missing from the entitlements")
        }
        return StoreFile(url: base.appendingPathComponent("store.json"))
    }

    /// nil when no document exists yet; throws when one exists but can't be
    /// parsed — the caller decides what to do with a corrupt file, it is
    /// never silently overwritten.
    func read() throws -> StoreDocument? {
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        let data = try Data(contentsOf: url)
        return try TickTime.decoder().decode(StoreDocument.self, from: data)
    }

    func write(_ doc: StoreDocument) throws {
        let data = try TickTime.encoder().encode(doc)
        try data.write(to: url, options: .atomic)
    }
}
