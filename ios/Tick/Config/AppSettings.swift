import Foundation

/// Non-secret preferences, UserDefaults-backed. The token lives in the
/// keychain; `isPaired` needs both halves.
@MainActor
@Observable
final class AppSettings {
    private let defaults: UserDefaults
    let keychain = KeychainStore()

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        baseURLString = defaults.string(forKey: "baseURL") ?? ""
        showDoneByDefault = defaults.bool(forKey: "showDone")
        lastOpenedProjectID = defaults.string(forKey: "lastProject")
    }

    var baseURLString: String {
        didSet { defaults.set(baseURLString, forKey: "baseURL") }
    }
    var showDoneByDefault: Bool {
        didSet { defaults.set(showDoneByDefault, forKey: "showDone") }
    }
    var lastOpenedProjectID: String? {
        didSet { defaults.set(lastOpenedProjectID, forKey: "lastProject") }
    }

    var baseURL: URL? { URL(string: baseURLString) }
    var isPaired: Bool { baseURL != nil && keychain.readToken() != nil }

    func pair(server: URL, token: String) {
        keychain.saveToken(token)
        baseURLString = server.absoluteString
    }

    func unpair() {
        keychain.deleteToken()
        baseURLString = ""
    }
}
