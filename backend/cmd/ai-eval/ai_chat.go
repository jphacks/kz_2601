package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiChat expect key: aiChatSpeak (bool) = some aiChat has speak: true (new
// AI replies are read aloud). Case: internal/ai/eval/ai_conversation_cases.json.
func init() {
	expectKeys["aiChatSpeak"] = expectBool
}

func aiChatSpeaks(a *dsl.App) (chats, speaking int) {
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompAiChat {
					chats++
					if x.Speak {
						speaking++
					}
				}
			}
		}
	}
	return chats, speaking
}

func checkAIChatSpeak(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiChatSpeak"].(bool)
	if !ok {
		return nil
	}
	chats, speaking := aiChatSpeaks(a)
	if (speaking > 0) != want {
		return []string{fmt.Sprintf("aiChatSpeak=%d/%d aiChat", speaking, chats)}
	}
	return nil
}
