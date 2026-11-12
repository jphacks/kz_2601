package expreval

import "testing"

func TestGraphemeExtendsTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    rune
		want bool
	}{
		{"ZWJ", 0x200D, true},
		{"ZWNJ", 0x200C, true},
		{"VS16", 0xFE0F, true},
		{"VS1", 0xFE00, true},
		{"ideographic VS", 0xE0100, true},
		{"ideographic VS last", 0xE01EF, true},
		{"skin tone light", 0x1F3FB, true},
		{"skin tone dark", 0x1F3FF, true},
		{"tag latin", 0xE0067, true},
		{"cancel tag", 0xE007F, true},
		{"hangul jungseong", 0x1161, true},
		{"hangul jongseong ext", 0xD7CB, true},
		{"combining acute (Mn)", 0x0301, true},
		{"enclosing keycap (Me)", 0x20E3, true},
		{"devanagari vowel sign (Mc)", 0x093E, true},
		{"dakuten combining", 0x3099, true},
		{"letter", 'a', false},
		{"hiragana", 'あ', false},
		{"emoji", 0x1F600, false},
		{"regional indicator", 0x1F1EF, false},
		{"just below skin tones", 0x1F3FA, false},
		{"hangul choseong", 0x1100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extends(tc.r); got != tc.want {
				t.Errorf("extends(%U) = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}

func TestGraphemeCountEmojiClusters(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    string
		want int
	}{
		{"ZWJ family", "\U0001F468\u200D\U0001F469\u200D\U0001F467", 1},
		{"skin tone", "\U0001F44D\U0001F3FD", 1},
		{"ZWJ with skin tones", "\U0001F9D1\U0001F3FB\u200D\U0001F91D\u200D\U0001F9D1\U0001F3FF", 1},
		{"flag", "\U0001F1EF\U0001F1F5", 1},
		{"two flags", "\U0001F1EF\U0001F1F5\U0001F1FA\U0001F1F8", 2},
		{"odd regional indicators", "\U0001F1EF\U0001F1F5\U0001F1FA", 2},
		{"tag sequence flag", "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", 1},
		{"keycap", "1\uFE0F\u20E3", 1},
		{"text and emoji", "a\U0001F44D\U0001F3FDb", 3},
		{"CRLF", "a\r\nb", 3},
		{"empty", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := graphemeCount(tc.s); got != tc.want {
				t.Errorf("graphemeCount(%q) = %d, want %d", tc.s, got, tc.want)
			}
		})
	}
}
