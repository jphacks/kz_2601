package ai

import "strings"

// aiChatInputEditRule: the aiChat section says 「aiChat は会話だけを読む」;
// this line, right after the inputEntity section (runs after
// ai_chat_rules_input.go: files initialize in name order), says that is the
// case without inputEntity and how to add it to an existing aiChat. It
// routes with 「…」は aiChat so a client without aiChat drops it (restrict.go).
var aiChatInputEditRule = `- 既存の aiChat への「日記も読んだうえで話して」「記録を見て答えて」は aiChat に inputEntity を足す patch(記録の entity が無ければフォーム・list の画面と一緒に add)。aiChat の節の「aiChat は会話だけを読む・記録を材料にまとめたいなら aiButton」は inputEntity を付けない時の話。`

func init() {
	systemPrompt = strings.Replace(systemPrompt, aiChatInputRules, aiChatInputRules+"\n"+aiChatInputEditRule, 1)
}
