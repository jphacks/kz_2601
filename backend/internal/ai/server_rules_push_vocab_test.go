package ai

import "testing"

func TestWantsServerRulesPush(t *testing.T) {
	for _, s := range []string{
		"部活のチャット。新着はプッシュして",
		"新着をプッシュ",
		"push notify me when someone posts",
		"Push me when a new message arrives",
		"push",
	} {
		if !wantsServerRules(s) {
			t.Errorf("%q should run the pass", s)
		}
	}
	// "push-up counter" is left out: "count" already runs the pass.
	for _, s := range []string{
		"プッシュアップを記録",
		"push-up log",
		"pushups tracker",
		"PUSH-UP diary",
	} {
		if wantsServerRules(s) {
			t.Errorf("%q should not run the pass", s)
		}
	}
}
