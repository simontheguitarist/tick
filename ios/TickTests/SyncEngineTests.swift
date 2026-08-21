import Foundation
import Testing
@testable import Tick

/// Records requests, replays scripted outcomes.
actor StubTransport: SyncTransport {
    enum Script {
        case ok(WireSyncResponse)
        case fail(APIError)
    }

    private(set) var requests: [WireSyncRequest] = []
    private var script: [Script]

    init(script: [Script]) {
        self.script = script
    }

    func sync(_ req: WireSyncRequest) async throws -> WireSyncResponse {
        requests.append(req)
        guard !script.isEmpty else { return WireSyncResponse(seq: req.since, projects: nil, steps: nil) }
        switch script.removeFirst() {
        case .ok(let resp): return resp
        case .fail(let err): throw err
        }
    }
}

@MainActor
@Suite struct SyncEngineTests {
    func makeStore() -> Store {
        Store(file: StoreFile(url: FileManager.default.temporaryDirectory
            .appendingPathComponent("tick-engine-\(UUID().uuidString).json")))
    }

    @Test func offlineKeepsDirtyAndReportsQuietly() async {
        let store = makeStore()
        let transport = StubTransport(script: [.fail(.transport("offline"))])
        let engine = SyncEngine(store: store) { transport }
        store.onMutate = nil // drive manually in tests
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("queued", to: p.id)

        await engine.syncNow()
        #expect(engine.status == .offline)
        #expect(store.pendingPushCount == 2, "dirty state must survive an offline sync")
        #expect(store.doc.since == 0)
    }

    @Test func reconnectPushesExactlyTheDirtySet() async {
        let store = makeStore()
        let transport = StubTransport(script: [.ok(WireSyncResponse(seq: 5, projects: nil, steps: nil))])
        let engine = SyncEngine(store: store) { transport }
        store.onMutate = nil
        let p = store.addProject(name: "proj", group: nil)
        store.addStep("a", to: p.id)

        await engine.syncNow()
        #expect(engine.status == .idle)
        #expect(store.pendingPushCount == 0)
        #expect(store.doc.since == 5)
        let reqs = await transport.requests
        #expect(reqs.count == 1)
        #expect(reqs[0].projects?.count == 1 && reqs[0].steps?.count == 1)
    }

    @Test func authFailureStopsWithoutRetry() async {
        let store = makeStore()
        let transport = StubTransport(script: [.fail(.unauthorized)])
        let engine = SyncEngine(store: store) { transport }
        store.onMutate = nil
        _ = store.addProject(name: "proj", group: nil)

        await engine.syncNow()
        #expect(engine.status == .authFailed)
        let reqs = await transport.requests
        #expect(reqs.count == 1)
    }

    @Test func unconfiguredEngineIsInert() async {
        let store = makeStore()
        let engine = SyncEngine(store: store) { nil }
        await engine.syncNow()
        #expect(engine.status == .idle)
        #expect(!engine.isConfigured)
    }
}
