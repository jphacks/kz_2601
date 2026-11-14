package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// PromptVersion identifies the prompt text built into this binary (#372 #298):
// "p-" + the first 12 hex of sha256(systemPrompt, editRules). It changes
// exactly when a prompt or recipe block in the code changes, so a generation
// records which prompt made it (Source.PromptVersion, ai.generate
// prompt_version) without a hand-bumped number. Which recipes a request sent
// is Meta.Recipes. Computed on first use: systemPrompt is assembled by init.
var PromptVersion = sync.OnceValue(func() string {
	h := sha256.New()
	h.Write([]byte(systemPrompt))
	h.Write([]byte{0})
	h.Write([]byte(editRules))
	return "p-" + hex.EncodeToString(h.Sum(nil))[:12]
})
