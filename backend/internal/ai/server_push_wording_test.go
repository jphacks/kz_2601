package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/paid"
)

// The anchors the rewrite replaces must stay in the static prompts.
func TestServerPushWording_Anchors(t *testing.T) {
	if strings.Count(systemPrompt, chatThreadPushCannot) != 1 {
		t.Fatal("systemPrompt: chatThreadPushCannot drifted from chatThreadRules")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if strings.Count(p, chatLimitPushItem) != 1 || strings.Count(p, "\n"+publicPIIChatRule) != 1 {
			t.Errorf("%s: chatLimitPushItem / publicPIIChatRule anchor drifted", name)
		}
	}
	// Default (pass off) never promises push to recipients.
	if strings.Contains(addressedChatRule, "プッシュ通知") {
		t.Error("addressedChatRule still promises push")
	}
}

func TestServerPushWording_Rewrite(t *testing.T) {
	gen := rewriteServerPush(systemPrompt)
	if strings.Contains(gen, chatThreadPushCannot) || !strings.Contains(gen, chatThreadPushLater) {
		t.Error("generate prompt: push line not swapped")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := rewriteServerPush(p)
		if strings.Contains(got, "サーバーからのプッシュ通知") || !strings.Contains(got, serverPushChatRule) {
			t.Errorf("%s: chatLimits push item not swapped", name)
		}
		if !strings.Contains(got, "アプリの中での通話・ビデオ通話") || !strings.Contains(got, "順位番号") {
			t.Errorf("%s: other limits must stay", name)
		}
	}
	if !strings.Contains(serverPushChatRule, "Tailor It Pro") || strings.Contains(chatThreadPushLater, "代替しました") {
		t.Error("wording: Pro note for the creator / no 代替 in the generate line")
	}
}

func TestServerPushWording_Generate(t *testing.T) {
	without := ""
	if f, ok := defaultPaidCatalog.c.ByID(paid.ServerRules); ok {
		without = f.WithoutInstruction.JA
	}
	if without == "" {
		t.Fatal("serverRules has no withoutInstruction")
	}
	cases := []struct {
		name       string
		pass       bool
		disabled   []string
		transcript string
		on         bool
	}{
		{"default off", false, nil, "家計簿を作って。毎日リマインドして", false},
		{"pass on", true, nil, "家計簿を作って。毎日リマインドして", true},
		// The pass skips requests that ask for no automatic action, so the
		// prompt must not say 「この後の工程で作られる」 for them.
		{"pass on, no ask", true, nil, "家計簿を作って", false},
		{"pass on, ask the regex misses", true, nil, "部活のチャット。新着はプッシュして", false},
		{"no shared data", true, []string{FeatureSharedData}, "家計簿を作って。毎日リマインドして", false},
		{"opted out", true, nil, "家計簿を作って。毎日リマインドして" + without, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeCaller{replies: []string{string(kakeibo(t))}}
			svc, _ := newSvc(f)
			svc.ServerRulesPass = c.pass
			if _, err := svc.Generate(context.Background(), Request{AppID: "push-" + c.name, Transcript: c.transcript, DisabledFeatures: c.disabled}); err != nil {
				t.Fatal(err)
			}
			sys := f.calls[0].System
			if got := strings.Contains(sys, chatThreadPushLater); got != c.on {
				t.Errorf("later line present=%t, want %t", got, c.on)
			}
			if got := strings.Contains(sys, chatThreadPushCannot); got == c.on {
				t.Errorf("cannot line present=%t, want %t", got, !c.on)
			}
			ran := false
			for _, call := range f.calls {
				ran = ran || call.System == serverRulesPrompt
			}
			if ran != c.on {
				t.Errorf("server-rules pass ran=%t, but the push wording says %t", ran, c.on)
			}
		})
	}
}

func TestServerPushWording_Chat(t *testing.T) {
	for _, on := range []bool{false, true} {
		svc, f := newEditChatSvc(t, editAnswerReply)
		svc.ServerRulesPass = on
		if _, err := svc.Chat(context.Background(), ChatRequest{AppID: "a1", Messages: user("新しいメッセージを通知できる?"),
			Capabilities: []string{"answer", "proposal"}}); err != nil {
			t.Fatal(err)
		}
		sys := f.calls[0].System
		if got := strings.Contains(sys, serverPushChatRule); got != on {
			t.Errorf("pass=%t: serverPushChatRule present=%t", on, got)
		}
		if got := strings.Contains(sys, chatLimitPushItem); got == on {
			t.Errorf("pass=%t: push limit present=%t", on, got)
		}
	}
}

// Client restrictions (old builds, review builds with shared data) must not
// drop the anchors: the rewrite would silently do nothing. A build without
// chatThread drops chatThreadRules on purpose (nothing claims "cannot").
func TestServerPushWording_RestrictedKeepsAnchors(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for c := range knownComponentTypes {
		all = append(all, c)
	}
	ctx := withRestriction(context.Background(), NewClientRestriction(all, []string{FeatureFieldHealth, FeatureCameraCapture}, nil))
	ctx = withServerPush(ctx, true)
	for name, p := range map[string]string{"generate": systemPrompt, "edit": systemPrompt + "\n" + editRules} {
		got := serverPushPrompt(ctx, restrictPrompt(ctx, p))
		if strings.Contains(got, chatThreadPushCannot) || !strings.Contains(got, chatThreadPushLater) {
			t.Errorf("%s: restricted prompt not rewritten", name)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		got := serverPushPrompt(ctx, restrictPrompt(ctx, p))
		if strings.Contains(got, chatLimitPushItem) || !strings.Contains(got, serverPushChatRule) {
			t.Errorf("%s: restricted prompt not rewritten", name)
		}
	}
}
