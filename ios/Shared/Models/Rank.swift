import Foundation

/// Fractional-index keys that order steps. This is a byte-for-byte port of the
/// Go implementation (internal/rank/rank.go); the golden vectors in
/// TickTests/Fixtures/rank_vectors.json pin the two together — change either
/// side only in lockstep.
enum Rank {
    static let alphabet: [UInt8] = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz".utf8)
    /// Rebalance threshold: past this length a project re-keys all its steps.
    static let maxLength = 24

    private static let zero = UInt8(ascii: "0")
    private static let zLower = UInt8(ascii: "z")

    private static func idx(_ c: UInt8) -> Int {
        alphabet.firstIndex(of: c) ?? -1
    }

    /// Reads s as if right-padded with '0' — how a shorter rank compares
    /// against a longer one sharing its prefix.
    private static func digitAt(_ s: [UInt8], _ i: Int) -> UInt8 {
        i < s.count ? s[i] : zero
    }

    private static func sliceFrom(_ s: [UInt8], _ n: Int) -> [UInt8] {
        n < s.count ? Array(s[n...]) : []
    }

    private static func lexLess(_ a: [UInt8], _ b: [UInt8]) -> Bool {
        for i in 0..<min(a.count, b.count) {
            if a[i] != b[i] { return a[i] < b[i] }
        }
        return a.count < b.count
    }

    /// A key strictly between a and b ("" = open end). Degrades to after(a)
    /// on a violated precondition, same as the Go side.
    static func mid(_ a: String, _ b: String) -> String {
        String(decoding: midBytes(Array(a.utf8), Array(b.utf8)), as: UTF8.self)
    }

    private static func midBytes(_ a: [UInt8], _ b: [UInt8]) -> [UInt8] {
        if !b.isEmpty && !lexLess(a, b) {
            return afterBytes(a)
        }
        if !b.isEmpty {
            var n = 0
            while n < b.count && digitAt(a, n) == b[n] { n += 1 }
            if n > 0 {
                return Array(b[0..<n]) + midBytes(sliceFrom(a, n), sliceFrom(b, n))
            }
        }
        let da = a.isEmpty ? 0 : idx(a[0])
        let db = b.isEmpty ? alphabet.count : idx(b[0])
        if db - da > 1 {
            return [alphabet[(da + db) / 2]]
        }
        if b.count > 1 {
            return [b[0]]
        }
        return [alphabet[da]] + midBytes(sliceFrom(a, 1), [])
    }

    /// A key after last ("" = empty list).
    static func after(_ last: String) -> String {
        String(decoding: afterBytes(Array(last.utf8)), as: UTF8.self)
    }

    private static func afterBytes(_ last: [UInt8]) -> [UInt8] {
        if last.isEmpty { return [UInt8(ascii: "V")] }
        let c = last[last.count - 1]
        if c != zLower {
            return Array(last[0..<(last.count - 1)]) + [alphabet[idx(c) + 1]]
        }
        return last + [UInt8(ascii: "V")]
    }

    /// A key before first ("" = empty list).
    static func before(_ first: String) -> String {
        let f = Array(first.utf8)
        if f.isEmpty { return "V" }
        let i = idx(f[0])
        if i >= 2 { // stop at 2 so we never mint the invalid single "0"
            return String(decoding: [alphabet[i - 1]], as: UTF8.self)
        }
        return mid("", first)
    }

    /// Evenly spread keys for bulk assignment (i in 0..<n), index-ordered.
    static func initial(_ i: Int, _ n: Int) -> String {
        var w = 2
        var span = 62 * 62
        while span / (n + 1) < 2 {
            w += 1
            span *= 62
        }
        var v = (i + 1) * (span) / (n + 1)
        var digits = [UInt8](repeating: zero, count: w)
        for p in stride(from: w - 1, through: 0, by: -1) {
            digits[p] = alphabet[v % 62]
            v /= 62
        }
        while digits.count > 1 && digits[digits.count - 1] == zero {
            digits.removeLast()
        }
        return String(decoding: digits, as: UTF8.self)
    }

    /// Well-formed: non-empty, alphabet-only, not ending in the zero digit.
    static func isValid(_ r: String) -> Bool {
        let b = Array(r.utf8)
        if b.isEmpty || b[b.count - 1] == zero { return false }
        return b.allSatisfy { idx($0) >= 0 }
    }
}
