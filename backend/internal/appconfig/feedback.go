package appconfig

// In-app feedback (#319): POST /v1/feedback stores feedback/{id}. Every
// attachment is opt-in on the device; record contents are never accepted.
// The DSL attachment is a reference (appId + dslVersion): the snapshot is
// read from the caller's own tree, so it is exactly the stored structure and
// another user's app cannot be attached. Retention and erasure: store.FeedbackRetention
// and cmd/purge (architecture.md §3.5).

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// routes (#319)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		h.feedbackPerDay = feedbackPerDay()
		mux.HandleFunc("POST /v1/feedback", h.createFeedback)
	})
}

// Feedback limits.
const (
	maxFeedbackMessage    = 2000      // runes
	maxFeedbackScreenshot = 512 << 10 // decoded JPEG bytes (Firestore documents stop at 1 MiB)
	maxFeedbackDSL        = 256 << 10 // snapshot bytes; larger DSLs are not attached
	maxFeedbackDeviceText = 64        // runes per device field
	maxFeedbackConsent    = 64        // runes
)

var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

type feedbackIn struct {
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	ConsentVersion string `json:"consentVersion"`
	Attachments    *struct {
		Screenshot string `json:"screenshot"` // base64 JPEG
		App        *struct {
			AppID      string `json:"appId"`
			DSLVersion int    `json:"dslVersion"`
		} `json:"app"`
		Device *store.FeedbackDevice `json:"device"`
	} `json:"attachments"`
}

type feedbackOut struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
	Attachments map[string]bool `json:"attachments"`
}

// feedbackPerDay reads FEEDBACK_DAILY_LIMIT (unset/invalid = default, 0 = off).
func feedbackPerDay() int {
	if n, err := strconv.Atoi(os.Getenv("FEEDBACK_DAILY_LIMIT")); err == nil && n >= 0 {
		return n
	}
	return store.MaxFeedbackPerDay
}

func badFeedback(field, msg string) error {
	return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, msg).With("field", field)
}

// createFeedback: POST /v1/feedback.
func (h *Handler) createFeedback(w http.ResponseWriter, r *http.Request) {
	var in feedbackIn
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	fb, err := h.feedbackFrom(r, in)
	if err != nil {
		fail(w, err)
		return
	}
	out, err := h.store.CreateFeedback(r.Context(), *fb, h.feedbackPerDay)
	if errors.Is(err, store.ErrFeedbackQuota) {
		writeFeedbackLimited(w, h.feedbackPerDay, time.Now())
		return
	}
	if err != nil {
		fail(w, fmt.Errorf("create feedback: %w", err))
		return
	}
	// never log the message or attachments
	slog.Info("appconfig: feedback received", slog.String("feedbackId", out.ID), slog.Any("attachments", fb.Attachments()))
	httpx.WriteJSON(w, http.StatusCreated, feedbackOut{ID: out.ID, Status: out.Status, CreatedAt: out.CreatedAt,
		Attachments: map[string]bool{"screenshot": len(fb.Screenshot) > 0, "dsl": len(fb.DSL) > 0, "device": fb.Device != nil}})
}

func (h *Handler) feedbackFrom(r *http.Request, in feedbackIn) (*store.Feedback, error) {
	msg := strings.TrimSpace(in.Message)
	consent := strings.TrimSpace(in.ConsentVersion)
	switch {
	case msg == "":
		return nil, badFeedback("message", "message is required")
	case tooLong(msg, maxFeedbackMessage):
		return nil, badFeedback("message", fmt.Sprintf("message must be at most %d characters", maxFeedbackMessage))
	case consent == "":
		return nil, badFeedback("consentVersion", "consentVersion is required (the user must agree before sending)")
	case tooLong(consent, maxFeedbackConsent):
		return nil, badFeedback("consentVersion", "consentVersion is too long")
	}
	fb := &store.Feedback{UID: uidOf(r), Kind: store.FeedbackOther, Message: msg, ConsentVersion: consent}
	switch in.Kind {
	case store.FeedbackBug, store.FeedbackIdea:
		fb.Kind = in.Kind
	}
	a := in.Attachments
	if a == nil {
		return fb, nil
	}
	if a.Screenshot != "" {
		if base64.StdEncoding.DecodedLen(len(a.Screenshot)) > maxFeedbackScreenshot+3 {
			return nil, apiErr(http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge,
				fmt.Sprintf("screenshot must be at most %d bytes", maxFeedbackScreenshot)).With("field", "attachments.screenshot")
		}
		img, err := base64.StdEncoding.DecodeString(a.Screenshot)
		switch {
		case err != nil:
			return nil, badFeedback("attachments.screenshot", "screenshot must be base64")
		case len(img) > maxFeedbackScreenshot:
			return nil, apiErr(http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge,
				fmt.Sprintf("screenshot must be at most %d bytes", maxFeedbackScreenshot)).With("field", "attachments.screenshot")
		case !bytes.HasPrefix(img, jpegMagic):
			return nil, badFeedback("attachments.screenshot", "screenshot must be a JPEG")
		}
		fb.Screenshot = img
	}
	if d := a.Device; d != nil {
		dev := store.FeedbackDevice{AppVersion: strings.TrimSpace(d.AppVersion), Build: strings.TrimSpace(d.Build),
			OS: strings.TrimSpace(d.OS), Model: strings.TrimSpace(d.Model), Locale: strings.TrimSpace(d.Locale)}
		for _, v := range []string{dev.AppVersion, dev.Build, dev.OS, dev.Model, dev.Locale} {
			if tooLong(v, maxFeedbackDeviceText) {
				return nil, badFeedback("attachments.device", fmt.Sprintf("device fields must be at most %d characters", maxFeedbackDeviceText))
			}
		}
		fb.Device = &dev
	}
	if app := a.App; app != nil {
		if !idRe.MatchString(app.AppID) || !validAppID(app.AppID) || app.DSLVersion < 1 {
			return nil, badFeedback("attachments.app", "appId and dslVersion (>= 1) are required")
		}
		v, err := h.store.GetVersion(r.Context(), uidOf(r), app.AppID, app.DSLVersion)
		switch {
		case errors.Is(err, store.ErrNotFound):
			// not on the server (offline / mock app): send the rest without it
		case err != nil:
			return nil, fmt.Errorf("feedback dsl: %w", err)
		case len(v.DSL) > maxFeedbackDSL:
			slog.Warn("appconfig: feedback dsl too large; not attached", slog.Int("bytes", len(v.DSL)))
		default:
			fb.AppID, fb.DSLVersion, fb.DSL = app.AppID, app.DSLVersion, compact(v.DSL)
		}
	}
	return fb, nil
}

// writeFeedbackLimited answers 429 rate_limited until the next JST midnight
// (same envelope as the ai limiter, so iOS reads it as RateLimitInfo).
func writeFeedbackLimited(w http.ResponseWriter, limit int, now time.Time) {
	jst := now.In(time.FixedZone("JST", 9*60*60))
	y, m, d := jst.Date()
	reset := time.Date(y, m, d+1, 0, 0, 0, 0, jst.Location())
	wait := max(1, int(reset.Sub(now).Seconds()+0.999))
	w.Header().Set("Retry-After", strconv.Itoa(wait))
	httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited,
		fmt.Sprintf("フィードバックは 1 日 %d 件までです。明日また送れます。", limit), map[string]any{
			"scope": "user", "limit": limit, "used": limit, "remaining": 0,
			"resetAt": reset.UTC().Format(time.RFC3339), "retryAfterSec": wait,
		})
}
