package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Refunds (#548). The middleware charges a request before it runs, then gives
// units back when the request ends:
//
//   - 502 / 503 / 504 (Claude 5xx, timeouts, appconfig or Claude unavailable):
//     everything charged comes back. Our failure, not the caller's.
//   - 2xx where the handler reported a cheaper mode than the one charged
//     (generate mode=auto charged as create, resolved to edit or undo):
//     the difference comes back.
//   - anything else (4xx, 429, 500) or a client that went away before the
//     end: nothing comes back. A disconnect may have cost a full Claude call.
//
// The handler reports how the request ended through the request context
// (ReportStatus / ReportMode), never through a response header, so nothing
// internal reaches the client. The X-RateLimit-Remaining header was already
// written before the handler ran, so it does not include the refund.

// refundTimeout bounds the refund write after the response is done.
const refundTimeout = 5 * time.Second

type outcomeKey struct{}

// outcome is filled by the handler and read by the middleware afterwards.
type outcome struct {
	mu     sync.Mutex
	status int    // set for SSE, whose HTTP status is always 200
	mode   string // generate: resolved mode create|edit|undo
}

func outcomeFrom(ctx context.Context) *outcome {
	o, _ := ctx.Value(outcomeKey{}).(*outcome)
	return o
}

// ReportStatus records the status a request really ended with when the HTTP
// status cannot tell (an `event: error` of an SSE stream answered 200).
// No-op outside the rate limiter.
func ReportStatus(ctx context.Context, status int) {
	if o := outcomeFrom(ctx); o != nil {
		o.mu.Lock()
		o.status = status
		o.mu.Unlock()
	}
}

// ReportMode records the generate mode a request resolved to (create, edit
// or undo), so a mode=auto request charged as create pays only what it did.
// No-op outside the rate limiter.
func ReportMode(ctx context.Context, mode string) {
	if o := outcomeFrom(ctx); o != nil {
		o.mu.Lock()
		o.mode = mode
		o.mu.Unlock()
	}
}

// refundUnits applies the rules above. charged is what the request paid,
// status the final status, mode the reported mode ("" = none) and gone
// whether the client disconnected before the end.
func (w Weights) refundUnits(charged, status int, mode string, gone bool) int {
	if charged <= 0 || gone {
		return 0
	}
	switch {
	case status == http.StatusBadGateway, status == http.StatusServiceUnavailable, status == http.StatusGatewayTimeout:
		return charged
	case status >= 200 && status < 300 && mode != "":
		return max(0, charged-w.modeWeight(mode))
	}
	return 0
}

// statusWriter remembers the status the handler wrote.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(status int) {
	if sw.status == 0 {
		sw.status = status
	}
	sw.ResponseWriter.WriteHeader(status)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	return sw.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush (SSE) through the wrapper.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

// Flush keeps `w.(http.Flusher)` working for the SSE handler.
func (sw *statusWriter) Flush() {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	_ = http.NewResponseController(sw.ResponseWriter).Flush()
}

// final prefers the status the handler reported over the HTTP one.
func (o *outcome) final(httpStatus int) (int, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	status := httpStatus
	if o.status != 0 {
		status = o.status
	}
	if status == 0 {
		status = http.StatusOK
	}
	return status, o.mode
}
