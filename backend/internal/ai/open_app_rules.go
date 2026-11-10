package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeatureOpenApp: openUrlButton app / to and shareButton shareTo x / mail
// ("open & send" integrations, pkg/dsl/open_app.go). An older build shows an
// openUrlButton without url as unsupported, so the AI does not write them
// there (the button is removed, shareTo x / mail falls back to the share
// sheet).
const FeatureOpenApp = "openUrlButton.app"

// openAppRules: two sentences, appended like photoIllustrateRules so
// prompt.go / catalog.go stay untouched and dropped with the feature key.
const openAppRules = "- 他のアプリとの連携(PayPay・LINE・Slack・Discord・X・Instagram・地図・メール・電話)は「開く」と「送る」で作る: 開くは openUrlButton{app, to?}(例: 割り勘 → {app: paypay, label: 「PayPay で払う」}、お店の電話 → {app: phone, to: \"{{phone}}\"}、場所 → {app: maps, to: \"{{address}}\"})、記録を文面で送るは shareButton{text: \"{{title}} {{amount}}円\", shareTo: line|x|mail|slack|discord}(例: 割り勘の結果を LINE で送る)。押すとそのアプリが開き、送る・払うのは利用者(自動の投稿・取り込みはできない)。to のリンク・番号・アドレス・ユーザー名は作らない(言われた物か {{fieldId}} だけ)。"

// paypayNoLinkRule replaces #413's "no link, no button" in paymentCallRules:
// a PayPay button can open the app without a link. A build without the
// feature gets paypayNoLinkRuleOff back (rewrite).
const (
	paypayNoLinkRule    = "(言われていなければ to を省いた openUrlButton{app: paypay, label: 「PayPay を開く」}(PayPay を開くだけ)にし、記録も作って app.description に「PayPay の請求リンク(https)を教えてもらえれば、そのページを直接開けます。」と書く)"
	paypayNoLinkRuleOff = "(言われていなければボタンは置かず、記録だけ作って app.description に「支払いページのリンク(https)を教えてもらえれば、開くボタンを置けます。」と書く)"
)

func isOpenApp(c map[string]any) bool {
	return isType(c, dsl.CompOpenURLButton) && (c["app"] != nil || c["to"] != nil)
}

func isShareToNew(c map[string]any) bool {
	return isType(c, dsl.CompShareButton) && (c["shareTo"] == "x" || c["shareTo"] == "mail")
}

func init() {
	systemPrompt += "\n\n" + openAppRules
	clientFeatures = append(clientFeatures, clientFeature{
		key:     FeatureOpenApp,
		rules:   []string{openAppRules},
		rewrite: [][2]string{{paypayNoLinkRule, paypayNoLinkRuleOff}},
		note:    "- openUrlButton の app / to と shareButton の shareTo x / mail は使わない(この版では動かない)。アプリを開くのは openUrlButton の url(https・tel:・mailto: など)、送るのは shareButton の shareTo line|slack|discord|any にする。\n",
		schema: func(defs map[string]any) {
			deleteProps(defs, "OpenUrlButton", "app", "to")
			dropEnumValues(defs, "ShareButton", "shareTo", "x", "mail")
			d, _ := defs["OpenUrlButton"].(map[string]any)
			if d == nil {
				return
			}
			d["required"] = []any{"type", "label", "url"}
			for _, k := range []string{"description", keyUsage} {
				if s, ok := d[k].(string); ok {
					d[k] = dropSentences(s, "app")
				}
			}
			if props, _ := d["properties"].(map[string]any); props != nil {
				if u, _ := props["url"].(map[string]any); u != nil {
					if s, ok := u["description"].(string); ok {
						u["description"] = strings.Replace(s, "(app とどちらか 1 つ)", "", 1)
					}
				}
			}
		},
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool { return isOpenApp(c) || isShareToNew(c) })
		},
		strip: func(root map[string]any) int {
			return dropComponents(root, isOpenApp) +
				editComponents(root, func(c map[string]any) bool { return isShareToNew(c) && deleteKey(c, "shareTo") })
		},
		violation: "openUrlButton app / to and shareButton shareTo x / mail are not available in this app build: use openUrlButton url (https / tel: / mailto:) and shareTo line|slack|discord|any",
	})
}
