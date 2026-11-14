package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// dataRolesRules (#272 #273): admins of shared data (editableBy / visibleTo /
// deleteBy), participants as a field (type member) and unique records. Before
// this the prompts said roles cannot be built ("店長だけが確定" -> edit
// members); init() swaps those sentences without touching prompt.go /
// chat_first.go. restrict.go drops the member / visibleTo lines for builds
// without FeatureDataRoles and the whole block for builds without shared data.
var dataRolesRules = strings.TrimSpace(`
## 管理者の権限と重複の防止(#272 #273)
- 管理者: shared / public のアプリでは作った人が最初の管理者。管理者は参加者を管理者にできる(共有シートの「参加している人」。DSL には書かない)。
- 「店長だけが確定できる」「承認は管理者だけ」「担当は管理者が決める」: その field(boolean confirmed など)に "editableBy":"admins"。管理者でない人はその項目を入れられず変えられない。管理者は他の人が作ったレコードでもその項目だけは変えられる(entity の edit を members にしなくてよい)。
- 「削除は管理者だけ」: entity に "deleteBy":"admins"。「本人も取り消せる・管理者も消せる」(予約・掲示板)は "authorOrAdmins"。言われなければ書かない(既定は作った人だけ)。
- 管理者だけのボタン: 確定・削除のボタンを管理者にだけ見せる時は button に "visibleTo":"admins"(表示を隠すだけ。守る値には editableBy / deleteBy も必ず付ける)。
- 担当者・当番・割り当てを参加者から選ぶ: type "member" の field(scope shared の entity だけ・picker で選ぶ・一覧には表示名が出る)。名前の手入力欄やメンバーの entity は作らない。投稿・回答した本人は今までどおり $author。
- 重複の防止: 「同じ時間に 2 人予約できないように」「同じ日に 2 件入れない」は entity に "unique":["<その field>"](日付と時刻が別の field なら 2 つ・最大 3 つ)。「1 人 1 回答」「1 人 1 票」は "unique":["$author"](shared / public だけ)、「予定ごとに 1 人 1 回答」は ["event","$author"](event は予定への ref)。サーバーが保存の時に断り、端末は「同じ内容の記録がもうあります」と出す。重複を探すボタン・filter・computed・注意書きの text は作らない。
- private の entity には editableBy / deleteBy / member / "$author" の unique を書かない。
`)

// dataRolesChatRule: the same capability for chat answers (できる?).
var dataRolesChatRule = strings.TrimSpace(`
## 管理者と重複の防止(#272 #273)
- 「店長だけが確定できる」「削除は管理者だけ」「確定ボタンを管理者だけに見せたい」は**できる**(参加した人だけ・みんなに公開のアプリで、作った人が管理者。管理者は参加者を管理者にできる)。担当者を参加者から選ぶこともできる。
- 「同じ時間に 2 人予約できないように」「1 人 1 回答」「1 人 1 票」は**できる**(同じ内容の記録はサーバーが保存を断る)。
`)

// Sentences of prompt.go / chat_first.go that said roles are impossible.
const (
	editRoleCannot   = "「店長だけが確定できる」のような役割ごとの権限は無いので、頼まれたら edit:\"members\" で代替し、その旨を書く。"
	editRoleCan      = "「店長だけが確定できる」のような管理者だけの権限は下の「管理者の権限と重複の防止」(editableBy / deleteBy / visibleTo)で作る。"
	limitRoleCannot  = "「店長だけが確定」のような役割・権限はできないので、members で全員が押せる形にするか確定を別の記録にして、「役割ごとの権限は今はできないため、○○は参加者の誰でも押せる形にしました。」と申告する。"
	limitRoleCan     = "「店長だけが確定」のような管理者だけの権限は下の「管理者の権限と重複の防止」で満たす。"
	chatRoleCannot   = "「店長だけが確定」のような役割・権限は今はできない。"
	chatRoleCan      = "「店長だけが確定」「削除は管理者だけ」のような管理者の権限もできる(作った人が管理者)。"
	dataRolesMember  = "- 担当者・当番・割り当てを参加者から選ぶ:"
	dataRolesButton  = "- 管理者だけのボタン:"
	dataRolesPrivate = "- 重複の防止(この版): 「同じ日に 2 件入れない」は entity に \"unique\":[\"<その field>\"](最大 3 つ)。サーバーが保存の時に断る。重複を探すボタン・filter・computed は作らない。"
)

