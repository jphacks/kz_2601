package ai

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Personal data in public records (#253). share_intent.go (#259 / #215) says
// that new public records are visible to anyone; this file adds the check for
// fields that look like personal data (name, phone, address, email, birthday,
// location): generate appends a sentence and meta.publicPersonalData, chat
// asks once before ready.

// PII kinds, also the values of PublicPersonalData.Kind / ChatResult.ConfirmPublic.
const (
	piiName     = "name"
	piiPhone    = "phone"
	piiAddress  = "address"
	piiEmail    = "email"
	piiBirthday = "birthday"
	piiLocation = "location"
)

// PublicPersonalData is one field that looks like personal data and became
// readable by anyone (public / shared entity) in this generation.
type PublicPersonalData struct {
	Entity     string `json:"entity"`
	EntityName string `json:"entityName"`
	Field      string `json:"field"`
	FieldName  string `json:"fieldName"`
	Kind       string `json:"kind"`
}

type piiRule struct {
	kind  string
	words []string // lower-case substrings of the field id / name
}

// Order matters: email before address (メールアドレス), birthday before name.
var piiFieldRules = []piiRule{
	{piiEmail, []string{"メール", "mail"}},
	{piiPhone, []string{"電話", "携帯", "連絡先", "phone", "mobile"}},
	{piiAddress, []string{"住所", "郵便番号", "番地", "address", "zip", "postal"}},
	{piiBirthday, []string{"生年月日", "誕生日", "birthday", "birth"}},
	{piiLocation, []string{"現在地", "位置情報", "居場所"}},
	{piiName, []string{"氏名", "本名", "フルネーム", "名字", "苗字", "客名", "者名", "員名", "徒名", "fullname", "full_name", "firstname", "first_name", "lastname", "last_name"}},
}

// A plain 「名前」/ name is personal only on a record about people.
var (
	piiGenericName = []string{"名前", "なまえ", "name"}
	piiPersonWords = []string{
		"客", "顧客", "会員", "メンバー", "生徒", "患者", "予約", "家族", "友達", "参加者", "連絡先", "名簿", "利用者", "社員", "スタッフ", "子ども", "子供",
		"customer", "client", "guest", "member", "contact", "person", "people", "student", "patient", "attendee", "reservation", "booking", "staff", "family", "friend",
	}
	// backend-owned ids (share_intent.go): not user data.
	piiSkipIDs = map[string]bool{"author": true, "authorname": true}
)

// Short ids that would match too much as substrings (tel ⊂ hotel).
var piiIDTokens = map[string]string{"tel": piiPhone, "dob": piiBirthday, "zip": piiAddress}

// lastToken is the last word of a snake / camel case id (guestTel → tel).
func lastToken(id string) string {
	start := 0
	for i, r := range id {
		if r == '_' || r == '-' {
			start = i + 1
		} else if r >= 'A' && r <= 'Z' && i > 0 {
			start = i
		}
	}
	return id[start:]
}

// piiKind classifies one field ("" = not personal data).
func piiKind(e *dsl.Entity, f *dsl.Field) string {
	if f.Computed != nil || f.Type == dsl.FieldRef || f.Type == dsl.FieldRefs || piiSkipIDs[strings.ToLower(f.ID)] {
		return ""
	}
	if f.Type == dsl.FieldLocation {
		return piiLocation
	}
	if k := piiIDTokens[strings.ToLower(lastToken(f.ID))]; k != "" {
		return k
	}
	text := strings.ToLower(f.ID + " " + f.Name)
	for _, r := range piiFieldRules {
		if containsAny(text, r.words) {
			return r.kind
		}
	}
	if containsAny(text, piiGenericName) && containsAny(strings.ToLower(e.ID+" "+e.Name+" "+text), piiPersonWords) {
		return piiName
	}
	return ""
}

func publicPII(a *dsl.App) []PublicPersonalData {
	var out []PublicPersonalData
	for i := range a.Entities {
		e := &a.Entities[i]
		if !e.IsShared() {
			continue
		}
		for j := range e.Fields {
			f := &e.Fields[j]
			if k := piiKind(e, f); k != "" {
				out = append(out, PublicPersonalData{Entity: e.ID, EntityName: e.Name, Field: f.ID, FieldName: f.Name, Kind: k})
			}
		}
	}
	return out
}

// newPublicPII lists the personal-data fields that are public in after but
// were not public in before (before nil = create).
func newPublicPII(before, after []byte) []PublicPersonalData {
	a, err := dsl.Parse(after)
	if err != nil {
		return nil
	}
	was := map[string]bool{}
	if len(before) > 0 {
		b, err := dsl.Parse(before)
		if err != nil {
			return nil
		}
		for _, p := range publicPII(b) {
			was[p.Entity+"/"+p.Field] = true
		}
	}
	var out []PublicPersonalData
	for _, p := range publicPII(a) {
		if !was[p.Entity+"/"+p.Field] {
			out = append(out, p)
		}
	}
	return out
}

// notePublicPII sets meta.publicPersonalData and appends the warning to the
// summary. The share note (withShareNote) already says who can see the
// records, so this sentence only names the fields.
func notePublicPII(ar *attemptResult, before []byte, loc Locale) {
	ps := newPublicPII(before, ar.raw)
	if len(ps) == 0 {
		return
	}
	ar.meta.PublicPersonalData = ps
	ar.summary = withPIINote(ar.summary, ps, loc)
}

const piiNoteMaxFields = 4

func withPIINote(summary string, ps []PublicPersonalData, loc Locale) string {
	n := len(ps)
	if n > piiNoteMaxFields {
		ps = ps[:piiNoteMaxFields]
	}
	if loc == LocaleEN {
		names := make([]string, 0, len(ps))
		for _, p := range ps {
			names = append(names, fmt.Sprintf("%q", p.FieldName))
		}
		more := ""
		if n > len(ps) {
			more = " and more"
		}
		return strings.TrimSpace(strings.TrimSpace(summary) + " Personal details such as " + joinEN(names, "and") + more +
			" will be public too, so check that it's OK for others to see them.")
	}
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		names = append(names, "「"+p.FieldName+"」")
	}
	more := ""
	if n > len(ps) {
		more = "など"
	}
	return strings.TrimSpace(summary) + strings.Join(names, "") + more + "のような個人の情報も公開されます。ほかの人に見られてよいか確かめてください。"
}

