package ai

import (
	"strings"
	"testing"
)

// #412: the music rules reach the generation prompt and both chat prompts,
// and the catalog / chat index (built from the schema) list musicPlayer.
func TestMusicRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 音楽を流す(#412)", "allowRequests: true", "Spotify・YouTube Music の再生・操作はできない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"edit": chatEditSystemPrompt, "create": chatCreateSystemPrompt} {
		if !strings.Contains(p, "Spotify は曲名から Spotify アプリを開くことだけできる") {
			t.Errorf("chat %s prompt lacks the Spotify limit", name)
		}
	}
	if !strings.Contains(catalog, "\n- musicPlayer:") {
		t.Error("catalog lacks musicPlayer")
	}
	if !strings.Contains(chatCapabilityIndex, "musicPlayer") {
		t.Error("chat capability index lacks musicPlayer")
	}
}

// Review build (#245): nearbyShare is not allowed, so the friend requests
// (same peer-to-peer link) are off too; playback stays.
func TestMusicRequestsOffWithoutNearby(t *testing.T) {
	all := []string{}
	for k := range knownComponentTypes {
		if k != "nearbyShare" {
			all = append(all, k)
		}
	}
	if note := restrictionNote(NewRestriction(all, nil)); !strings.Contains(note, "allowRequests / autoAccept は使わない") {
		t.Errorf("restriction without nearbyShare should turn off song requests:\n%s", note)
	}
	if note := restrictionNote(NewRestriction(nil, []string{FeatureCameraCapture})); strings.Contains(note, "allowRequests") {
		t.Error("song requests turned off although nearbyShare is allowed")
	}
}

// The chat rule is read by non-engineers' replies: it names no DSL terms.
func TestMusicChatRuleIsPlain(t *testing.T) {
	if left := LeakedTerms(musicChatRule); len(left) > 0 {
		t.Errorf("musicChatRule uses DSL terms %v", left)
	}
	if strings.Contains(musicChatRule, "musicPlayer") || strings.Contains(musicChatRule, "allowRequests") {
		t.Error("musicChatRule should not spell the component or its keys")
	}
}
