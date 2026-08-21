import Foundation

/// The `tick://` links the app understands:
///   tick://pair?url=<server>&token=<secret>   (QR code / CLI pairing)
///   tick://project/<id>                       (widget tap)
enum DeepLink: Equatable {
    case pair(server: URL, token: String)
    case project(id: String)

    init?(_ url: URL) {
        guard url.scheme == "tick" else { return nil }
        switch url.host() {
        case "pair":
            guard let comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
                  let raw = comps.queryItems?.first(where: { $0.name == "url" })?.value,
                  let token = comps.queryItems?.first(where: { $0.name == "token" })?.value,
                  !token.isEmpty,
                  let server = URL(string: raw),
                  server.scheme == "https" || server.scheme == "http",
                  server.host() != nil
            else { return nil }
            self = .pair(server: server, token: token)
        case "project":
            let id = url.path().trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            guard !id.isEmpty else { return nil }
            self = .project(id: id)
        default:
            return nil
        }
    }
}
