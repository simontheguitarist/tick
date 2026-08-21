import Foundation

/// Drives the exchanges: on launch, on foreground, debounced after every
/// mutation, on pull-to-refresh — always as one in-flight run with a trailing
/// rerun, offline backoff doubling 2s→60s. UI never blocks: build request from
/// the current document, await the network, merge, hand the result back to the
/// store on the main actor.
@MainActor
@Observable
final class SyncEngine {
    enum Status: Equatable {
        case idle
        case syncing
        case offline
        case authFailed
    }

    private(set) var status: Status = .idle
    private(set) var lastSynced: Date?

    private let store: Store
    private let makeTransport: @MainActor () -> (any SyncTransport)?
    private var pending: Task<Void, Never>?
    private var rerun = false
    private var failures = 0
    /// Bumped by a full resync so an exchange that was in flight against the
    /// old cursor/dirty state is discarded instead of applied to the reset
    /// document.
    private var generation = 0

    init(store: Store, makeTransport: @escaping @MainActor () -> (any SyncTransport)?) {
        self.store = store
        self.makeTransport = makeTransport
        store.onMutate = { [weak self] in self?.kick() }
    }

    var isConfigured: Bool { makeTransport() != nil }

    /// Debounced trigger: rapid edits collapse into one exchange ~300ms after
    /// the last.
    func kick(after delay: Duration = .milliseconds(300)) {
        guard isConfigured else { return }
        pending?.cancel()
        pending = Task { [weak self] in
            try? await Task.sleep(for: delay)
            guard !Task.isCancelled else { return }
            await self?.runOnce()
        }
    }

    /// Immediate sync (launch, foreground, pull-to-refresh).
    func syncNow() async {
        guard isConfigured else { return }
        pending?.cancel()
        failures = 0
        await runOnce()
    }

    /// Re-push and re-pull the world (pairing, "re-sync everything").
    func fullResync() async {
        generation += 1
        store.markAllDirtyAndResetCursor()
        await syncNow()
    }

    private func runOnce() async {
        if status == .syncing {
            rerun = true
            return
        }
        repeat {
            rerun = false
            await exchange()
        } while rerun && status == .idle
    }

    private func exchange() async {
        guard let transport = makeTransport() else { return }
        status = .syncing
        let gen = generation
        let req = Merge.buildRequest(store.doc)
        do {
            let resp = try await transport.sync(req)
            guard gen == generation else { // superseded by a full resync mid-flight
                status = .idle
                rerun = true
                return
            }
            let (merged, pulled) = Merge.apply(response: resp, request: req, to: store.doc)
            store.applyMerged(merged, pulled: pulled)
            lastSynced = .now
            status = .idle
            failures = 0
        } catch let e as APIError where e.isAuth {
            status = .authFailed
        } catch {
            failures += 1
            status = .offline
            let backoff = min(60.0, pow(2.0, Double(failures)))
            kick(after: .seconds(backoff))
        }
    }
}
