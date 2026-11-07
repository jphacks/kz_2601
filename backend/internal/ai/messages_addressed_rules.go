package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addressedRules (#271): messages to one person or a few chosen people that
// only the sender and the recipients read (entity.addressedTo + a member /
// members field). Before this, "1 対 1 / DM" became a chatThread everybody
// reads (chat_thread_rules.go, which this block overrides when recipients
// are chosen). Appended by init() so prompt.go / chat_first.go stay
// untouched; restrict.go drops it for builds without FeatureAddressed.
var addressedRules = strings.TrimSpace(`
## 宛先を選んで送るメッセージ(1 対 1・グループ宛て・#271)
- 「1 対 1 のメッセージ」「DM」「個別に連絡」「先生にだけ送る」「選んだ人にだけ送る」「何人かに送る」は宛先付きの記録: entity に "scope":"shared" と "addressedTo":"to"、field は to(name 宛先・1 人なら type "member"・何人かなら type "members"・required)と本文(text・required)。**読めるのは送った人と宛先の人だけ**(サーバーが守る。作成者でも宛先でなければ見えない)。上の chatThread の「1 対 1 / DM」より優先(chatThread では宛先を選べない)。宛先の名前・送信者の名前・送信日時の field は作らない($author / $createdAt)。
- 画面: 受信箱 = list(filter {"field":"$author","op":"neq","value":{"me":true}}・読めるのは自分宛てだけなので、これが自分宛て)・item は title 本文・subtitle $author。送信済み = list(op eq)・subtitle to。送る画面 = screen.entity のフォーム(picker で to・textEditor で本文・toolbar の saveRecord)を toolbar の navigate(sheet・「送る」・icon paperplane)で開く。
- 未読: entity に "marks":[{"id":"read","icon":"eye","label":"既読"}]、受信箱の item に "mark":"read"(押すと既読)。「未読だけ」は filter {"field":"$marks.read","op":"neq","value":{"me":true}}、未読の数は stat の count に同じ filter。
- 宛先は送った後で変えられない(edit は書かない・edit "members" は不可)。返信は同じ entity に、元の送り手を宛先にした新しい記録。unique を書くなら "$author" を含める。
- serverRules で宛先付きの entity をきっかけにする時は、通知は to.kind "author" か "recipients"(宛先の人に届く・field は addressedTo の field)だけ・書き換えはその記録だけ(他の記録へのコピー・数える・宛先の書き換えはできない)。宛先付きの entity は on.each・agg・ref 先の読み取りに使えない。
- 参加者みんなで話すチャット(全員が読む)は今までどおり chatThread。
`)

// addressedChatRule: the same capability for chat answers (できる?).
var addressedChatRule = strings.TrimSpace(`
## 宛先を選んで送るメッセージ(#271)
- 「1 対 1 のメッセージ」「DM」「選んだ人にだけ送る」は**できる**(参加した人の中から宛先を選び、送った人と宛先の人だけが読める。受信箱・送信済み・未読の印も作れる)。宛先の人へのプッシュ通知もできる。
`)

// addressedNote is the restrictionNote line for builds without FeatureAddressed.
const addressedNote = "- entity の addressedTo と type members の field は使わない(この版では宛先を選べない)。宛先の人だけが読むメッセージを頼まれたら、参加者みんなが読む chatThread で作り、summary に「宛先の人だけが読めるメッセージはこの版では使えません。最新版に更新すると使えます」と添える。\n"

// FeatureAddressed (#271) is a supportedFeatures value: the iOS build picks
// several participants for a members field. Older builds show it as text
// (the server refuses it), so they are not given addressed messages.
const FeatureAddressed = "addressedMessages"

func init() {
	systemPrompt += "\n\n" + addressedRules
	with := chatCapabilities + "\n\n" + addressedChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
}

// dropAddressedRules gives builds without FeatureAddressed the prompts from before #271.
func dropAddressedRules(s string) string {
	s = strings.Replace(s, "\n\n"+addressedRules, "", 1)
	return strings.Replace(s, "\n\n"+addressedChatRule, "", 1)
}

// restrictAddressed drops addressedTo and type members from the schema defs.
func restrictAddressed(defs map[string]any) {
	if entity, _ := defs["Entity"].(map[string]any); entity != nil {
		props, _ := entity["properties"].(map[string]any)
		delete(props, "addressedTo")
	}
	if field, _ := defs["Field"].(map[string]any); field != nil {
		props, _ := field["properties"].(map[string]any)
		if tp, _ := props["type"].(map[string]any); tp != nil {
			if en, ok := tp["enum"].([]any); ok {
				kept := make([]any, 0, len(en))
				for _, v := range en {
					if v != string(dsl.FieldMembers) {
						kept = append(kept, v)
					}
				}
				tp["enum"] = kept
			}
		}
	}
}

// unaddress removes what an older build cannot use from a generated
// document: addressedTo goes and a members field becomes string (when
// noAddressed). A picker left on a string field (members here, member in
// unrole) becomes a textField so the document stays valid. It returns how
// many places changed.
func unaddress(root map[string]any, noAddressed bool) int {
	n := 0
	stringFields := map[string]map[string]bool{} // entity -> string field ids
	ents, _ := root["entities"].([]any)
	for _, e := range ents {
		em, _ := e.(map[string]any)
		if em == nil {
			continue
		}
		id, _ := em["id"].(string)
		if _, has := em["addressedTo"]; has && noAddressed {
			delete(em, "addressedTo")
			n++
		}
		fs, _ := em["fields"].([]any)
		for _, f := range fs {
			fm, _ := f.(map[string]any)
			if fm == nil {
				continue
			}
			if fm["type"] == string(dsl.FieldMembers) && noAddressed {
				fm["type"] = string(dsl.FieldString)
				n++
			}
			if fm["type"] == string(dsl.FieldString) {
				if stringFields[id] == nil {
					stringFields[id] = map[string]bool{}
				}
				fid, _ := fm["id"].(string)
				stringFields[id][fid] = true
			}
		}
	}
	var walk func(entity string, cs []any)
	walk = func(entity string, cs []any) {
		for _, v := range cs {
			c, _ := v.(map[string]any)
			if c == nil {
				continue
			}
			if f, _ := c["field"].(string); c["type"] == string(dsl.CompPicker) && stringFields[entity][f] {
				c["type"] = string(dsl.CompTextField)
				n++
			}
			if ch, ok := c["children"].([]any); ok {
				walk(entity, ch)
			}
		}
	}
	screens, _ := root["screens"].([]any)
	for _, s := range screens {
		if sc, _ := s.(map[string]any); sc != nil {
			entity, _ := sc["entity"].(string)
			cs, _ := sc["components"].([]any)
			walk(entity, cs)
		}
	}
	if noAddressed {
		n += unrecipient(root)
	}
	return n
}

// unrecipient: without addressedTo, a rule's notify to the recipients goes
// to the record's author instead (kind recipients needs an addressed entity).
func unrecipient(root map[string]any) int {
	n := 0
	rules, _ := root["serverRules"].([]any)
	for _, r := range rules {
		rm, _ := r.(map[string]any)
		do, _ := rm["do"].([]any)
		for _, a := range do {
			am, _ := a.(map[string]any)
			nm, _ := am["notify"].(map[string]any)
			if to, _ := nm["to"].(map[string]any); to != nil && to["kind"] == dsl.RecipientRecipients {
				nm["to"] = map[string]any{"kind": dsl.RecipientAuthor}
				n++
			}
		}
	}
	return n
}
