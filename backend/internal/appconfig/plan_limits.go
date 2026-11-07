package appconfig

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Plan limits (#317, internal/plan, docs/backend/rate-limit.md): apps per
// user, records and photos per app. They count against the payer's plan: the
// caller for its own apps, the creator for an app someone else made.
// Over the limit: 403 plan_limit_reached (details: resource, plan, limit,
// used, upgrade; reason creator_plan_limit when the payer is the creator).
//
// Without WithPlanLimits (tests, tools), or when the store cannot count,
// nothing is checked. A failed count lets the write through (logged): the
// limits are a guard against runaway use, not worth failing a save over.

// reasonCreatorPlanLimit is details.reason when the plan that ran out is the
// app creator's, not the caller's.
const reasonCreatorPlanLimit = "creator_plan_limit"

// WithPlanLimits enforces t (plan.FromEnv). The plan is Pro when the Pro
// checker of WithPro says so.
func WithPlanLimits(t plan.Table) Option {
	return func(h *Handler) { h.planLimits = &t }
}

// planCounter is the store's Counter when the limits are on.
func (h *Handler) planCounter() (store.Counter, bool) {
	if h.planLimits == nil || !h.planLimits.Mode.Checks() {
		return nil, false
	}
	c, ok := h.store.(store.Counter)
	return c, ok
}

func (h *Handler) planOf(ctx context.Context, uid string) plan.Plan {
	if h.pro != nil && uid != "" && h.pro.IsPro(ctx, uid) {
		return plan.Pro
	}
	return plan.Free
}

// checkAppLimit runs before the caller creates a new app of its own.
func (h *Handler) checkAppLimit(r *http.Request) error {
	c, ok := h.planCounter()
	if !ok {
		return nil
	}
	uid := uidOf(r)
	n, err := c.CountApps(r.Context(), uid)
	if err != nil {
		slog.WarnContext(r.Context(), "appconfig: plan limit count failed", slog.String("resource", plan.Apps), slog.Any("error", err))
		return nil
	}
	return h.planStop(r, uid, uid, plan.Apps, n, 1)
}

// checkRecordLimits runs before a record write of ent in cur. data is the
// written data (the delta of a patch); prev is the record's data before the
// write (nil for a new one). A new record counts against recordsPerApp
// (not on a single entity: one record per user), each photo field that goes
// from empty to set against photosPerApp.
func (h *Handler) checkRecordLimits(r *http.Request, cur *store.App, ent *dsl.Entity, data, prev map[string]any, isNew bool) error {
	c, ok := h.planCounter()
	if !ok {
		return nil
	}
	newRecord := isNew && !ent.Single
	photos := 0
	for i := range ent.Fields {
		if f := &ent.Fields[i]; f.Type == dsl.FieldPhoto && data[f.ID] != nil && prev[f.ID] == nil {
			photos++
		}
	}
	if !newRecord && photos == 0 {
		return nil
	}
	ctx := r.Context()
	app, err := h.parsedDSL(uidOf(r), cur)
	if err != nil {
		return err
	}
	payer, err := h.appOwner(ctx, cur.ID)
	if err != nil {
		return err
	}
	if payer == "" {
		payer = uidOf(r)
	}
	records, photoCount, err := countAppRecords(r, c, cur, app, ent, photos > 0)
	if err != nil {
		slog.WarnContext(ctx, "appconfig: plan limit count failed", slog.String("resource", plan.RecordsPerApp), slog.Any("error", err))
		return nil
	}
	if newRecord {
		if err := h.planStop(r, payer, uidOf(r), plan.RecordsPerApp, records, 1); err != nil {
			return err
		}
	}
	if photos > 0 {
		return h.planStop(r, payer, uidOf(r), plan.PhotosPerApp, photoCount, photos)
	}
	return nil
}

// countAppRecords sums, over every entity of app, the records the caller
// sees (its own and the shared ones) and, when withPhotos, their set photo
// fields. target replaces its entry of app (its sharing is resolved).
func countAppRecords(r *http.Request, c store.Counter, cur *store.App, app *dsl.App, target *dsl.Entity, withPhotos bool) (records, photos int, err error) {
	ctx := r.Context()
	for i := range app.Entities {
		e := &app.Entities[i]
		if e.ID == target.ID {
			e = target
		}
		rs := recordSet(r, cur, e, e.ID)
		n, err := c.CountRecords(ctx, rs, "")
		if err != nil {
			return 0, 0, err
		}
		records += n
		if !withPhotos || n == 0 {
			continue
		}
		for j := range e.Fields {
			if e.Fields[j].Type != dsl.FieldPhoto {
				continue
			}
			p, err := c.CountRecords(ctx, rs, e.Fields[j].ID)
			if err != nil {
				return 0, 0, err
			}
			photos += p
		}
	}
	return records, photos, nil
}

// planStop answers 403 plan_limit_reached when adding more resources to
// used goes over payer's plan (and PLAN_LIMITS_MODE enforces).
func (h *Handler) planStop(r *http.Request, payer, caller, resource string, used, adding int) error {
	t := *h.planLimits
	p := h.planOf(r.Context(), payer)
	if t.Allows(p, resource, used+adding-1) {
		return nil
	}
	limit := t.For(p).Of(resource)
	if !t.Mode.Stop(r.Context(), p, resource, payer, used, limit) {
		return nil
	}
	pro := t.Pro.Of(resource)
	upgrade := p == plan.Free && (pro <= 0 || pro > limit)
	byCreator := payer != caller
	e := apiErr(http.StatusForbidden, httpx.CodePlanLimitReached, plan.Message(resource, limit, upgrade, byCreator)).
		With("resource", resource).
		With("plan", string(p)).
		With("limit", limit).
		With("used", used).
		With("upgrade", upgrade)
	if byCreator {
		e = e.With("reason", reasonCreatorPlanLimit)
	}
	return e
}
