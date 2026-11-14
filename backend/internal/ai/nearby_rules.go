package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// nearbyPassbyRules (#425): passby exchange and UWB distance are a mode of
// nearbyShare. Appended like visualRules so prompt.go stays untouched.
var nearbyPassbyRules = strings.TrimSpace(`
## すれ違い通信・近くの人との距離(#425)
- 「すれ違った人とプロフィールを交換」「すれ違い通信」「近くにいる同じアプリの人と自動で交換」「会った人の記録」「近くの友だちとの距離・方向を表示」は nearbyShare の **mode:"passby"**(距離・方向は **distance:true** を足す。距離だけの別部品は無い)。ボタンで相手を選ぶ send(既定)ではない。
- 形: 自分のプロフィールの entity(例 profile{name: string(required), comment: string, hobby: enum(任意)}・**scope は private のまま**。受け取った相手のプロフィールも同じ entity に追加される)+ すれ違いの画面(タブ)にその部品{mode:"passby", label:「すれ違い通信」, entity:profile, fields:[送る field id], limit?:1, distance?:true}(A1:F8)+ 交換したプロフィールの list(A9:F12)+ 自分のプロフィールを書くフォーム画面(screen.entity + saveRecord)への導線。
- **fields には送ってよい項目だけを明示する**(名前・ひとこと・趣味など)。hidden・photo・video・ref・refs・computed は指定できない。電話番号・住所・メールなど個人の連絡先は、ユーザーが明示した時だけ入れる。
- できないこと: アプリを裏に回した・画面を閉じた後のすれ違い(iOS の制約。画面を開いている間だけ動く)/ 写真・動画の交換 / 距離・方向は対応端末(iPhone 11 以降・SE を除く)だけ。求められたら summary で正直に伝える。
`)

// FeatureNearbyPassby (#1955) is a disabledFeatures value: the client renders
// nearbyShare but cannot run mode passby (Mac). The AI then makes only send;
// a client that does not send the value keeps passby.
const FeatureNearbyPassby = "nearbyShare.passby"

func init() {
	systemPrompt += "\n\n" + nearbyPassbyRules
	disabledClientFeatures = append(disabledClientFeatures, nearbyPassbyFeature)
}

var nearbyPassbyFeature = clientFeature{
	key:   FeatureNearbyPassby,
	rules: []string{nearbyPassbyRules},
	note:  "- nearbyShare の mode passby(すれ違い通信・近くの人との距離と方向)は使わない(この端末では動かない)。nearbyShare はボタンで相手を選んで送る send だけにする。すれ違い通信や距離の表示を頼まれたら、summary で「この端末ではすれ違い通信は使えないため、ボタンで相手を選んで送る形にしました。」と正直に伝える。\n",
	schema: func(defs map[string]any) {
		dropEnumValues(defs, "NearbyShare", "mode", dsl.NearbyModePassby)
		deleteProps(defs, "NearbyShare", "fields", "limit", "distance")
		d, _ := defs["NearbyShare"].(map[string]any)
		if d == nil {
			return
		}
		for _, k := range []string{"description", keyUsage} {
			if s, ok := d[k].(string); ok {
				d[k] = dropSentences(s, "passby", "distance", "プロフィール")
			}
		}
		props, _ := d["properties"].(map[string]any)
		for _, name := range []string{"mode", "entity"} {
			if p, _ := props[name].(map[string]any); p != nil {
				if s, ok := p["description"].(string); ok {
					p["description"] = dropSentences(s, "passby")
				}
			}
		}
	},
	places:    func(root map[string]any) []string { return componentPlaces(root, isNearbyPassby) },
	strip:     func(root map[string]any) int { return dropComponents(root, isNearbyPassby) },
	violation: "nearbyShare mode passby is not available on this device: use mode send (a button that picks the receiver) without fields / limit / distance, or remove it",
}

func isNearbyPassby(c map[string]any) bool {
	return isType(c, dsl.CompNearbyShare) && c["mode"] == dsl.NearbyModePassby
}
