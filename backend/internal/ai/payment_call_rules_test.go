package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #413 / #424: the decline rules reach the generation prompt and chat lists
// both as limits with the allowed alternative.
func TestPaymentCallRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## お金のやり取り・通話(#413・#424)", paywallNoteJA, paywallNoteEN, callNoteJA, callNoteEN, "paypay:// などの独自スキームは使わない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		for _, want := range []string{"払ったら有料の中身が開く仕組み", "PayPay の請求リンク", "アプリの中での通話・ビデオ通話", "tel:"} {
			if !strings.Contains(p, want) {
				t.Errorf("%s chat prompt lacks %q", name, want)
			}
		}
	}
}

// #413: paypay:// stays off the openUrlButton allowlist (https links only).
func TestPaymentNoPayPayScheme(t *testing.T) {
	for _, s := range dsl.OpenURLSchemes {
		if s == "paypay" {
			t.Fatal("paypay must not be an allowed openUrlButton scheme until confirmed (#413)")
		}
	}
}

// The decline survives a review build without openUrlButton (#245): no rule
// line has the 「…」は <type> form that restrictPrompt drops.
func TestPaymentCallRulesKeptWhenRestricted(t *testing.T) {
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != "openUrlButton" {
			allowed = append(allowed, typ)
		}
	}
	got := restrictPrompt(withRestriction(context.Background(), NewRestriction(allowed, nil)), systemPrompt)
	for _, want := range []string{paywallNoteJA, callNoteJA} {
		if !strings.Contains(got, want) {
			t.Errorf("restricted prompt lost %q", want)
		}
	}
}

func TestPaymentCallOmissions(t *testing.T) {
	app := &dsl.App{}
	ids := func(tr, honest string) string {
		return strings.Join(omissionIDs(omissions(tr, app, honest, true)), ",")
	}
	for _, c := range []struct{ tr, want string }{
		{"PayPayで払ったら有料の記事が読めるアプリ", "paywall"},
		{"課金したら全部の機能が使えるようにして", "paywall"},
		{"有料会員だけ見られるページを作りたい", "paywall"},
		{"Unlock premium content after payment", "paywall"},
		{"家族とビデオ通話できるアプリ", "call"},
		{"アプリの中で通話したい", "call"},
		{"I want to video call my team", "call"},
		// allowed: real-world bills, 割り勘, tel:
		{"有料駐車場の料金を記録したい", ""},
		{"PayPayで割り勘して払った人に印をつけたい", ""},
		{"ガソリン代の記録と、PayPayの請求リンクを開くボタン", ""},
		{"お店に電話をかけるボタンが欲しい", ""},
		{"通話料金を毎月記録する", ""},
	} {
		if got := ids(c.tr, ""); got != c.want {
			t.Errorf("%q: omissions = %q, want %q", c.tr, got, c.want)
		}
	}
	// An honest sentence in the description clears it.
	if got := ids("PayPayで払ったら有料の記事が読めるアプリ", paywallNoteJA); got != "" {
		t.Errorf("paywall still omitted after the honest note: %q", got)
	}
	if got := ids("家族とビデオ通話できるアプリ", callNoteJA); got != "" {
		t.Errorf("call still omitted after the honest note: %q", got)
	}
	if got := ids("I want to video call my team", callNoteEN); got != "" {
		t.Errorf("call still omitted after the en note: %q", got)
	}
	if got := ids("Unlock premium content after payment", paywallNoteEN); got != "" {
		t.Errorf("paywall still omitted after the en note: %q", got)
	}
}
