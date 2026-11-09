package dsl

import (
	"strings"
	"unicode"
)

// Profile fields that identify or reach a person (#507): strangers browse
// match:"profile" records, so contact details belong in the match:"matched"
// entity and identity details stay out. The DSL has no phone/email/url field
// types, so detection is by keyword in the field id and name. A lint warning
// (not a Validate error) so the AI regenerates without the field.
// Keyword list: docs/backend/features/matching.md §6.

// sensitiveKind is one group of keywords. ja: substrings of the id or name;
// en: whole words of the id or name, split at _ - space and camelCase
// ("lineId" -> line id, "full_name" -> full name).
type sensitiveKind struct {
	what    string
	contact bool // reaches the person: a string/text field; an enum (連絡方法: LINE / メール) is a choice, not the value
	ja      []string
	en      []string
	except  []string // the id or name containing one of these is not this kind (年齢層の好み)
}

var profileSensitiveKinds = []sensitiveKind{
	{what: "a phone number", contact: true, ja: []string{"電話", "携帯番号"}, en: []string{"phone", "tel", "telephone", "phone number"}},
	{what: "an email address", contact: true, ja: []string{"メール"}, en: []string{"email", "e mail", "mail"}},
	{what: "contact info", contact: true, ja: []string{"連絡先"}, en: []string{"contact"}},
	{what: "a LINE ID or SNS account", contact: true, ja: []string{"インスタ", "ツイッター"}, en: []string{"line", "sns", "twitter", "instagram", "insta", "discord", "facebook", "tiktok", "handle"}},
	{what: "an address", contact: true, ja: []string{"住所", "番地", "郵便番号"}, en: []string{"address", "postal", "postcode", "zipcode"}},
	{what: "an age or birthday", ja: []string{"年齢", "生年月日", "誕生日"}, en: []string{"age", "birthday", "birthdate", "birth", "dob"}, except: []string{"年齢層", "age range", "age group"}},
	{what: "a school", ja: []string{"学校", "高校", "大学名"}, en: []string{"school", "university", "college"}},
	{what: "a real name", ja: []string{"本名", "氏名", "実名", "フルネーム", "名字", "苗字"}, en: []string{"full name", "fullname", "real name", "realname", "first name", "last name", "family name", "given name", "surname"}},
	{what: "a gender", ja: []string{"性別"}, en: []string{"gender", "sex"}},
}

// ProfileSensitiveField reports what a field of a match:"profile" entity
// looks like ("a phone number", "an age or birthday", ...) or "" when it is
// fine. Hidden and computed fields and booleans never warn; an enum warns
// only for identity (an age bracket, a gender), not for a contact method.
func ProfileSensitiveField(f *Field) string {
	if k := profileSensitive(f); k != nil {
		return k.what
	}
	return ""
}

func profileSensitive(f *Field) *sensitiveKind {
	if f.Hidden || f.Computed != nil || f.Type == FieldBoolean {
		return nil
	}
	words := " " + strings.Join(append(fieldWords(f.ID), fieldWords(f.Name)...), " ") + " "
	text := strings.ToLower(f.ID + " " + f.Name)
	for i := range profileSensitiveKinds {
		k := &profileSensitiveKinds[i]
		if k.contact && f.Type == FieldEnum {
			continue
		}
		if containsAny(text, k.except) || containsAny(words, k.except) {
			continue
		}
		if containsAny(text, k.ja) {
			return k
		}
		for _, w := range k.en {
			if strings.Contains(words, " "+w+" ") {
				return k
			}
		}
	}
	return nil
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// fieldWords splits an id or name into lower-case ASCII words: "lineId" ->
// [line id], "E-mail" -> [e mail], "LINE ID" -> [line id]. Non-ASCII runs
// separate words (the ja keywords are matched as substrings instead).
func fieldWords(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		// camelCase boundary: lower->Upper, or the last Upper of an acronym before a lower (URLText)
		if unicode.IsUpper(r) && len(cur) > 0 {
			prev := cur[len(cur)-1]
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

func (l *linter) matchProfileFields() {
	e := l.app.ProfileEntity()
	if e == nil {
		return
	}
	for i := range e.Fields {
		f := &e.Fields[i]
		k := profileSensitive(f)
		if k == nil {
			continue
		}
		hint := "remove it from the profile (the display name already shows who wrote it)"
		if k.contact {
			hint = `remove it from the profile and put it in the match "matched" entity, which only matched people read`
		}
		l.add(`entity %q field %q (%s) looks like %s; people browsing match "profile" records are strangers, so %s`, e.ID, f.ID, f.Name, k.what, hint)
	}
}
