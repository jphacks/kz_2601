package books

import "strings"

// NormalizeISBN accepts an ISBN-10 or ISBN-13 (hyphens / spaces allowed,
// e.g. what a barcode or a person types) and returns the ISBN-13 digits.
// The checksum must hold and an ISBN-13 must start with 978 / 979 (the
// "Bookland" prefixes), so a random EAN or a typo never reaches the source.
func NormalizeISBN(raw string) (string, bool) {
	if len(raw) > 40 {
		return "", false
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'x' || r == 'X':
			b.WriteByte('X')
		case r == '-' || r == ' ' || r == ' ':
		default:
			return "", false
		}
	}
	s := b.String()
	switch len(s) {
	case 10:
		if !validISBN10(s) {
			return "", false
		}
		return isbn13("978" + s[:9]), true
	case 13:
		if strings.ContainsRune(s, 'X') || (!strings.HasPrefix(s, "978") && !strings.HasPrefix(s, "979")) {
			return "", false
		}
		if isbn13(s[:12]) != s {
			return "", false
		}
		return s, true
	}
	return "", false
}

// validISBN10: weights 10..1, X (= 10) only in the last place, sum % 11 == 0.
func validISBN10(s string) bool {
	sum := 0
	for i := range 10 {
		c := s[i]
		v := int(c - '0')
		if c == 'X' {
			if i != 9 {
				return false
			}
			v = 10
		}
		sum += v * (10 - i)
	}
	return sum%11 == 0
}

// isbn13 appends the EAN-13 check digit to 12 digits.
func isbn13(twelve string) string {
	sum := 0
	for i := range 12 {
		v := int(twelve[i] - '0')
		if i%2 == 1 {
			v *= 3
		}
		sum += v
	}
	return twelve + string(rune('0'+(10-sum%10)%10))
}
