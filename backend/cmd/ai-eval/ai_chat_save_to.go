package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiChat expect key: aiChatSaveTo (bool) = some aiChat proposes records
// from the conversation (saveTo; dsl/ai_chat_save_to.go). Cases:
// internal/ai/eval/ai_conversation_cases.json.
func init() {
	expectKeys["aiChatSaveTo"] = expectBool
}

func checkAIChatSaveTo(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiChatSaveTo"].(bool)
	if !ok {
		return nil
	}
	chats, saving := 0, 0
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompAiChat {
					chats++
					if x.SaveTo != nil && len(x.SaveTo.Fields) > 0 {
						saving++
					}
				}
			}
		}
	}
	if (saving > 0) != want {
		return []string{fmt.Sprintf("aiChatSaveTo=%d/%d aiChat", saving, chats)}
	}
	return nil
}
