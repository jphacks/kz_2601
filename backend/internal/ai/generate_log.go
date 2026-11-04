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

// GenerateEvent is the message (and `event` attr) of the per-request log of
// POST /v1/generate (#297 #322). Cloud Logging filter: jsonPayload.event="ai.generate".
const GenerateEvent = "ai.generate"

// ai.generate outcomes. validated / regenerated / unchanged passed validation.
const (
	GenValidated   = "validated"   // first attempt passed validation
	GenRegenerated = "regenerated" // passed after a retry, area strip, downgrade or lint retry
	GenUnchanged   = "unchanged"   // edit passed but changed nothing (#211)
	GenRestored    = "restored"    // undo (no generation)
	GenRefused     = "refused"     // the model refused
	GenFailed      = "failed"      // validation / upstream / store failure
	GenBadRequest  = "bad_request" // caller error (4xx), excluded from the success rate
)

// genTrace is what GenerateStream learns before it can fail or return.
type genTrace struct {
	mode  string // create|edit|undo; "" before the mode is decided
	first bool   // create on a blank (voice-only seed) app
	appID string // logged only as metrics.AppHash
}

// generateOutcome classifies one /v1/generate result.
func generateOutcome(res *Result, err error) string {
	if err != nil {
		var ge *Error
		if !errors.As(err, &ge) {
			return GenFailed
		}
		switch {
		case ge.Code == httpx.CodeRefused:
			return GenRefused
		case ge.Code == httpx.CodeGenerationFailed, ge.Code == httpx.CodeRateLimited: // rate_limited here = Claude's 429
			return GenFailed
		case ge.Status >= 400 && ge.Status < 500:
			return GenBadRequest
		}
		return GenFailed
	}
	switch {
	case res.Mode == "restore":
		return GenRestored
	case res.Unchanged:
		return GenUnchanged
	}
	m := res.Meta
	if m.Regenerations > 0 || m.AreaFallback || m.DowngradedToFull || m.LintRetried {
		return GenRegenerated
	}
	return GenValidated
}

// logGenerate writes one ai.generate line. Labels, counts and the uid hash
// only: never the transcript, summary, DSL, appId or raw uid.
func (s *Service) logGenerate(ctx context.Context, tr genTrace, stream bool, res *Result, err error, d time.Duration) {
	lg := s.logger()
	mode := tr.mode
	if mode == "" {
		mode = "unknown"
	}
	outcome := generateOutcome(res, err)
	passed := outcome == GenValidated || outcome == GenRegenerated || outcome == GenUnchanged
	attrs := []slog.Attr{
		slog.String("event", GenerateEvent),
		slog.String("mode", mode),
		slog.String("outcome", outcome),
		slog.Bool("validation_passed", passed),
		slog.Bool("first_generation", tr.first),
		slog.Bool("stream", stream),
		slog.Int64("duration_ms", d.Milliseconds()),
	}
	if res != nil {
		m := res.Meta
		attrs = append(attrs,
			slog.Int("attempts", m.Attempts),
			slog.Int("regenerations", m.Regenerations),
			slog.Bool("area_fallback", m.AreaFallback),
			slog.Bool("downgraded_to_full", m.DowngradedToFull),
			slog.Bool("lint_retried", m.LintRetried),
			slog.Int64("claude_ms", res.LatencyMs.Claude),
		)
	}
	var ge *Error
	if errors.As(err, &ge) {
		attrs = append(attrs, slog.String("error_code", ge.Code), slog.Int("attempts", ge.Attempts))
	} else if err != nil {
		attrs = append(attrs, slog.String("error_code", "internal"))
	}
	attrs = append(attrs, tallyOf(ctx).attrs()...) // tokens and cost of every attempt (#298)
	if h := UIDHash(auth.UID(ctx)); h != "" {
		attrs = append(attrs, slog.String("uid_hash", h))
	}
	if h := metrics.AppHash(tr.appID); h != "" { // per-app edit counts (#297)
		attrs = append(attrs, slog.String("app_hash", h))
	}
	level := slog.LevelInfo
	if outcome == GenFailed {
		level = slog.LevelWarn
	}
	lg.LogAttrs(ctx, level, GenerateEvent, attrs...)
}
