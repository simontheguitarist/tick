import SwiftUI

/// A pairing request arriving by deep link or QR — always confirmed by a
/// human before it can replace credentials.
struct PairRequest: Identifiable, Equatable {
    let server: URL
    let token: String
    var id: String { server.absoluteString }
}

/// The app's object graph: one store, one engine, one settings object, plus
/// navigation state. Built once at launch.
@MainActor
@Observable
final class AppModel {
    let settings = AppSettings()
    let store = Store()
    let engine: SyncEngine
    var path: [String] = [] // project ids on the navigation stack
    var pendingPair: PairRequest?

    init() {
        let settings = self.settings
        engine = SyncEngine(store: store) {
            guard let url = settings.baseURL, let token = settings.keychain.readToken() else { return nil }
            return SyncClient(baseURL: url, token: token)
        }
    }

    func handle(_ url: URL) {
        switch DeepLink(url) {
        case .pair(let server, let token):
            pendingPair = PairRequest(server: server, token: token)
        case .project(let id):
            if store.doc.project(id) != nil {
                path = [id]
            }
        case nil:
            break
        }
    }

    func confirmPair(_ req: PairRequest) {
        settings.pair(server: req.server, token: req.token)
        Haptics.paired()
        pendingPair = nil
        Task { await engine.fullResync() }
    }

    /// Open where you left off — the phone-side sibling of bare `tick`.
    func restoreLastProject() {
        if path.isEmpty, let id = settings.lastOpenedProjectID, store.doc.project(id) != nil {
            path = [id]
        }
    }
}

@main
struct TickApp: App {
    @State private var app = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(app)
                .onOpenURL { app.handle($0) }
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active {
                        Task { await app.engine.syncNow() }
                    }
                }
        }
    }
}
