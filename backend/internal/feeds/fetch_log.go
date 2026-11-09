package feeds

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"
)

// FetchEvent is the message (and `event` attr) of the one line written per
// outbound fetch, success or failure (#348). Cache hits do not go out and
// write no line. Cloud Logging filter: jsonPayload.event="feeds.fetch".
const FetchEvent = "feeds.fetch"

// Errors of one outbound GET, for FetchStatus.
var (
	ErrTooLarge        = errors.New("feeds: response body too large")
	ErrBadResponse     = errors.New("feeds: response could not be read")
	ErrRedirectBlocked = errors.New("feeds: redirect not allowed")
)

// StatusError is a non-200 answer.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return "status " + strconv.Itoa(e.Code) }

// FetchStatus is the content-free `status` of a fetch line: "ok", the HTTP
// status code, or an error class. Never log the error text itself: a
// *url.Error carries the URL with its query (search words, ISBN).
func FetchStatus(err error) string {
	var se *StatusError
	var ne net.Error
	var ue *url.Error
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &se):
		return strconv.Itoa(se.Code)
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case errors.Is(err, ErrTooLarge):
		return "too_large"
	case errors.Is(err, ErrBadResponse):
		return "bad_response"
	case errors.Is(err, ErrRedirectBlocked):
		return "redirect_blocked"
	case errors.Is(err, ErrBlockedAddress):
		return "blocked_address"
	case errors.Is(err, ErrUnknown):
		return "not_allowed"
	case errors.As(err, &ue), errors.As(err, &ne):
		return "network"
	}
	return "error"
}

// FetchLine is one feeds.fetch line. Labels and counts only: never the
// search words, the URL, an ISBN, a uid or the body.
type FetchLine struct {
	Provider string
	Err      error
	Bytes    int
	Duration time.Duration
	Attrs    []slog.Attr // extra content-free labels (allowlisted ids, counts)
}

// LogFetch writes l to lg (slog.Default() when nil): Info when it worked,
// Warn when not.
func LogFetch(ctx context.Context, lg *slog.Logger, l FetchLine) {
	if lg == nil {
		lg = slog.Default()
	}
	level := slog.LevelInfo
	if l.Err != nil {
		level = slog.LevelWarn
	}
	attrs := append([]slog.Attr{
		slog.String("event", FetchEvent),
		slog.String("provider", l.Provider),
		slog.String("status", FetchStatus(l.Err)),
		slog.Int("bytes", l.Bytes),
		slog.Int64("duration_ms", l.Duration.Milliseconds()),
	}, l.Attrs...)
	lg.LogAttrs(ctx, level, FetchEvent, attrs...)
}
