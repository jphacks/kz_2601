package dsl

import (
	"strings"
	"testing"
)

// #507: profile fields that reach or identify a person get a lint warning
// (the AI regenerates without them); ordinary hobby fields do not.
func TestProfileSensitiveField(t *testing.T) {
	cases := []struct {
		id, name string
		typ      FieldType
		want     string // "" = no warning
	}{
		// ja
		{"phone", "電話番号", FieldString, "a phone number"},
		{"contact_mail", "メールアドレス", FieldString, "an email address"},
		{"renraku", "連絡先", FieldText, "contact info"},
		{"line", "LINE ID", FieldString, "a LINE ID or SNS account"},
		{"insta", "インスタのアカウント", FieldString, "a LINE ID or SNS account"},
		{"jusho", "住所", FieldString, "an address"},
		{"nenrei", "年齢", FieldInteger, "an age or birthday"},
		{"nendai", "年齢", FieldEnum, "an age or birthday"},
		{"bday", "生年月日", FieldDate, "an age or birthday"},
		{"gakko", "学校名", FieldString, "a school"},
		{"honmyo", "本名", FieldString, "a real name"},
		{"seibetsu", "性別", FieldEnum, "a gender"},
		// en (id or name)
		{"phoneNumber", "Phone", FieldString, "a phone number"},
		{"tel", "Tel", FieldString, "a phone number"},
		{"eMail", "E-mail", FieldString, "an email address"},
		{"lineId", "LINE", FieldString, "a LINE ID or SNS account"},
		{"twitter_handle", "Twitter", FieldString, "a LINE ID or SNS account"},
		{"home_address", "Address", FieldString, "an address"},
		{"age", "Age", FieldInteger, "an age or birthday"},
		{"birthday", "Birthday", FieldDate, "an age or birthday"},
		{"school", "School", FieldString, "a school"},
		{"fullName", "Full name", FieldString, "a real name"},
		{"gender", "Gender", FieldEnum, "a gender"},
		// fine
		{"intro", "自己紹介", FieldText, ""},
		{"hobby", "好きなゲーム", FieldString, ""},
		{"area", "地域", FieldEnum, ""},
		{"online", "オンライン参加", FieldBoolean, ""},
		{"place", "オンライン or 対面", FieldString, ""},
		{"pref_age", "年齢層の好み", FieldEnum, ""},
		{"age_range", "Preferred age range", FieldEnum, ""},
		{"nickname", "ニックネーム", FieldString, ""},
		{"name", "Name", FieldString, ""},
		{"how", "連絡方法", FieldEnum, ""},
		{"contact_method", "Contact method", FieldEnum, ""},
		{"deadline", "Deadline", FieldDate, ""},
		{"hotel", "Hotel", FieldString, ""},
		{"page", "Page", FieldInteger, ""},
		{"message", "Message", FieldText, ""},
		{"skill", "Skill level", FieldEnum, ""},
		{"over18", "18歳以上", FieldBoolean, ""},
	}
	for _, c := range cases {
		f := Field{ID: c.id, Name: c.name, Type: c.typ}
		if got := ProfileSensitiveField(&f); got != c.want {
			t.Errorf("%s / %s: got %q, want %q", c.id, c.name, got, c.want)
		}
	}
	hidden := Field{ID: "phone", Name: "電話番号", Type: FieldString, Hidden: true}
	if got := ProfileSensitiveField(&hidden); got != "a phone number" {
		t.Errorf("hidden field (display-only, still on every device) must warn: got %q", got)
	}
}

func TestMatchProfileLint(t *testing.T) {
	a := matchingApp(t)
	for _, w := range a.Lint() {
		if strings.Contains(w, "strangers") {
			t.Errorf("testdata must be clean: %s", w)
		}
	}
	a.Entities[0].Fields = append(a.Entities[0].Fields,
		Field{ID: "phone", Name: "電話番号", Type: FieldString},
		Field{ID: "age", Name: "年齢", Type: FieldInteger})
	a.Entities[1].Fields = append(a.Entities[1].Fields, Field{ID: "line", Name: "LINE ID", Type: FieldString})
	got := a.Lint()
	wantLint(t, got, `entity "profile" field "phone" (電話番号) looks like a phone number`)
	wantLint(t, got, `put it in the match "matched" entity`)
	wantLint(t, got, `entity "profile" field "age" (年齢) looks like an age or birthday`)
	for _, w := range got {
		if strings.Contains(w, `field "line"`) {
			t.Errorf("the matched entity may hold contact info: %s", w)
		}
	}
}
