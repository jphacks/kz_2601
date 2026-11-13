package feeds

import (
	"io"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
)

func encode2593(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}

func rss2593(charset string) string {
	return `<?xml version="1.0" encoding="` + charset + `"?>` +
		`<rss version="2.0"><channel><title>お知らせ</title>` +
		`<item><title>図書館の休館日のお知らせ</title><link>https://example.com/a</link></item>` +
		`</channel></rss>`
}

func TestParseRSSCharsets2593(t *testing.T) {
	cases := []struct {
		label string
		enc   encoding.Encoding
	}{
		{"Shift_JIS", japanese.ShiftJIS},
		{"windows-31j", japanese.ShiftJIS},
		{"EUC-JP", japanese.EUCJP},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			items, err := parseRSS(encode2593(t, c.enc, rss2593(c.label)))
			if err != nil {
				t.Fatalf("parseRSS: %v", err)
			}
			if len(items) != 1 || items[0].Title != "図書館の休館日のお知らせ" || items[0].URL != "https://example.com/a" {
				t.Errorf("items = %+v", items)
			}
		})
	}
}

func TestParseRSSUnknownCharset2593(t *testing.T) {
	_, err := parseRSS([]byte(rss2593("ISO-2022-JP")))
	if err == nil || !strings.Contains(err.Error(), "unsupported charset") {
		t.Errorf("err = %v, want unsupported charset", err)
	}
}

func TestCharsetReader2593(t *testing.T) {
	for _, label := range []string{"", "utf-8", " UTF8 "} {
		in := strings.NewReader("あ")
		r, err := charsetReader(label, in)
		if err != nil {
			t.Fatalf("charsetReader(%q): %v", label, err)
		}
		if r != io.Reader(in) {
			t.Errorf("charsetReader(%q) wrapped a UTF-8 reader", label)
		}
	}
	for _, label := range []string{"sjis", "shift-jis", "cp932"} {
		r, err := charsetReader(label, strings.NewReader(string(encode2593(t, japanese.ShiftJIS, "日本"))))
		if err != nil {
			t.Fatalf("charsetReader(%q): %v", label, err)
		}
		got, err := io.ReadAll(r)
		if err != nil || string(got) != "日本" {
			t.Errorf("charsetReader(%q) read %q, %v", label, got, err)
		}
	}
	if _, err := charsetReader("latin-9", strings.NewReader("")); err == nil {
		t.Error("unknown charset: want error")
	}
}
