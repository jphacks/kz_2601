package ai

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// 「無い機能を頼まれた」の印 (#1769 / self-improvement loop): the prompt makes
// the model declare, in the summary, every part of a request it replaced or
// could not do ("○○は△△で代替しました。" / "○○は今はできません。" / "X was
// replaced with Y."; prompt.go 依頼の点検, omission.go adds a fixed sentence
// when it stayed silent). That declaration is the one reliable signal this
// service has that a user wanted something the DSL cannot do, so each such
// sentence becomes one ai.unsupported log line. The loop's ingest reads them
// (internal/loop/ingest, LOOP_SOURCE_UNSUPPORTED) and turns a day's worth of
// the same phrase into a feature-request inbox item.
//
// What is logged: the requested phrase and the substitute as the model wrote
// them, masked with pii.Mask and cut short, plus the whole masked sentence
// (note), the mode, locale, platform and the app hash. Never the transcript,
// the rest of the summary, the DSL, the app id or the raw uid.

// UnsupportedEvent is the event (and message) of the per-sentence line.
const UnsupportedEvent = "ai.unsupported"

// maxUnsupportedRunes bounds each logged phrase.
const maxUnsupportedRunes = 160

// UnsupportedNote is one parsed declaration.
type UnsupportedNote struct {
	// Requested is what the user asked for ("カレンダー表示", "a tournament bracket").
	Requested string
	// Substitute is what stood in for it ("日付順の一覧"), "" when nothing did.
	Substitute string
	// Note is the whole sentence.
	Note string
}

var (
	// the clause that names the missing part, in the shapes the prompt dictates
	reUnsupportedTailJA = regexp.MustCompile(`(?:は|が|を)?(?:今は|この版では|このアプリでは)?(?:使えない|使えません|作れない|作れません|組めなかった|組めない|できない|できません|出来ない|出来ません|無い|ない|ありません)(?:ため|ので|から)?$`)
	reUnsupportedTailEN = regexp.MustCompile(`(?i)\s+(?:isn't|is not|aren't|are not|can't|cannot|can not|couldn't|could not|wasn't|weren't)\b.*$`)
	reReplacedWithEN    = regexp.MustCompile(`(?i)^(.+?)\s+(?:was|were|is|are|got|has been|have been)\s+replaced\s+(?:with|by)\s+(.+?)[.。]?$`)
	// shapes some prompt rules dictate that honestRe (omission.go) does not list
	reUnsupportedExtra = regexp.MustCompile(`作れない|作れません|組めなかった|使えない|使えません|(?i:aren't possible|aren't supported|couldn't be|could not be|can't be built)`)
)

// unsupportedSentences are the sentences of summary that declare a missing
// or replaced part (honestRe plus reUnsupportedExtra).
func unsupportedSentences(summary, sep string) []string {
	var out []string
	for _, sentence := range strings.SplitAfter(summary, sep) {
		if s := strings.TrimSpace(sentence); s != "" && (honestRe.MatchString(s) || reUnsupportedExtra.MatchString(s)) {
			out = append(out, s)
		}
	}
	return out
}

// ParseUnsupported extracts the declarations from a summary written for
// locale (ja splits on 。, en on .). Sentences without one are ignored.
func ParseUnsupported(summary string, locale Locale) []UnsupportedNote {
	sep := "。"
	if locale == LocaleEN {
		sep = "."
	}
	var out []UnsupportedNote
	for _, s := range unsupportedSentences(summary, sep) {
		n := parseUnsupportedSentence(strings.TrimSpace(s), locale)
		if n.Requested == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

func parseUnsupportedSentence(s string, locale Locale) UnsupportedNote {
	n := UnsupportedNote{Note: s}
	if m := reReplacedWithEN.FindStringSubmatch(s); m != nil {
		n.Requested, n.Substitute = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		return n
	}
	if locale == LocaleEN {
		req := reUnsupportedTailEN.ReplaceAllString(strings.TrimSuffix(s, "."), "")
		if req != strings.TrimSuffix(s, ".") {
			n.Requested = strings.TrimSpace(req)
		}
		return n
	}
	// ja: "<requested>は<substitute>で代替しました" or
	// "<requested>は今はできないため、<substitute>で代替しました" or
	// "<requested>は今はできません" / "<requested>は入れていません"
	body := strings.TrimSuffix(s, "。")
	if i := strings.Index(body, "で代替"); i >= 0 {
		left := body[:i]
		if j := strings.LastIndex(left, "、"); j >= 0 {
			n.Substitute = strings.TrimSpace(left[j+len("、"):])
			left = left[:j]
		} else if j := strings.LastIndex(left, "は"); j >= 0 {
			n.Substitute = strings.TrimSpace(left[j+len("は"):])
			left = left[:j]
		}
		n.Requested = unsupportedRequestedJA(left)
		return n
	}
	clause := body
	if j := strings.Index(clause, "、"); j >= 0 {
		clause = clause[:j]
	}
	n.Requested = unsupportedRequestedJA(clause)
	return n
}

// unsupportedRequestedJA strips the "は今はできないため" tail of a clause.
func unsupportedRequestedJA(clause string) string {
	clause = strings.TrimSpace(clause)
	req := reUnsupportedTailJA.ReplaceAllString(clause, "")
	if req == clause {
		// "○○は入れていません" / "○○は省きました": keep what is before the particle
		for _, p := range []string{"は入れて", "は省き", "は押せ", "は"} {
			if j := strings.LastIndex(clause, p); j > 0 {
				req = clause[:j]
				break
			}
		}
	}
	return strings.TrimSpace(req)
}

// maskUnsupported masks and cuts one logged phrase.
func maskUnsupported(s string) string {
	s = strings.TrimSpace(pii.Mask(s).Text)
	if utf8.RuneCountInString(s) > maxUnsupportedRunes {
		s = string([]rune(s)[:maxUnsupportedRunes])
	}
	return s
}

// logUnsupported writes one ai.unsupported line per declaration in res.
// Nothing is written for a failed, restored or unchanged result.
func (s *Service) logUnsupported(ctx context.Context, tr genTrace, res *Result) {
	if res == nil || res.Mode == "restore" || res.Unchanged {
		return
	}
	notes := ParseUnsupported(res.Summary, res.Locale)
	if len(notes) == 0 {
		return
	}
	platform := clientPlatformOf(ctx)
	if platform == "" {
		platform = "unknown"
	}
	mode := tr.mode
	if mode == "" {
		mode = "unknown"
	}
	lg := s.logger()
	for _, n := range notes {
		attrs := []slog.Attr{
			slog.String("event", UnsupportedEvent),
			slog.String("mode", mode),
			slog.String("locale", string(res.Locale)),
			slog.String("platform", platform),
			slog.String("requested", maskUnsupported(n.Requested)),
			slog.String("substitute", maskUnsupported(n.Substitute)),
			slog.String("note", maskUnsupported(n.Note)),
		}
		if h := metrics.AppHash(tr.appID); h != "" {
			attrs = append(attrs, slog.String("app_hash", h))
		}
		lg.LogAttrs(ctx, slog.LevelInfo, UnsupportedEvent, attrs...)
	}
}
