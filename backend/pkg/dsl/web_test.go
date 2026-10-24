package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gallery-web.json must exercise webView and every shareButton preset so the
// backend, iOS decoding and the renderer all see each one at least once.
func TestGalleryWebCoversTypes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "gallery-web.json"))
	if err != nil {
		t.Skip("gallery not found:", err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ComponentType]bool{}
	presets := map[string]bool{}
	schemes := map[string]bool{}
	for _, s := range app.Screens {
		for _, c := range s.Components {
			seen[c.Type] = true
			if c.Type == CompShareButton {
				presets[c.ShareTo] = true
			}
			if c.Type == CompOpenURLButton {
				scheme, _, _ := strings.Cut(c.URL, ":")
				schemes[scheme] = true
			}
		}
	}
	for _, want := range []ComponentType{CompWebView, CompShareButton, CompOpenURLButton} {
		if !seen[want] {
			t.Errorf("gallery-web.json does not use %q", want)
		}
	}
	for _, want := range []string{"", "line", "slack", "discord"} {
		if !presets[want] {
			t.Errorf("gallery-web.json has no shareButton with shareTo %q", want)
		}
	}
	for _, want := range []string{"maps", "tel", "mailto", "line"} {
		if !schemes[want] {
			t.Errorf("gallery-web.json has no openUrlButton with scheme %q", want)
		}
	}
}

func TestWebSemanticErrors(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompWebView, URL: "http://example.com"},             // https only
		{Type: CompWebView, URL: "line://nv/chat"},                 // custom scheme not allowed
		{Type: CompShareButton, Text: "hi", ShareTo: "telegram"},   // unknown preset
		{Type: CompOpenURLButton, Label: "x", URL: "http://a.com"}, // http not allowed
		{Type: CompOpenURLButton, Label: "x", URL: "ftp://a.com"},  // scheme not allowlisted
		{Type: CompOpenURLButton, Label: "x", URL: "tel:"},         // empty rest
	}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`webView url "http://example.com" must be an https URL`,
		`webView url "line://nv/chat" must be an https URL`,
		`shareButton shareTo "telegram" must be one of any|line|slack|discord`,
		`openUrlButton url "http://a.com" must be https or one of the schemes`,
		`openUrlButton url "ftp://a.com" must be https`,
		`openUrlButton url "tel:" must be https`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems missing %q:\n%s", want, joined)
		}
	}
	if n := len(ve.Problems); n != 6 {
		t.Errorf("expected exactly 6 problems, got %d:\n%s", n, joined)
	}
}

func TestWebValidPasses(t *testing.T) {
	app := memoApp()
	app.Screens[0].Components = []Component{
		{Type: CompWebView, URL: "https://example.com", Title: "例", Area: "A1:F6"},
		{Type: CompShareButton, Text: "a", ShareTo: "line"},
		{Type: CompShareButton, Text: "b", ShareTo: "slack"},
		{Type: CompShareButton, Text: "c", ShareTo: "discord"},
		{Type: CompShareButton, Text: "d", ShareTo: "any"},
		{Type: CompShareButton, Text: "e"},
		{Type: CompOpenURLButton, Label: "https", URL: "https://example.com"},
		{Type: CompOpenURLButton, Label: "line", URL: "line://nv/chat"},
		{Type: CompOpenURLButton, Label: "slack", URL: "slack://open"},
		{Type: CompOpenURLButton, Label: "discord", URL: "discord://"},
		{Type: CompOpenURLButton, Label: "tel", URL: "tel:0761234567"},
		{Type: CompOpenURLButton, Label: "mail", URL: "mailto:a@example.com"},
		{Type: CompOpenURLButton, Label: "maps", URL: "maps:?q=Kanazawa"},
		{Type: CompOpenURLButton, Label: "gmaps", URL: "comgooglemaps://?q=Kanazawa"},
	}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

// The schema itself (not only Validate) must reject an unknown shareTo value
// and a webView without url, so Claude's structured output cannot emit them.
func TestWebSchemaRejects(t *testing.T) {
	for name, comp := range map[string]string{
		"shareTo enum":  `{"type":"shareButton","text":"x","shareTo":"telegram"}`,
		"webView url":   `{"type":"webView"}`,
		"webView extra": `{"type":"webView","url":"https://a.com","height":"large"}`,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","entities":[],
		"screens":[{"id":"home","title":"t","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s: expected schema error", name)
		}
	}
}
