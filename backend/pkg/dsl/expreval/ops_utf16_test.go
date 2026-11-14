package expreval

import "testing"

func TestCompareUTF16Table(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "abc", "abc", 0},
		{"both empty", "", "", 0},
		{"less", "abc", "abd", -1},
		{"greater", "b", "a", 1},
		{"prefix is less", "ab", "abc", -1},
		{"longer is greater", "abc", "ab", 1},
		{"empty is less", "", "a", -1},
		// U+1F600 is D83D DE00 in UTF-16: below U+FF5E (FF5E) though above it as a rune
		{"surrogate pair sorts before BMP high", "\U0001F600", "～", -1},
		{"BMP high sorts after surrogate pair", "～", "\U0001F600", 1},
		{"pair vs pair low unit", "\U0001F600", "\U0001F601", -1},
		{"pair vs pair high unit", "\U0001F910", "\U0001F600", 1},
		{"same pair", "a\U0001F600", "a\U0001F600", 0},
		{"pair then more", "\U0001F600", "\U0001F600a", -1},
		{"japanese", "あ", "い", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareUTF16(tc.a, tc.b); got != tc.want {
				t.Errorf("compareUTF16(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
