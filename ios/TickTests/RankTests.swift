import Foundation
import Testing
@testable import Tick

struct RankVectors: Decodable {
    struct MidV: Decodable { let A, B, Want: String }
    struct AfterV: Decodable { let Last, Want: String }
    struct BeforeV: Decodable { let First, Want: String }
    struct InitV: Decodable { let I, N: Int; let Want: String }
    let mid: [MidV]
    let after: [AfterV]
    let before: [BeforeV]
    let initial: [InitV]
}

@Suite struct RankTests {
    func loadVectors() throws -> RankVectors {
        let url = try #require(Bundle(for: BundleToken.self).url(forResource: "rank_vectors", withExtension: "json"))
        return try JSONDecoder().decode(RankVectors.self, from: Data(contentsOf: url))
    }

    @Test func goldenVectorsMatchGo() throws {
        let v = try loadVectors()
        #expect(v.mid.count > 300) // the fixture really loaded
        for c in v.mid {
            #expect(Rank.mid(c.A, c.B) == c.Want, "mid(\(c.A),\(c.B))")
        }
        for c in v.after {
            #expect(Rank.after(c.Last) == c.Want, "after(\(c.Last))")
        }
        for c in v.before {
            #expect(Rank.before(c.First) == c.Want, "before(\(c.First))")
        }
        for c in v.initial {
            #expect(Rank.initial(c.I, c.N) == c.Want, "initial(\(c.I),\(c.N))")
        }
    }

    @Test func randomInsertionsStaySorted() {
        var rng = SystemRandomNumberGenerator()
        var keys: [String] = []
        for _ in 0..<1000 {
            let pos = Int.random(in: 0...keys.count, using: &rng)
            let k: String
            if keys.isEmpty {
                k = Rank.mid("", "")
            } else if pos == 0 {
                k = Rank.before(keys[0])
            } else if pos == keys.count {
                k = Rank.after(keys[keys.count - 1])
            } else {
                k = Rank.mid(keys[pos - 1], keys[pos])
            }
            #expect(Rank.isValid(k))
            keys.insert(k, at: pos)
        }
        #expect(keys == keys.sorted())
        #expect(Set(keys).count == keys.count)
    }
}

private final class BundleToken {}
