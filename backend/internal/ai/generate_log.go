package ai

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Retry reasons (Meta.RetryReasons, retry_reasons of ai.generate): why an
// attempt was rejected and the model asked again (or the edit fell back).
const (
	RetryValidation    = "validation"     // dsl validation problems (schema / semantic rules)
	RetryInvalidOutput = "invalid_output" // not JSON / not a PatchDocument / patch does not apply
	RetryTruncated     = "truncated"      // stopped at max_tokens
	RetryRestriction   = "restriction"    // a part this build cannot render (#245)
	RetryLint          = "lint"           // content warnings or omissions (LintRetry)
	RetryShare         = "share"          // share request not answered (#254 #259)
)

// retryReason labels an attempt rejected with err.
func retryReason(err error, resp *Response) string {
	if resp != nil && resp.StopReason == "max_tokens" {
		return RetryTruncated
	}
	var ve *dsl.ValidationError
	if errors.As(err, &ve) && !patchOnly(ve.Codes()) {
		return RetryValidation
	}
	return RetryInvalidOutput
}

// patchOnly reports whether every code is a patch_* code: the patch itself
// was bad or did not apply, so the DSL was never validated.
func patchOnly(codes []string) bool {
	for _, c := range codes {
		if !strings.HasPrefix(c, "patch_") {
			return false
		}
	}
	return len(codes) > 0
}

// noteRejected records why an attempt was rejected with err: its retry
// reason and, for a dsl.ValidationError (patch_* included), the code of each
// problem (#1772).
func (m *Meta) noteRejected(err error, resp *Response) {
	r := retryReason(err, resp)
	m.RetryReasons = append(m.RetryReasons, r)
	var ve *dsl.ValidationError
	if r != RetryTruncated && errors.As(err, &ve) {
		m.ValidationCodes = append(m.ValidationCodes, ve.Codes()...)
	}
}

// codeCounts renders codes as "code:count,..." sorted by code
// (validation_codes of ai.generate).
func codeCounts(codes []string) string {
	n := map[string]int{}
	for _, c := range codes {
		n[c]++
	}
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ":" + strconv.Itoa(n[k])
	}
	return strings.Join(parts, ",")
}

// contentRetryReasons labels a valid output retried for its content.
func contentRetryReasons(lint, share bool) []string {
	var r []string
	if lint {
		r = append(r, RetryLint)
	}
	if share {
		r = append(r, RetryShare)
	}
	return r
}

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
		if len(m.RetryReasons) > 0 { // labels only, in order (e.g. "validation,validation")
			attrs = append(attrs, slog.String("retry_reasons", strings.Join(m.RetryReasons, ",")))
		}
		if len(m.ValidationCodes) > 0 { // dsl problem codes and counts only (#1772)
			attrs = append(attrs, slog.String("validation_codes", codeCounts(m.ValidationCodes)))
		}
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