// ---- chat: ask before ready ----

// piiTextRules read a request / brief (not a field) for personal data.
var piiTextRules = []piiRule{
	{piiEmail, []string{"メールアドレス", "email", "e-mail"}},
	{piiPhone, []string{"電話", "携帯番号", "phone number"}},
	{piiAddress, []string{"住所", "郵便番号", "address"}},
	{piiBirthday, []string{"生年月日", "誕生日", "birthday", "date of birth"}},
	{piiLocation, []string{"現在地", "位置情報", "居場所", "where i am", "my location"}},
	{piiName, []string{"氏名", "本名", "フルネーム", "full name"}},
}

// Names that are not about people.
var piiNotPersonName = []string{"アプリの名前", "アプリ名", "画面の名前", "ボタンの名前", "タブの名前", "app name", "name of the app"}

func piiKindsInText(text string) []string {
	t := strings.ToLower(text)
	for _, w := range piiNotPersonName {
		t = strings.ReplaceAll(t, w, "")
	}
	found := map[string]bool{}
	for _, r := range piiTextRules {
		if containsAny(t, r.words) {
			found[r.kind] = true
		}
	}
	if !found[piiName] && personNameInText(t) {
		found[piiName] = true
	}
	var kinds []string
	for _, k := range piiOrder {
		if found[k] {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

var piiOrder = []string{piiName, piiPhone, piiAddress, piiEmail, piiBirthday, piiLocation}

// personNameInText: a 「名前」/ name right after a word about people
// (「お客さんの名前」 yes, 「ペットの名前」 no).
func personNameInText(t string) bool {
	for _, w := range piiGenericName {
		rest := t
		for {
			i := strings.Index(rest, w)
			if i < 0 {
				break
			}
			before := []rune(rest[:i])
			if len(before) > 6 {
				before = before[len(before)-6:]
			}
			if containsAny(string(before), piiPersonWords) {
				return true
			}
			rest = rest[i+len(w):]
		}
	}
	return containsAny(t, []string{"客名", "者名", "員名", "徒名"})
}

// Markers of the confirmation already being asked (by guardPublicPII or by
// the model following publicPIIChatRule).
var piiAskedMarkers = []string{"公開してよいですか", "個人の情報", "ok to make public", "personal details"}

func piiAlreadyAsked(history []ChatMessage) bool {
	for _, m := range history {
		if m.Role == "assistant" && containsAny(strings.ToLower(m.Text), piiAskedMarkers) {
			return true
		}
	}
	return false
}

// publicPIIKinds returns the personal-data kinds a ready request would put in
// records anyone can see. shared = the current app already has public /
// shared records (edit); otherwise the request has to ask for sharing.
func publicPIIKinds(transcript string, brief *Brief, shared bool) []string {
	text := transcript
	if brief != nil {
		text += "\n" + strings.Join(brief.Entities, "、") + "\n" + strings.Join(brief.Features, "、")
	}
	if !shared {
		in := classifyShareIntent(text)
		if in != shareEveryone && in != shareLimited {
			return nil
		}
	}
	return piiKindsInText(text)
}

// guardPublicPII turns a model-decided ready into one confirmation question
// when the request puts personal data in public records and nobody asked yet.
// Forced / shortcut readies are left alone (the user asked to build now; the
// generate summary and meta.publicPersonalData still warn).
func guardPublicPII(res *ChatResult, history []ChatMessage, shared bool, loc Locale) {
	if res.Kind != "ready" || res.Source != "claude" || piiAlreadyAsked(history) {
		return
	}
	kinds := publicPIIKinds(res.Transcript, res.Brief, shared)
	if len(kinds) == 0 {
		return
	}
	res.Kind = "question"
	res.Text = piiQuestion(kinds, loc)
	res.Suggestions = piiSuggestions(loc)
	res.MultiSelect = false
	res.Brief, res.Transcript = nil, ""
	res.ConfirmPublic = kinds
}

var piiLabels = map[string][2]string{ // ja, en
	piiName:     {"名前", "names"},
	piiPhone:    {"電話番号", "phone numbers"},
	piiAddress:  {"住所", "addresses"},
	piiEmail:    {"メールアドレス", "email addresses"},
	piiBirthday: {"誕生日", "birthdays"},
	piiLocation: {"居場所", "locations"},
}

func piiQuestion(kinds []string, loc Locale) string {
	i := 0
	if loc == LocaleEN {
		i = 1
	}
	labels := make([]string, 0, len(kinds))
	for _, k := range kinds {
		labels = append(labels, piiLabels[k][i])
	}
	if loc == LocaleEN {
		return "Personal details like " + joinEN(labels, "and") + " would go into records anyone who opens the app can see. Is it OK to make public?"
	}
	return strings.Join(labels, "・") + "のような個人の情報が、アプリを開いた人なら誰でも見られる記録に入ります。公開してよいですか?"
}

func piiSuggestions(loc Locale) []string {
	if loc == LocaleEN {
		return []string{"Yes, make it public", "Leave those details out", "Keep it private to me"}
	}
	return []string{"公開してよい", "個人の情報は入れない", "自分だけの記録にする"}
}

// publicPIIChatRule is appended to the chat prompts with shareChatRule.
const publicPIIChatRule = "- 個人の情報の公開(#253): みんなに公開の記録に名前・電話番号・住所・メールアドレス・誕生日・居場所を入れる依頼は、ready にする前に question で「〜のような個人の情報が、アプリを開いた人なら誰でも見られる記録に入ります。公開してよいですか?」と 1 回だけ確かめ、答え(公開してよい / 入れない / 自分だけ)を transcript に入れる。"
