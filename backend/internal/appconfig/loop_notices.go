package appconfig

// 「あなたの声で直りました」 (#374): the caller's notices from the
// self-improvement loop (loop_notices, written by cmd/feedback-loop fixed)
// and the push consent for them. Only the caller's own: there is no uid in
// the paths and another user's notice id answers 404 like a missing one.
// docs/backend/features/feedback-loop.md

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// routes (#374)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.notices == nil {
			h.notices = defaultNotices(h.store)
		}
		mux.HandleFunc("GET /v1/me/notices", h.listLoopNotices)
		mux.HandleFunc("POST /v1/me/notices/{noticeId}/read", h.readLoopNotice)
		// more specific than /v1/me/notify-consent/{appId}, so it wins for this path
		mux.HandleFunc("GET /v1/me/notify-consent/feedback-fixed", h.getFeedbackFixedConsent)
		mux.HandleFunc("PUT /v1/me/notify-consent/feedback-fixed", h.putFeedbackFixedConsent)
	})
}

// maxLoopNotices is how many of the caller's notices are read per request
// (one per fixed issue they wrote about, kept 90 days).
const maxLoopNotices = 500

var noticeIDRe = regexp.MustCompile(`^ntc_[0-9a-f]{24}$`)

// WithLoopNotices sets where the loop's notices are read (tests). The default
// is loop_notices next to the store (Firestore) or an empty in-memory one.
func WithLoopNotices(n loop.NoticeReader) Option {
	return func(h *Handler) { h.notices = n }
}

func defaultNotices(s store.Store) loop.NoticeReader {
	if fs, ok := s.(*store.Firestore); ok {
		return loop.NewFirestore(fs.Client())
	}
	return loop.NewMemory()
}

type loopNoticeOut struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}

func noticeOut(n loop.Notice) loopNoticeOut {
	o := loopNoticeOut{ID: n.ID, Title: n.Title, Body: n.Body, CreatedAt: n.CreatedAt}
	if !n.ReadAt.IsZero() {
		at := n.ReadAt
		o.ReadAt = &at
	}
	return o
}

// listLoopNotices: GET /v1/me/notices?pageSize&pageToken. Newest first;
// expired ones (waiting for purge) are left out. unreadCount covers all pages.
func (h *Handler) listLoopNotices(w http.ResponseWriter, r *http.Request) {
	p, err := parsePaging(r)
	if err != nil {
		fail(w, err)
		return
	}
	all, err := h.notices.ListNotices(r.Context(), uidOf(r), maxLoopNotices)
	if err != nil {
		fail(w, fmt.Errorf("list notices: %w", err))
		return
	}
	now := time.Now()
	live := all[:0]
	unread := 0
	for _, n := range all {
		if !n.ExpiresAt.IsZero() && n.ExpiresAt.Before(now) {
			continue
		}
		if n.ReadAt.IsZero() {
			unread++
		}
		live = append(live, n)
	}
	items := []loopNoticeOut{}
	for i := p.offset; i < len(live) && i < p.offset+p.size; i++ {
		items = append(items, noticeOut(live[i]))
	}
	resp := map[string]any{"items": items, "unreadCount": unread}
	if nt := p.next(max(0, len(live)-p.offset)); nt != "" {
		resp["nextPageToken"] = nt
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// readLoopNotice: POST /v1/me/notices/{noticeId}/read. Idempotent (the first
// readAt stays). 404 for a missing notice and for someone else's.
func (h *Handler) readLoopNotice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("noticeId")
	if !noticeIDRe.MatchString(id) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid noticeId"))
		return
	}
	n, err := h.notices.MarkNoticeRead(r.Context(), uidOf(r), id, time.Now().UTC())
	if errors.Is(err, loop.ErrNotFound) {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "notice not found"))
		return
	}
	if err != nil {
		fail(w, fmt.Errorf("mark notice read: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, noticeOut(*n))
}

type feedbackFixedConsentOut struct {
	Enabled   bool       `json:"enabled"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

// getFeedbackFixedConsent: GET /v1/me/notify-consent/feedback-fixed. No
// stored consent = not enabled.
func (h *Handler) getFeedbackFixedConsent(w http.ResponseWriter, r *http.Request) {
	ok, err := h.store.NotifyAllowed(r.Context(), uidOf(r), loop.FeedbackFixedConsentID)
	if err != nil {
		fail(w, fmt.Errorf("feedback-fixed consent: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, feedbackFixedConsentOut{Enabled: ok})
}

// putFeedbackFixedConsent: PUT /v1/me/notify-consent/feedback-fixed {enabled}.
// Whether cmd/feedback-loop fixed may also push 「直りました」 (LOOP_NOTIFY_PUSH,
// at most once a week). Stored where it reads it: users/{uid}/notifyConsent/_feedback_fixed.
func (h *Handler) putFeedbackFixedConsent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	if in.Enabled == nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "enabled is required").With("field", "enabled"))
		return
	}
	c, err := h.store.SetNotifyConsent(r.Context(), uidOf(r), loop.FeedbackFixedConsentID, *in.Enabled)
	if err != nil {
		fail(w, fmt.Errorf("set feedback-fixed consent: %w", err))
		return
	}
	slog.Info("appconfig: feedback-fixed consent set", slog.Bool("enabled", c.Allowed))
	at := c.UpdatedAt
	httpx.WriteJSON(w, http.StatusOK, feedbackFixedConsentOut{Enabled: c.Allowed, UpdatedAt: &at})
}
