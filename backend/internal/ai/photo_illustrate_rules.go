package ai

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// FeatureIllustrate: photoField.illustrate (pkg/dsl/photo_illustrate.go,
// Apple Image Playground on the device). A build without it ignores the key,
// so the AI does not write it there. The review build does not send it.
const FeatureIllustrate = "photoField.illustrate"

// photoIllustrateRules is one sentence; appended like addMinutesRules so
// prompt.go stays untouched and dropped with the feature key.
const photoIllustrateRules = "- 「アイコン・イラスト・挿絵を AI で作る」は photoField に illustrate: true(端末内の Image Playground でタイトルから作り、写真と同じく photo field に入れる。使えない端末ではボタンが出ないので、写真を選ぶ手段は残す)。"

func isIllustrate(c map[string]any) bool {
	return isType(c, dsl.CompPhotoField) && c["illustrate"] != nil
}

func init() {
	systemPrompt += "\n\n" + photoIllustrateRules
	clientFeatures = append(clientFeatures, clientFeature{
		key:    FeatureIllustrate,
		rules:  []string{photoIllustrateRules},
		note:   "- photoField の illustrate(端末内でイラストを作る)は使わない(この版では動かない)。写真を選ぶ photoField にし、頼まれたら「イラストを作る機能はこの版ではできないため、写真を選ぶ形にしました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "PhotoField", "illustrate") },
		places: func(root map[string]any) []string { return componentPlaces(root, isIllustrate) },
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool { return isIllustrate(c) && deleteKey(c, "illustrate") })
		},
		violation: "photoField illustrate is not available in this app build: omit illustrate (the photo field still takes a picked photo)",
	})
}
