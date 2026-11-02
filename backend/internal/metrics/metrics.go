// Package metrics writes the product-metric events (#297) as structured log
// lines: Cloud Logging filter jsonPayload.event=~"^(session|app|share)\.".
// Labels, counts and pseudonymous keys only: never input text, DSL contents,
// names, raw uids, raw appIds or share tokens. docs/metrics/product-metrics.md
package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
)

// Event names (the message and the `event` attr).
const (
	SessionStart  = "session.start"  // GET /v1/apps, first page
	AppOpened     = "app.opened"     // GET /v1/apps/{appId}/dsl without If-None-Match
	AppShared     = "app.shared"     // POST /v1/apps/{appId}/share succeeded
	AppPublished  = "app.published"  // POST /v1/apps/{appId}/publish succeeded
	ShareImported = "share.imported" // share import / public install created a copy
)

// UIDHash is the pseudonymous user key. Same value as ai.UIDHash (ai.usage,
// ai.generate) so the events join per user; empty uid gives "".
func UIDHash(uid string) string {
	if uid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("ai-usage:" + uid))
	return hex.EncodeToString(sum[:8])
}

// AppHash is the pseudonymous app key; empty appId gives "".
func AppHash(appID string) string {
	if appID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("metrics-app:" + appID))
	return hex.EncodeToString(sum[:8])
}

// Emit writes one event line on slog.Default with uid_hash added.
func Emit(ctx context.Context, event, uid string, attrs ...slog.Attr) {
	all := append([]slog.Attr{slog.String("event", event)}, attrs...)
	if h := UIDHash(uid); h != "" {
		all = append(all, slog.String("uid_hash", h))
	}
	slog.Default().LogAttrs(ctx, slog.LevelInfo, event, all...)
}
