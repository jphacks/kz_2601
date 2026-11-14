package ai

import (
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// FeatureCallButton is the supportedFeatures value of callButton (#447,
// pkg/dsl/call_button.go). A build without it (and the review build) gets
// the old decline instead (payment_call_rules.go); so does every build while
// appconfig has CALLS_ENABLED off (Service.NoCalls).
const FeatureCallButton = "component.callButton"

// callButtonRules is one sentence. Without the feature it is rewritten to
// callDeclineRule, and the chat limit callLimitItem comes back.
const callButtonRules = "- 「アプリで通話・ビデオ通話したい」は callButton{label, video?: true}(参加者どうしでその場で話し、着信は無く、使った人のコインで払う・共有のアプリだけなので連絡の entity を scope shared にする)。"

// callLimitItem is the chat limit of a build without calls (#424).
const callLimitItem = "アプリの中での通話・ビデオ通話(代わり → 電話をかけるボタン(tel:)と、みんなで見られる記録での連絡・共有)、"

// CallsEnabled is appconfig's CALLS_ENABLED (cmd/ai: the AI writes
// callButton only while tokens can be issued).
func CallsEnabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("CALLS_ENABLED"))) {
	case "1", "true", "on":
		return true
	}
	return false
}

func isCallButton(c map[string]any) bool { return isType(c, dsl.CompCallButton) }

// dropCallButtonSchema removes CallButton from the Leaf union and $defs.
func dropCallButtonSchema(defs map[string]any) {
	if leaf, _ := defs["Leaf"].(map[string]any); leaf != nil {
		arr, _ := leaf["anyOf"].([]any)
		kept := []any{}
		for _, v := range arr {
			if sub, _ := v.(map[string]any); sub != nil && sub["$ref"] == "#/$defs/CallButton" {
				continue
			}
			kept = append(kept, v)
		}
		leaf["anyOf"] = kept
	}
	delete(defs, "CallButton")
}

// payLimitItem is the chat limit the call limit follows (#413).
func payLimitItem() string {
	for _, l := range chatLimits {
		if strings.HasPrefix(l.What, "ログイン・アプリの中での課金") {
			return l.What + "(代わり → " + l.Instead + ")、"
		}
	}
	panic("ai: the payment chat limit moved; update call_button_rules.go")
}

func init() {
	systemPrompt += "\n\n" + callButtonRules
	pay := payLimitItem()
	clientFeatures = append(clientFeatures, clientFeature{
		key: FeatureCallButton,
		rewrite: [][2]string{
			{callButtonRules, callDeclineRule},
			{pay, pay + callLimitItem},
		},
		note:      "- callButton(通話)は使わない(この版では動かない)。\n",
		schema:    dropCallButtonSchema,
		places:    func(root map[string]any) []string { return componentPlaces(root, isCallButton) },
		strip:     func(root map[string]any) int { return dropComponents(root, isCallButton) },
		violation: "component type \"callButton\" is not available in this app build: remove it (offer a tel: openUrlButton and shared records instead)",
	})
}
