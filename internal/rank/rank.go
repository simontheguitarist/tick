// Package rank implements the fractional-index keys that order steps. A rank is
// a short string over a base-62 alphabet whose byte order IS the step order, so
// clients sort with a plain string compare and moving one step re-keys only
// that step. The same algorithm is ported byte-for-byte to Swift
// (ios/Shared/Models/Rank.swift); docs/protocol.md carries shared golden
// vectors that both test suites assert against.
package rank

import "strings"

// alphabet is ASCII-ordered so lexicographic byte compare equals rank compare.
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Ranks never end in '0': "A0" sorts between "A" and "A1" but is equivalent to
// "A" as a midpoint anchor, so trailing zeros would only waste length.

func idx(c byte) int { return strings.IndexByte(alphabet, c) }

// digitAt reads s as if right-padded with '0' (the zero digit), which is how a
// shorter rank compares against a longer one sharing its prefix.
func digitAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return '0'
}

func sliceFrom(s string, n int) string {
	if n < len(s) {
		return s[n:]
	}
	return ""
}

// Mid returns a key strictly between a and b. "" as a means "before every
// key"; "" as b means "after every key". Callers must pass a < b (with ""
// interpreted as above); on a violated precondition it degrades to After(a)
// so a bug never mints an invalid or colliding rank.
func Mid(a, b string) string {
	if b != "" && a >= b {
		return After(a)
	}
	if b != "" {
		// Skip the shared prefix (with a right-padded by '0') and recurse on
		// the first differing digit. This also guarantees the base case below
		// never sees b starting at digit '0'.
		n := 0
		for n < len(b) && digitAt(a, n) == b[n] {
			n++
		}
		if n > 0 {
			return b[:n] + Mid(sliceFrom(a, n), b[n:])
		}
	}
	da := 0
	if a != "" {
		da = idx(a[0])
	}
	db := len(alphabet)
	if b != "" {
		db = idx(b[0])
	}
	if db-da > 1 {
		return string(alphabet[(da+db)/2])
	}
	// Consecutive first digits: reuse b's head if it has room, else extend a.
	if len(b) > 1 {
		return b[:1]
	}
	return string(alphabet[da]) + Mid(sliceFrom(a, 1), "")
}

// After returns a key after last ("" = the list is empty). Incrementing the
// final digit instead of midpointing keeps repeated appends at one char per
// ~30 steps.
func After(last string) string {
	if last == "" {
		return "V"
	}
	if c := last[len(last)-1]; c != 'z' {
		return last[:len(last)-1] + string(alphabet[idx(c)+1])
	}
	return last + "V"
}

// Before returns a key before first ("" = the list is empty).
func Before(first string) string {
	if first == "" {
		return "V"
	}
	// Stop at index 2 so we never mint the invalid single "0".
	if i := idx(first[0]); i >= 2 {
		return string(alphabet[i-1])
	}
	return Mid("", first)
}

// Initial spreads n keys evenly for bulk assignment (migration, rebalance),
// preserving index order: Initial(0,n) < Initial(1,n) < … < Initial(n-1,n).
func Initial(i, n int) string {
	w, span := 2, 62*62
	for span/(n+1) < 2 { // keep at least a gap of 2 between neighbours
		w++
		span *= 62
	}
	v := int(int64(i+1) * int64(span) / int64(n+1))
	digits := make([]byte, w)
	for p := w - 1; p >= 0; p-- {
		digits[p] = alphabet[v%62]
		v /= 62
	}
	out := string(digits)
	for len(out) > 1 && out[len(out)-1] == '0' {
		out = out[:len(out)-1]
	}
	return out
}

// IsValid reports whether r is a well-formed rank: non-empty, alphabet-only,
// and not ending in the zero digit.
func IsValid(r string) bool {
	if r == "" || r[len(r)-1] == '0' {
		return false
	}
	for i := 0; i < len(r); i++ {
		if idx(r[i]) < 0 {
			return false
		}
	}
	return true
}

// MaxLen is the rebalance threshold: when any rank in a project grows past
// this, the whole project is re-keyed with Initial.
const MaxLen = 24
