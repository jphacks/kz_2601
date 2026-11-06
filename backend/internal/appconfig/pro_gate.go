package appconfig

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Pro features (#316, docs/backend/features/pro.md). The list is
// internal/paid/paid_features.json.
//
//   - Saving a DSL (PUT / PATCH /dsl, restore, POST /apps with a dsl,
//     duplicate) that adds a save-gated paid feature (shared data, server
//     rules) needs the creator to have Pro: 402 pro_required otherwise. An
//     app that already has the feature keeps it (a lapsed Pro opens, reads
//     and edits it; only new paid features are refused). A participant's copy
//     of someone else's app is not checked: the creator pays, and the copy's
//     own private rules are stopped by the runner.
//   - Writing a shared record (create / patch, a mark) needs the app's
//     creator to have Pro. Reading, and deleting one's own records, stay open.
//
// Without a checker (WithPro not given: tests, tools) nothing is checked.
// PRO_GATE_MODE (WithProMode): enforce (default) / log (only
// pro_gate.would_block is logged) / off.

// reasonCreatorNeedsPro is details.reason when the caller is not the one
// who has to buy (a participant of a shared app whose creator has no Pro).
const reasonCreatorNeedsPro = "creator_needs_pro"

// Option configures New.
type Option func(h *Handler)

// WithPro enforces the paid features with p (entitlement.ProChecker).
func WithPro(p paid.ProChecker) Option {
	return func(h *Handler) { h.pro = p }
}

// WithProMode sets PRO_GATE_MODE (paid.ModeFromEnv).
func WithProMode(m paid.Mode) Option {
	return func(h *Handler) { h.proMode = m }
}

// gateOn reports whether the paid features are looked at.
func (h *Handler) gateOn() bool { return h.pro != nil && h.proMode.Checks() }

// lang picks the language of details.features from Accept-Language.
func lang(r *http.Request) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept-Language"))), "en") {
		return "en"
	}
	return "ja"
}

func proRequired(features []paid.Feature) *apiError {
	return apiErr(http.StatusPaymentRequired, httpx.CodeProRequired,
		"this needs Tailor It Pro: "+strings.Join(paid.IDs(features), ", ")).With("features", features)
}

// checkPaidSave refuses next when it adds paid features to prev (nil = a new
// app) and the caller, the app's creator, has no Pro.
func (h *Handler) checkPaidSave(r *http.Request, appID string, next, prev *dsl.App) error {
	if !h.gateOn() {
		return nil
	}
	added := paid.Default.Added(next, prev, lang(r))
	if len(added) == 0 {
		return nil
	}
	owner, err := h.appOwner(r.Context(), appID)
	if err != nil {
		return err
	}
	uid := uidOf(r)
	if owner != "" && owner != uid {
		return nil // a participant's copy: never published, its rules run as the participant
	}
	if h.pro.IsPro(r.Context(), uid) || !h.proMode.Stop(r.Context(), nil, "appconfig.dsl_save", uid, paid.IDs(added)) {
		return nil
	}
	return proRequired(added)
}

// checkPaidSaveRaw is checkPaidSave with the stored DSL of prev (nil = none).
func (h *Handler) checkPaidSaveRaw(r *http.Request, appID string, next *dsl.App, prev *store.App) error {
	if !h.gateOn() {
		return nil
	}
	var old *dsl.App
	if prev != nil {
		a, err := decodeStored(prev.DSL)
		if err != nil {
			return err
		}
		old = a
	}
	return h.checkPaidSave(r, appID, next, old)
}

// checkSharedWrite refuses a write to a shared record when the app's creator
// has no Pro (the creator pays for the app's shared data).
func (h *Handler) checkSharedWrite(r *http.Request, cur *store.App, ent *dsl.Entity) error {
	if !h.gateOn() || !ent.IsShared() {
		return nil
	}
	owner, err := h.appOwner(r.Context(), cur.ID)
	if err != nil {
		return err
	}
	if owner == "" || h.pro.IsPro(r.Context(), owner) || !h.proMode.Stop(r.Context(), nil, "appconfig.shared_write", owner, []string{paid.SharedData}) {
		return nil
	}
	f := paid.Feature{ID: paid.SharedData}
	if d, ok := paid.Default.ByID(paid.SharedData); ok {
		f = d.Public(lang(r))
	}
	e := proRequired([]paid.Feature{f})
	if owner != uidOf(r) {
		e = e.With("reason", reasonCreatorNeedsPro)
	}
	return e
}

// decodeStored decodes a stored DSL without validating it (an older version
// today's rules reject still tells which features it had).
func decodeStored(raw json.RawMessage) (*dsl.App, error) {
	a := &dsl.App{}
	if err := json.Unmarshal(raw, a); err != nil {
		return nil, err
	}
	return a, nil
}
