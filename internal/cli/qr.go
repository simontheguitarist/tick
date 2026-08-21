package cli

import (
	"strings"

	"rsc.io/qr"
)

// qrArt renders a QR code with half-block glyphs, two module rows per text
// line. Light modules are drawn as bright blocks and dark modules as blank
// terminal background — on the usual dark terminal that yields a scannable
// dark-on-light code, quiet zone included.
func qrArt(payload string) (string, error) {
	code, err := qr.Encode(payload, qr.L)
	if err != nil {
		return "", err
	}
	const quiet = 2
	size := code.Size
	// black reports the module at x,y with the quiet zone folded in.
	black := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		if x < 0 || y < 0 || x >= size || y >= size {
			return false // quiet zone is light
		}
		return code.Black(x, y)
	}
	total := size + 2*quiet
	var b strings.Builder
	for y := 0; y < total; y += 2 {
		for x := 0; x < total; x++ {
			top, bottom := black(x, y), black(x, y+1)
			if y+1 >= total {
				bottom = true // pad the last odd row with dark (background)
			}
			switch {
			case top && bottom:
				b.WriteRune(' ')
			case top && !bottom:
				b.WriteRune('▄')
			case !top && bottom:
				b.WriteRune('▀')
			default:
				b.WriteRune('█')
			}
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
