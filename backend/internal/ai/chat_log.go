package ai

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// ChatEvent is the message (and `event` attr) of the per-turn log of
// POST /v1/chat. Cloud Logging filter: jsonPayload.event="ai.chat".
const ChatEvent = "ai.chat"

// reply_kind of ai.chat: the kind the turn returned (ChatResult.Kind).
const (
	ChatKindQuestion = "question"
	ChatKindReady    = "ready"
	ChatKindAnswer   = "answer"
	ChatKindProposal = "proposal"
	ChatKindOther    = "other" // a kind this list does not know yet
)

// chatModeClarify labels the legacy clarify path (ChatResult.Mode empty).
const chatModeClarify = "clarify"

// chatReplyKind maps ChatResult.Kind onto the fixed vocabulary.
func chatReplyKind(k string) string {
	switch k {
	case ChatKindQuestion, ChatKindReady, ChatKindAnswer, ChatKindProposal:
		return k
	}
	return ChatKindOther
}

// logChat writes one ai.chat line for a /v1/chat turn. An early reply returns
// while its Claude call still runs; the line then waits for it (up to
// coinLateWait) so the token counts are whole. The labels are read now,
// before the handler goes on with res.
func (s *Service) logChat(ctx context.Context, req ChatRequest, res *ChatResult, err error, d time.Duration) {
	attrs, level := s.chatLogAttrs(ctx, req, res, err, d)
	t := tallyOf(ctx)
	if err != nil || !t.busy() {
		s.writeChatLog(ctx, attrs, level, false)
		return
	}
	go func() {
		bctx := context.WithoutCancel(ctx)
		wctx, cancel := context.WithTimeout(bctx, coinLateWait)
		t.wait(wctx)
		cancel()
		s.writeChatLog(bctx, attrs, level, true)
	}()
}

func (s *Service) writeChatLog(ctx context.Context, attrs []slog.Attr, level slog.Level, early bool) {
	attrs = append(attrs, slog.Bool("early_reply", early))
	attrs = append(attrs, tallyOf(ctx).attrs()...) // tokens and cost of the turn's Claude calls
	s.logger().LogAttrs(ctx, level, ChatEvent, attrs...)
}

// chatLogAttrs: labels, counts and hashes only. Never the messages, reply
// text, suggestions, options, brief, transcript, appId or raw uid.
func (s *Service) chatLogAttrs(ctx context.Context, req ChatRequest, res *ChatResult, err error, d time.Duration) ([]slog.Attr, slog.Level) {
	mode := chatModeClarify
	if wantsChatFirst(req) {
		mode = "unknown" // chat-first failed before create / edit was decided
	}
	if res != nil && res.Mode != "" {
		mode = res.Mode
	}
	attrs := []slog.Attr{
		slog.String("event", ChatEvent),
		slog.String("mode", mode),
		slog.Int64("duration_ms", d.Milliseconds()),
	}
	model := s.ChatConfig.resolved().Model
	if err == nil && res != nil {
		kind := chatReplyKind(res.Kind)
		if res.Meta.Model != "" {
			model = res.Meta.Model
		}
		attrs = append(attrs,
			slog.String("outcome", "replied"),
			slog.String("reply_kind", kind),
			slog.String("source", res.Source), // claude|shortcut|forced
			slog.Int("turn", res.Turn),
			// a ready after at least one earlier turn: the clarify chain ended in a build request
			slog.Bool("clarify_ended_in_build", kind == ChatKindReady && res.Turn > 1),
			slog.Int64("claude_ms", res.LatencyMs.Claude),
			slog.Int64("first_token_ms", res.FirstTokenMs),
			slog.Int("attempts", res.Meta.Attempts),
			slog.Int("suggestions", len(res.Suggestions)),
			slog.Int("options", len(res.Options)),
			slog.Bool("requires_pro", res.RequiresPro),
		)
	} else {
		outcome, code := "failed", "internal"
		var ge *Error
		if errors.As(err, &ge) {
			code = ge.Code
			if ge.Status >= 400 && ge.Status < 500 && ge.Code != httpx.CodeRateLimited && ge.Code != httpx.CodeTimeout {
				outcome = "bad_request"
			}
		}
		attrs = append(attrs, slog.String("outcome", outcome), slog.String("error_code", code))
	}
	attrs = append(attrs, slog.String("model", model))
	if h := UIDHash(auth.UID(ctx)); h != "" {
		attrs = append(attrs, slog.String("uid_hash", h))
	}
	if h := metrics.AppHash(req.AppID); h != "" { // groups turns into conversations
		attrs = append(attrs, slog.String("app_hash", h))
	}
	level := slog.LevelInfo
	var ge *Error
	if err != nil && (!errors.As(err, &ge) || ge.Status >= 500) {
		level = slog.LevelWarn
	}
	return attrs, level
}
