package textfilter

import "testing"

func TestDefaultParses(t *testing.T) {
	f, err := Parse(termsTxt)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.native) == 0 || len(f.ascii) == 0 || len(f.allow) == 0 {
		t.Fatalf("terms.txt: native=%d ascii=%d allow=%d", len(f.native), len(f.ascii), len(f.allow))
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{"slur", "nope word", "slur 死ね*", "slur ・"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q): want error", src)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"ＡＢＣ　ｄｅｆ":       "abc def",
		"ｶﾀｶﾅ":          "カタカナ",
		"ひらがな":          "ヒラガナ",
		"a\u200bb":      "ab",
		"Hello,  World": "hello world",
		"x・y・z":         "xyz",
		"今日 は 晴れ":       "今日 ハ 晴レ",
		"":              "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckRefuses(t *testing.T) {
	for in, want := range map[string]Category{
		"死ね":            Threat,
		"お前を殺してやる":      Threat,
		"ころしてやる":        Threat,
		"コロシテヤル":        Threat,
		"ｺﾛｼﾃﾔﾙ":        Threat,
		"kill yourself": Threat,
		"just KYS":      Threat,
		"キチガイ":          Slur,
		"そんなの porn だよ":  Sexual,
		"これはpornです":     Sexual,
		"Ｆｕｃｋ":          Sexual,
		"fucking":       Sexual,
		"f u c k":       Sexual,
		"死\u200bね":      Threat,
		"殺・し・て・や・る":     Threat,
		"せっくす":          Sexual,
	} {
		got, ok := Check(in)
		if !ok || got != want {
			t.Errorf("Check(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestCheckAllowsCommonText(t *testing.T) {
	for _, in := range []string{
		"",
		"今日の夕飯はカレー",
		"パチンコ店の前で待ち合わせ",
		"ヒューマンコミュニケーション",
		"ウーマンコミュニティ",
		"ロマンコメディを観た",
		"ユニセックスのTシャツ",
		"グレイプフルーツ",
		"ガイジン墓地",
		"まだ死ねない",
		"シネマに行く",
		"ところすき",
		"skill yourself up",
		"grape and rapeseed",
		"Scunthorpe United",
		"a cocktail party",
		"Middlesex",
		"skyscraper",
		"kill you in chess",
	} {
		if cat, ok := Check(in); ok {
			t.Errorf("Check(%q) = %q; want allowed", in, cat)
		}
	}
}
