package ai

import "testing"

func TestWithPrivateNote2574(t *testing.T) {
	const (
		jaNote = "共有の設定は変えていないため、記録は自分だけのままです。"
		enNote = " Sharing wasn't turned on, so the records stay private to you."
	)
	cases := []struct {
		name    string
		summary string
		loc     Locale
		want    string
	}{
		{"ja with period", "家計簿を作りました。", LocaleJA, "家計簿を作りました。" + jaNote},
		{"ja without period", "家計簿を作りました", LocaleJA, "家計簿を作りました" + jaNote},
		{"ja trims spaces", "  家計簿を作りました。 ", LocaleJA, "家計簿を作りました。" + jaNote},
		{"ja empty", "", LocaleJA, jaNote},
		{"ja mentions sharing", "共有はしていません。", LocaleJA, "共有はしていません。"},
		{"ja mentions private", "記録は自分だけです。", LocaleJA, "記録は自分だけです。"},
		{"unknown locale is ja", "家計簿を作りました。", Locale("fr"), "家計簿を作りました。" + jaNote},
		{"en with period", "Made a budget app.", LocaleEN, "Made a budget app." + enNote},
		{"en without period", "Made a budget app", LocaleEN, "Made a budget app" + enNote},
		{"en trims spaces", "  Made a budget app. ", LocaleEN, "Made a budget app." + enNote},
		{"en empty", "", LocaleEN, enNote[1:]},
		{"en mentions sharing", "Made it Shareable.", LocaleEN, "Made it Shareable."},
		{"en mentions private", "Records are PRIVATE.", LocaleEN, "Records are PRIVATE."},
		{"en japanese text", "家計簿", LocaleEN, "家計簿" + enNote},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withPrivateNote(c.summary, c.loc); got != c.want {
				t.Fatalf("withPrivateNote(%q, %q) = %q, want %q", c.summary, c.loc, got, c.want)
			}
		})
	}
}

func TestAppendSentence2574(t *testing.T) {
	cases := []struct {
		name       string
		summary, s string
		loc        Locale
		want       string
	}{
		{"ja with period", "作りました。", "通知も入れました。", LocaleJA, "作りました。通知も入れました。"},
		{"ja without period", "作りました", "通知も入れました。", LocaleJA, "作りました通知も入れました。"},
		{"ja trims both", " 作りました。 ", " 通知も入れました。 ", LocaleJA, "作りました。通知も入れました。"},
		{"empty sentence keeps summary as is", " 作りました。 ", "  ", LocaleJA, " 作りました。 "},
		{"empty summary", "", "通知も入れました。", LocaleJA, "通知も入れました。"},
		{"blank summary", "   ", " 通知も入れました。", LocaleEN, "通知も入れました。"},
		{"both empty", "", "", LocaleEN, ""},
		{"en with period", "Made it.", "Added a reminder.", LocaleEN, "Made it. Added a reminder."},
		{"en without period", "Made it", "Added a reminder.", LocaleEN, "Made it Added a reminder."},
		{"en trims both", " Made it. ", " Added a reminder. ", LocaleEN, "Made it. Added a reminder."},
		{"unknown locale is ja", "Made it.", "Added.", Locale("fr"), "Made it.Added."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := appendSentence(c.summary, c.s, c.loc); got != c.want {
				t.Fatalf("appendSentence(%q, %q, %q) = %q, want %q", c.summary, c.s, c.loc, got, c.want)
			}
		})
	}
}
