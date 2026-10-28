package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// marksRules (#255): likes came out as a like entity with a post picker (one
// person could press it forever) and "save someone's recipe" as a copy entity
// re-typing every field. entity.marks is a once-per-person toggle stored by
// appconfig. Appended like visualRules so prompt.go / catalog.go stay untouched;
// the catalog part is built from the schema so the two cannot drift.
var marksRules = strings.TrimSpace(`
## いいね・保存・既読は entity.marks(#255)
- 共有(public / shared)のレコードに「いいね」「お気に入り」「保存」「ブックマーク」「既読」「行きたい」「役に立った」を付けたい時は、その entity に "marks":[{"id":"like","icon":"heart","label":"いいね"}] を置き、一覧の item に "mark":"like" を書く。行の右端にアイコンと件数が出て、1 人 1 回で付け外しできる(他の人の投稿にも付けられる・押してもフォームは開かない)。
- **いいね用・保存用・既読用の別 entity、親を指す ref、件数の computed(agg count)、「いいねする」フォームやボタンを作らない**(1 人で何回でも押せる・投稿を選び直させる・全項目の打ち直しになる)。他の人のレシピやお店を自分用に取っておくのも、写しの entity ではなく marks の save。コメント・出欠・回答のように中身のある記録は今まで通り子 entity。
- 件数: 行のトグルに件数が出るので item.trailing には要らない。表や並べ替えには "$marks.<id>"(table の列 {"field":"$marks.like","title":"いいね"}・人気順 sort {"field":"$marks.like","direction":"desc"})。agg の中では数えられない。
- 「保存したもの」「お気に入り」「既読にしたもの」のタブ・一覧: list の filter {"field":"$marks.save","op":"eq","value":{"me":true}} と同じ item.mark(そこで外せる)。未読・まだ見ていないもの = op "neq"。自分が付けた数 = stat aggregate count + 同じ filter。
- item.mark は 1 行に 1 つ。いいねと保存の両方を頼まれたら marks に 2 つ置き、新着の一覧の行は like、もう 1 つの一覧(人気順・さがす など)の行と「保存した」タブの行は save にする。
- private の entity(自分だけの記録)には marks を書かない。自分の記録の「済み」「お気に入り」は boolean field + list.item.toggle。いいねを頼まれた個人の記録アプリは、みんなで見る用途なら entity を public にしてから marks。
- 編集ターン(「いいねボタンを付けて」「保存できるようにして」): add /entities/<i>/marks(無ければ配列ごと)と、一覧の item に add .../item/mark。保存したタブは /screens/- と /app/tabs/- に add。既存の子 entity を作り直さない。
`)

func init() { systemPrompt += "\n\n" + marksRules + "\n" + marksCatalog }

// marksCatalog renders Entity.marks and $defs/Mark from the canonical schema.
var marksCatalog = mustMarksCatalog(dsl.SchemaJSON())

func mustMarksCatalog(raw []byte) string {
	out, err := buildMarksCatalog(raw)
	if err != nil {
		panic("ai: build marks catalog: " + err.Error())
	}
	return out
}

func buildMarksCatalog(schema []byte) (string, error) {
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return "", err
	}
	defs, _ := root["$defs"].(map[string]any)
	entity, _ := defs["Entity"].(map[string]any)
	props, _ := entity["properties"].(map[string]any)
	marks, _ := props["marks"].(map[string]any)
	mark, _ := defs["Mark"].(map[string]any)
	if marks == nil || mark == nil {
		return "", fmt.Errorf("schema has no $defs/Entity.properties.marks or $defs/Mark")
	}
	var b strings.Builder
	desc, _ := marks["description"].(string)
	fmt.Fprintf(&b, "### Entity の marks(1 人 1 回の印)— %s\n", desc)
	if usage, _ := marks[keyUsage].(string); usage != "" {
		fmt.Fprintf(&b, "  使い方: %s\n", usage)
	}
	fmt.Fprintf(&b, "- mark{%s}", propList(defs, mark, "", false, nil))
	if d, _ := mark["description"].(string); d != "" {
		b.WriteString(" — " + d)
	}
	b.WriteString("\n")
	icons := enumValues(mark, "icon")
	if len(icons) == 0 {
		return "", fmt.Errorf("schema has no $defs/Mark.properties.icon enum")
	}
	parts := make([]string, 0, len(icons))
	for _, v := range icons {
		parts = append(parts, v+"="+enumDescOf(mark, "icon", v))
	}
	b.WriteString("- icon: " + strings.Join(parts, " / "))
	return b.String(), nil
}
