package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiChat expect key: aiChatInput (bool) = some aiChat reads the user's own
// records (inputEntity; dsl/ai_chat_input.go). Cases:
// internal/ai/eval/ai_conversation_cases.json.
func init() {
	expectKeys["aiChatInput"] = expectBool
}

func checkAIChatInput(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiChatInput"].(bool)
	if !ok {
		return nil
	}
	chats, reading := 0, 0
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompAiChat {
					chats++
					if x.InputEntity != "" {
						reading++
					}
				}
			}
		}
	}
	if (reading > 0) != want {
		return []string{fmt.Sprintf("aiChatInput=%d/%d aiChat", reading, chats)}
	}
	return nil
}
