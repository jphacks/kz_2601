package notify

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"unicode/utf8"
)

// ErrFakeRejected is returned by FakeSender for a token given a failing Result.
var ErrFakeRejected = errors.New("notify: fake sender rejected the push")

// FakeSender records pushes (tests). Results maps a token to the Result to
// return; tokens not in it are accepted.
type FakeSender struct {
	mu      sync.Mutex
	Results map[string]Result
	pushes  []Push
}

// Send records p.
func (f *FakeSender) Send(_ context.Context, p Push) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushes = append(f.pushes, p)
	if r, ok := f.Results[p.Token]; ok {
		return r, ErrFakeRejected
	}
	return Result{Status: 200}, nil
}

// Pushes returns the recorded pushes.
func (f *FakeSender) Pushes() []Push {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Push(nil), f.pushes...)
}

// LogSender logs instead of sending (local development). It logs neither the
// text nor the token (#339: no content, no raw ids in logs).
type LogSender struct {
	Logger *slog.Logger // nil = slog.Default()
}

// Send logs the shape of p.
func (s LogSender) Send(ctx context.Context, p Push) (Result, error) {
	l := s.Logger
	if l == nil {
		l = slog.Default()
	}
	l.InfoContext(ctx, "notify: push (log only)",
		slog.String("app_id", p.AppID),
		slog.String("collapse_id", p.CollapseID),
		slog.Int("title_runes", utf8.RuneCountInString(p.Title)),
		slog.Int("body_runes", utf8.RuneCountInString(p.Body)))
	return Result{Status: 200}, nil
}

// MemoryTokens is a TokenReader over a map (tests).
type MemoryTokens map[string][]string

// Tokens returns m[uid].
func (m MemoryTokens) Tokens(_ context.Context, uid string) ([]string, error) {
	return append([]string(nil), m[uid]...), nil
}
