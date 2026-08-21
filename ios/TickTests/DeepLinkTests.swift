import Foundation
import Testing
@testable import Tick

@Suite struct DeepLinkTests {
    @Test func pairLinkParses() throws {
        let url = URL(string: "tick://pair?url=https%3A%2F%2Ftick.dply.ch&token=abc123")!
        let link = try #require(DeepLink(url))
        #expect(link == .pair(server: URL(string: "https://tick.dply.ch")!, token: "abc123"))
    }

    @Test func pairAcceptsLocalhostHTTP() throws {
        let url = URL(string: "tick://pair?url=http%3A%2F%2Flocalhost%3A8080&token=t")!
        #expect(DeepLink(url) != nil)
    }

    @Test func projectLinkParses() {
        #expect(DeepLink(URL(string: "tick://project/abc-123")!) == .project(id: "abc-123"))
    }

    @Test func garbageRejected() {
        for bad in ["tick://pair?url=https%3A%2F%2Fx.ch", // no token
                    "tick://pair?token=t",                 // no url
                    "tick://pair?url=ftp%3A%2F%2Fx&token=t", // bad scheme
                    "tick://project/",                     // no id
                    "https://tick.dply.ch/",               // wrong scheme
                    "tick://unknown"] {
            #expect(DeepLink(URL(string: bad)!) == nil, Comment(rawValue: bad))
        }
    }
}
