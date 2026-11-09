package ai

import (
	"context"
	"strings"
)

// editSystemSuffix is the second system block of an edit turn. The first block
// is systemPrompt, byte-identical to create's, so edit reuses create's cached
// prefix instead of writing its own ~100K-token entry (#630).
func editSystemSuffix() string { return "\n" + editRules }

// restrictPromptSuffix restricts a second system block: the rewrites of
// restrictPrompt without the trailing restriction note, which the first block
// already carries.
func restrictPromptSuffix(ctx context.Context, suffix string) string {
	r := restrictionFrom(ctx)
	if suffix == "" || !r.Active() {
		return suffix
	}
	return strings.TrimSuffix(restrictPrompt(ctx, suffix), "\n\n"+restrictionNote(r))
}
