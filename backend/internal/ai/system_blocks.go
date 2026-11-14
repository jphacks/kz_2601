package ai

import (
	"context"
	"fmt"
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

// Cache lifetimes of the cached system blocks (#2295).
const (
	CacheTTL5m = "5m" // the API's default: cache_control has no ttl
	CacheTTL1h = "1h"
)

// GenerateCacheTTLFromEnv reads AI_GENERATE_CACHE_TTL: unset or 5m keeps the
// 5-minute cache, 1h writes the create turn's system blocks for an hour
// (#1770 案 A). Anything else is a startup error.
func GenerateCacheTTLFromEnv(getenv func(string) string) (string, error) {
	switch v := strings.TrimSpace(getenv("AI_GENERATE_CACHE_TTL")); v {
	case "", CacheTTL5m:
		return CacheTTL5m, nil
	case CacheTTL1h:
		return CacheTTL1h, nil
	default:
		return "", fmt.Errorf("AI_GENERATE_CACHE_TTL must be 5m or 1h, got %q", v)
	}
}

// systemCacheTTL is the lifetime of the cached system blocks of a call: 1h
// only for a create turn (OpGenerate) with GenerateCacheTTL 1h; chat and
// edit keep 5m.
func (c *Client) systemCacheTTL(ctx context.Context) string {
	if c.GenerateCacheTTL == CacheTTL1h && operationOf(ctx) == OpGenerate {
		return CacheTTL1h
	}
	return CacheTTL5m
}

// cacheControl is the cache_control of a cached system block of ttl.
func cacheControl(ttl string) map[string]any {
	if ttl == CacheTTL1h {
		return map[string]any{"type": "ephemeral", "ttl": CacheTTL1h}
	}
	return map[string]any{"type": "ephemeral"}
}