func init() {
	systemPrompt = strings.Replace(systemPrompt, editRoleCannot, editRoleCan, 1)
	systemPrompt = strings.Replace(systemPrompt, limitRoleCannot, limitRoleCan, 1)
	systemPrompt += "\n\n" + dataRolesRules
	// chatCapabilities is the anchor other rules files insert after: change
	// it and the prompts together so their Replace still finds it.
	caps := strings.Replace(chatCapabilities, chatRoleCannot, chatRoleCan, 1)
	with := caps + "\n\n" + dataRolesChatRule
	chatEditSystemPrompt = strings.Replace(chatEditSystemPrompt, chatCapabilities, with, 1)
	chatCreateSystemPrompt = strings.Replace(chatCreateSystemPrompt, chatCapabilities, with, 1)
	chatCapabilities = caps
	for i := range omissionRules {
		if omissionRules[i].id != "shared_update" {
			continue
		}
		omitted := omissionRules[i].omitted
		// an admin-only field is the requested "only the manager confirms"
		omissionRules[i].omitted = func(t string, a *dsl.App) bool { return omitted(t, a) && !a.UsesRoles() }
	}
}

// FeatureDataRoles (#272) is a supportedFeatures value: the iOS build lists
// participants for a member field and hides visibleTo "admins" buttons from
// non-admins. Older builds show a member field as free text (the server
// refuses it) and every button to everyone.
const FeatureDataRoles = "dataRoles"

// dropDataRolesRules: the member / visibleTo lines go for builds without
// FeatureDataRoles; without shared data only unique on fields stays.
func dropDataRolesRules(s string, noShared bool) string {
	if noShared {
		s = strings.Replace(s, dataRolesRules, dataRolesPrivate, 1)
		return strings.Replace(s, "\n\n"+dataRolesChatRule, "", 1)
	}
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(ln, dataRolesMember) || strings.HasPrefix(ln, dataRolesButton) {
			continue
		}
		kept = append(kept, ln)
	}
	return strings.Join(kept, "\n")
}

// dataRolesNote is the restrictionNote line.
func dataRolesNote(noShared bool) string {
	if noShared {
		return "- editableBy / deleteBy / visibleTo、type member の field、unique の \"$author\" は使わない(記録はすべて自分だけ)。\n"
	}
	return "- type member の field と button の visibleTo は使わない(この版では参加者を選ぶ・管理者だけにボタンを出す表示ができない)。担当者は string の field、管理者だけの確定は field の editableBy \"admins\" で守る。\n"
}

// restrictDataRoles drops the parts from the schema defs.
func restrictDataRoles(defs map[string]any, noShared bool) {
	if field, _ := defs["Field"].(map[string]any); field != nil {
		props, _ := field["properties"].(map[string]any)
		if tp, _ := props["type"].(map[string]any); tp != nil {
			if en, ok := tp["enum"].([]any); ok {
				kept := make([]any, 0, len(en))
				for _, v := range en {
					if v != string(dsl.FieldMember) {
						kept = append(kept, v)
					}
				}
				tp["enum"] = kept
			}
		}
		if noShared {
			delete(props, "editableBy")
		}
	}
	if button, _ := defs["Button"].(map[string]any); button != nil {
		props, _ := button["properties"].(map[string]any)
		delete(props, "visibleTo")
	}
	if noShared {
		if entity, _ := defs["Entity"].(map[string]any); entity != nil {
			props, _ := entity["properties"].(map[string]any)
			delete(props, "deleteBy")
		}
	}
}

// unrole removes what the build cannot show from a generated document: a
// member field becomes string, visibleTo goes; without shared data
// editableBy, deleteBy and "$author" in unique go too. It returns how many
// places changed.
func unrole(root map[string]any, noShared bool) int {
	n := 0
	ents, _ := root["entities"].([]any)
	for _, e := range ents {
		em, _ := e.(map[string]any)
		if em == nil {
			continue
		}
		fs, _ := em["fields"].([]any)
		for _, f := range fs {
			fm, _ := f.(map[string]any)
			if fm == nil {
				continue
			}
			if fm["type"] == string(dsl.FieldMember) {
				fm["type"] = string(dsl.FieldString)
				n++
			}
			if _, has := fm["editableBy"]; has && noShared {
				delete(fm, "editableBy")
				n++
			}
		}
		if !noShared {
			continue
		}
		if _, has := em["deleteBy"]; has {
			delete(em, "deleteBy")
			n++
		}
		if u, ok := em["unique"].([]any); ok {
			kept := make([]any, 0, len(u))
			for _, v := range u {
				if v != dsl.RecordAuthor {
					kept = append(kept, v)
				}
			}
			if len(kept) != len(u) {
				n++
				if len(kept) == 0 {
					delete(em, "unique")
				} else {
					em["unique"] = kept
				}
			}
		}
	}
	var walk func([]any)
	walk = func(cs []any) {
		for _, v := range cs {
			c, _ := v.(map[string]any)
			if c == nil {
				continue
			}
			if _, has := c["visibleTo"]; has {
				delete(c, "visibleTo")
				n++
			}
			if ch, ok := c["children"].([]any); ok {
				walk(ch)
			}
		}
	}
	screens, _ := root["screens"].([]any)
	for _, s := range screens {
		if sc, _ := s.(map[string]any); sc != nil {
			cs, _ := sc["components"].([]any)
			walk(cs)
		}
	}
	return n
}
