import Foundation

/// Wire DTOs — exactly the contract in docs/protocol.md. Optional fields
/// mirror the server's omitempty; unknown keys are ignored for forward compat.
struct WireProject: Codable, Sendable {
    var id: String
    var name: String?
    var path: String?
    var group: String?
    var deleted: Bool?
    var updated: String
    var seq: Int64?
}

struct WireStep: Codable, Sendable {
    var id: String
    var projectId: String
    var text: String?
    var done: Bool?
    var important: Bool?
    var rank: String?
    var created: String?
    var doneAt: String?
    var deleted: Bool?
    var updated: String
    var seq: Int64?

    enum CodingKeys: String, CodingKey {
        case id, text, done, important, rank, created, deleted, updated, seq
        case projectId = "project_id"
        case doneAt = "done_at"
    }
}

struct WireSyncRequest: Codable, Sendable {
    var device: String
    var since: Int64
    var projects: [WireProject]?
    var steps: [WireStep]?
}

struct WireSyncResponse: Codable, Sendable {
    var seq: Int64
    var projects: [WireProject]?
    var steps: [WireStep]?
}

enum APIError: Error, Equatable {
    case unauthorized
    case server(Int, String)
    case decoding
    case transport(String)

    var isAuth: Bool { self == .unauthorized }
}

/// One request, one endpoint. Injectable so tests replay canned exchanges.
protocol SyncTransport: Sendable {
    func sync(_ req: WireSyncRequest) async throws -> WireSyncResponse
}

/// The real transport. Fails fast when offline (no waitsForConnectivity —
/// an offline sync should report offline, not hang half a minute).
struct SyncClient: SyncTransport {
    var baseURL: URL
    var token: String

    private static let session: URLSession = {
        let cfg = URLSessionConfiguration.ephemeral
        cfg.waitsForConnectivity = false
        cfg.timeoutIntervalForRequest = 10
        return URLSession(configuration: cfg)
    }()

    func sync(_ req: WireSyncRequest) async throws -> WireSyncResponse {
        var r = URLRequest(url: baseURL.appending(path: "/v1/sync"))
        r.httpMethod = "POST"
        r.setValue("application/json", forHTTPHeaderField: "Content-Type")
        r.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        r.httpBody = try JSONEncoder().encode(req)

        let data: Data
        let resp: URLResponse
        do {
            (data, resp) = try await Self.session.data(for: r)
        } catch {
            throw APIError.transport(error.localizedDescription)
        }
        guard let http = resp as? HTTPURLResponse else { throw APIError.decoding }
        switch http.statusCode {
        case 200:
            guard let out = try? JSONDecoder().decode(WireSyncResponse.self, from: data) else {
                throw APIError.decoding
            }
            return out
        case 401:
            throw APIError.unauthorized
        default:
            let msg = String(data: data.prefix(300), encoding: .utf8) ?? ""
            throw APIError.server(http.statusCode, msg)
        }
    }
}
