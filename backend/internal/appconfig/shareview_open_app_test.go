package appconfig

import (
	"strings"
	"testing"
)

// An openUrlButton app without label shows the service's name (disabled like
// every button) and never its link.
func TestSharePageOpenAppButton(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"app":{"name":"割り勘","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"bill","name":"支払い","fields":[{"id":"title","name":"内容","type":"string"}]}],
"screens":[{"id":"home","title":"ホーム","components":[
 {"type":"list","entity":"bill","item":{"title":"title"}},
 {"type":"openUrlButton","app":"paypay","to":"https://qr.paypay.ne.jp/p2p01_secret"}]}]}`)
	p := buildWebPage("", raw, nil)
	found := false
	for _, s := range p.Screens {
		for _, n := range append(s.Grid, s.Stack...) {
			found = found || (n.Kind == "button" && n.Label == "PayPay")
		}
	}
	if !found {
		t.Fatalf("no PayPay button: %+v", p.Screens)
	}
	p.Lang, p.Nonce = "ja", "n"
	body, err := renderSharePage(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "p2p01_secret") {
		t.Fatal("the share page must not show the payment link")
	}
}
