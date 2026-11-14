package pii

import (
	"strings"
	"testing"
)

// All values below are made up (#367: tests use invented values only).
func TestMask(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind Kind
		gone string // must not survive
	}{
		{"email", "連絡は taro.example@example.com まで", Email, "example.com"},
		{"mobile hyphen", "電話 090-1234-5678 にください", Phone, "5678"},
		{"mobile plain", "電話は09012345678です", Phone, "12345678"},
		{"full-width digits", "電話は０９０ー１２３４ー５６７８", Phone, "5678"},
		{"international", "call +81 90 1234 5678", Phone, "5678"},
		{"landline", "03-1234-5678 の店", Phone, "5678"},
		{"url", "ここ https://example.com/a?b=1 が落ちる", URL, "example.com"},
		{"www", "www.example.org を見て", URL, "example.org"},
		{"bearer", "Authorization: Bearer abc.def-ghi", Secret, "abc.def"},
		{"jwt", "token eyJhbGciOi.eyJzdWIiOi.c2lnbmF0dXJl", Secret, "eyJ"},
		{"api key shape", "key sk-ant-abcdefghijklmnop", Secret, "abcdefghijklmnop"},
		{"long hex", "id 0123456789abcdef0123456789abcdef", Secret, "0123456789abcdef"},
		{"feedback id", "fb_01HZXAMPLE12345 の件", ID, "01HZXAMPLE"},
		{"uid shape", "uid aB3dE5gH7jK9mN1pQ3sT5vX7yZ12 です", ID, "aB3dE5gH7"},
		{"card", "カード 4111 1111 1111 1111 で払った", Number, "1111 1111"},
		{"my number", "番号 123456789012", Number, "123456789012"},
		{"postal", "〒920-0000 に住む", Number, "920-0000"},
		{"address", "石川県金沢市広坂1-1 に住んでいます", Address, "広坂"},
		{"latlng", "36.5613, 136.6562 で", Place, "136.6562"},
		{"ip", "192.168.10.20 から", Place, "192.168"},
		{"mention", "@someone_dev に聞いた", Name, "someone_dev"},
		{"honorific kanji", "田中さんの記録が消えた", Name, "田中"},
		{"honorific katakana", "タナカ様へ", Name, "タナカ"},
		{"english name", "Mr. Smith said", Name, "Smith"},
		{"yen", "家賃 85,000円 を入れた", Money, "85,000"},
		{"yen sign", "¥1,200 の買い物", Money, "1,200"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Mask(c.in)
			if got.Counts[c.kind] == 0 {
				t.Fatalf("want %s in %q, got %q %v", c.kind, c.in, got.Text, got.Counts)
			}
			if strings.Contains(got.Text, c.gone) {
				t.Fatalf("%q survived: %q", c.gone, got.Text)
			}
			if !strings.Contains(got.Text, string(c.kind)) {
				t.Fatalf("placeholder missing: %q", got.Text)
			}
			if k := Find(got.Text); len(k) != 0 {
				t.Fatalf("masked text still has %v: %q", k, got.Text)
			}
		})
	}
}

func TestMaskKeepsOrdinaryText(t *testing.T) {
	for _, in := range []string{
		"写真を追加すると落ちる",
		"一覧の並べ替えが効かない。iOS 26 で起きる",
		"版 1.4.2 (build 120) で、ボタンを 3 回押すと止まる",
		"I love this app, thanks!",
	} {
		got := Mask(in)
		if got.Total() != 0 {
			t.Errorf("%q masked as %v: %q", in, got.Counts, got.Text)
		}
	}
}

func TestMaskIsIdempotent(t *testing.T) {
	once := Mask("連絡は a@example.com と 090-1234-5678、田中さん").Text
	twice := Mask(once)
	if twice.Text != once || twice.Total() != 0 {
		t.Fatalf("second pass changed: %q -> %q %v", once, twice.Text, twice.Counts)
	}
}

func TestRatio(t *testing.T) {
	if r := Mask("a@example.com").Ratio; r < 0.99 {
		t.Fatalf("all-PII ratio = %v", r)
	}
	if r := Mask("落ちる").Ratio; r != 0 {
		t.Fatalf("clean ratio = %v", r)
	}
	if r := Mask("").Ratio; r != 0 {
		t.Fatalf("empty ratio = %v", r)
	}
}

func TestCleanDropsInvisible(t *testing.T) {
	in := "a\u200bb\u202ec\x07d\ne"
	if !HasInvisible(in) {
		t.Fatal("HasInvisible missed")
	}
	if got := Clean(in); got != "abcd\ne" {
		t.Fatalf("Clean = %q", got)
	}
	if HasInvisible("plain\ttext\n") {
		t.Fatal("newline/tab are not invisible")
	}
}
