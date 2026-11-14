package ai

// FeatureEncryptedFields: Field.encrypted (pkg/dsl/encrypted.go, #233). A
// build without it would store the value as plain text, so the AI does not
// write it there. The review build does not send it (no Tailor It Pro).
const FeatureEncryptedFields = "field.encrypted"

// encryptedFieldsRules is one sentence; appended like photoIllustrateRules so
// prompt.go stays untouched and dropped with the feature key.
const encryptedFieldsRules = "- 「暗号化して保存」「見られたくない」と頼まれた string / text の field には encrypted: true(端末で暗号化しサーバーと AI には暗号文だけ。private のアプリだけで、式・filter・sort・集計・serverRules・aiButton の input・aiChat の textField には使わない)。"

func init() {
	systemPrompt += "\n\n" + encryptedFieldsRules
	clientFeatures = append(clientFeatures, clientFeature{
		key:    FeatureEncryptedFields,
		rules:  []string{encryptedFieldsRules},
		note:   "- field の encrypted(暗号化して保存)は使わない(この版ではできない)。頼まれたら「項目の暗号化はこの版ではできないため、ふつうの項目にしました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "Field", "encrypted") },
		places: func(root map[string]any) []string {
			var out []string
			eachField(root, func(entity string, f map[string]any) {
				if f["encrypted"] != nil {
					id, _ := f["id"].(string)
					out = append(out, "entity "+entity+" field "+id)
				}
			})
			return out
		},
		strip: func(root map[string]any) int {
			n := 0
			eachField(root, func(_ string, f map[string]any) {
				if deleteKey(f, "encrypted") {
					n++
				}
			})
			return n
		},
		violation: "field encrypted is not available in this app build: omit encrypted (the value is stored as a normal field)",
	})
}

// eachField calls fn on every field object of every entity.
func eachField(root map[string]any, fn func(entity string, f map[string]any)) {
	ents, _ := root["entities"].([]any)
	for _, e := range ents {
		em, _ := e.(map[string]any)
		id, _ := em["id"].(string)
		fs, _ := em["fields"].([]any)
		for _, f := range fs {
			if fm, ok := f.(map[string]any); ok {
				fn(id, fm)
			}
		}
	}
}
