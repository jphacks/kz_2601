package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// publicAppsFake2595 answers GetPublicApp only (the other methods panic).
type publicAppsFake2595 struct {
	AppMembers
	app *store.PublicApp
	err error
}

func (f *publicAppsFake2595) GetPublicApp(context.Context, string) (*store.PublicApp, error) {
	return f.app, f.err
}

func TestPublishedApp2595(t *testing.T) {
	cases := []struct {
		name     string
		fake     *publicAppsFake2595
		wantOK   bool
		wantName string // "" = app is nil
	}{
		{"not published", &publicAppsFake2595{err: store.ErrNotFound}, true, ""},
		{"not published (wrapped)", &publicAppsFake2595{err: fmt.Errorf("get: %w", store.ErrNotFound)}, true, ""},
		{"lookup failed", &publicAppsFake2595{err: errors.New("unavailable")}, false, ""},
		{"published", &publicAppsFake2595{app: &store.PublicApp{DSL: []byte(`{"schemaVersion":1,"app":{"name":"家計簿"}}`)}}, true, "家計簿"},
		{"published but not decodable", &publicAppsFake2595{app: &store.PublicApp{DSL: []byte(`{"app":`)}}, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &Service{Members: c.fake, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			app, ok := svc.publishedApp(context.Background(), "A")
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v", ok, c.wantOK)
			}
			switch {
			case c.wantName == "" && app != nil:
				t.Errorf("app = %+v, want nil", app)
			case c.wantName != "" && (app == nil || app.App.Name != c.wantName):
				t.Errorf("app = %+v, want name %q", app, c.wantName)
			}
		})
	}
}

func TestPIISuggestions2595(t *testing.T) {
	cases := []struct {
		loc  Locale
		want []string
	}{
		{LocaleJA, []string{"公開してよい", "個人の情報は入れない", "自分だけの記録にする"}},
		{LocaleEN, []string{"Yes, make it public", "Leave those details out", "Keep it private to me"}},
		{Locale(""), []string{"公開してよい", "個人の情報は入れない", "自分だけの記録にする"}},
	}
	for _, c := range cases {
		got := piiSuggestions(c.loc)
		if len(got) != len(c.want) {
			t.Fatalf("piiSuggestions(%q) = %q", c.loc, got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("piiSuggestions(%q)[%d] = %q, want %q", c.loc, i, got[i], c.want[i])
			}
		}
	}
}

func TestServerRulesFailed2595(t *testing.T) {
	ja := "自動の動き(サーバーからの通知など)は今回うまく作れなかったため、入れていません。"
	cases := []struct {
		loc  Locale
		want string
	}{
		{LocaleJA, ja},
		{LocaleEN, "The automatic action could not be made this time, so it was left out."},
		{Locale("fr"), ja},
	}
	for _, c := range cases {
		if got := c.loc.serverRulesFailed(); got != c.want {
			t.Errorf("Locale(%q).serverRulesFailed() = %q, want %q", c.loc, got, c.want)
		}
	}
}
