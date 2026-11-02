package expreval

import "unicode"

// graphemeCount approximates Swift's String.count (extended grapheme
// clusters) without a Unicode segmentation table: combining and spacing marks,
// ZWJ sequences, variation selectors, emoji skin tones, tag characters,
// regional-indicator pairs (flags), CR LF and conjoining Hangul jamo (V/T)
// join the previous character. Other rare clusters (Indic conjuncts,
// prepended characters) may count differently from iOS.
func graphemeCount(s string) int {
	n := 0
	prev := rune(-1)
	joinNext := false
	riOdd := false
	for _, r := range s {
		isRI := r >= 0x1F1E6 && r <= 0x1F1FF
		switch {
		case n == 0:
			n++
		case joinNext:
		case prev == '\r' && r == '\n':
		case extends(r):
		case isRI && riOdd:
		default:
			n++
		}
		joinNext = r == 0x200D
		if isRI {
			riOdd = !riOdd
		} else {
			riOdd = false
		}
		prev = r
	}
	return n
}

func extends(r rune) bool {
	switch {
	case r == 0x200D, r == 0x200C:
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF:
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF:
		return true
	case r >= 0xE0020 && r <= 0xE007F:
		return true
	case r >= 0x1160 && r <= 0x11FF, r >= 0xD7B0 && r <= 0xD7FF:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
}
