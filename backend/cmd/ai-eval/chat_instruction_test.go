package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

func TestChatInstruction(t *testing.T) {
	two := []ai.ChatOption{{Instruction: "家計簿を作る"}, {Instruction: "グラフを足す"}}
	for _, tc := range []struct {
		name    string
		res     ai.ChatResult
		all     bool
		want    string
		wantErr string
	}{
		{"multi-select all joins every option", ai.ChatResult{Kind: "proposal", MultiSelect: true, Options: two}, true, "・家計簿を作る\n・グラフを足す", ""},
		{"multi-select all with one option", ai.ChatResult{Kind: "proposal", MultiSelect: true, Options: two[:1]}, true, "家計簿を作る", ""},
		{"multi-select without all takes the first", ai.ChatResult{Kind: "proposal", MultiSelect: true, Options: two}, false, "家計簿を作る", ""},
		{"all on a single-select proposal takes the first", ai.ChatResult{Kind: "proposal", Options: two}, true, "家計簿を作る", ""},
		{"proposal takes the first option", ai.ChatResult{Kind: "proposal", Options: two}, false, "家計簿を作る", ""},
		{"ready uses the transcript", ai.ChatResult{Kind: "ready", Transcript: "家計簿アプリ"}, false, "家計簿アプリ", ""},
		{"proposal without options", ai.ChatResult{Kind: "proposal"}, false, "", "nothing to generate (kind proposal)"},
		{"question", ai.ChatResult{Kind: "question", Transcript: "x"}, true, "", "nothing to generate (kind question)"},
		{"empty kind", ai.ChatResult{}, false, "", "nothing to generate (kind )"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chatInstruction(&tc.res, tc.all)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
