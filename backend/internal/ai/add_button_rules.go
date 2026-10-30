package ai

import (
	"encoding/json"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addButtonRules (#416): the top-right ＋ (toolbar) and the bottom-right add
// button both opened the same form. Appended like randomRules so prompt.go
// stays untouched; dropDuplicateAddToolbar enforces it without a retry.
var addButtonRules = strings.TrimSpace(`
## 追加ボタンは 1 画面に 1 つ(#416)
- 同じ画面に「同じフォームを開く」追加の操作を 2 つ置かない。一覧の画面は右下の button(D11:F12 など)だけにし、toolbar に同じフォームへの navigate(＋)を**重ねない**。
- toolbar の＋にするのは、その画面に追加の button を置かない時だけ(グリッドが埋まっている・一覧だけの画面など)。
- 既存アプリの編集で右下に追加ボタンを足す時は、同じフォームを開く toolbar の項目を remove する(逆も同じ)。
`)

func init() { systemPrompt += "\n\n" + addButtonRules }

// dropDuplicateAddToolbar removes the toolbar items dsl.DuplicateAddToolbar
// reports (a button on the same screen opens the same form). Returns raw
// unchanged when nothing applies or the result does not validate.
func dropDuplicateAddToolbar(raw []byte, app *dsl.App) []byte {
	drop := map[string]map[int]bool{}
	for i := range app.Screens {
		s := &app.Screens[i]
		for _, j := range app.DuplicateAddToolbar(s) {
			if drop[s.ID] == nil {
				drop[s.ID] = map[int]bool{}
			}
			drop[s.ID][j] = true
		}
	}
	if len(drop) == 0 {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	screens, _ := doc["screens"].([]any)
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		id, _ := sm["id"].(string)
		idx := drop[id]
		if idx == nil {
			continue
		}
		tb, _ := sm["toolbar"].([]any)
		var keep []any
		for j, t := range tb {
			if !idx[j] {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			delete(sm, "toolbar")
		} else {
			sm["toolbar"] = keep
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	if _, err := dsl.Parse(out); err != nil {
		return raw
	}
	return out
}
