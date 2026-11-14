package appconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/plan"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Held apps and app slots (#630, owner decision 2026-11-10,
// docs/backend/rate-limit.md "持てるアプリの数"). A user holds the apps in
// their list: the ones they created and the shared ones they joined
// (store.Counter.CountApps: past version 1). The limit is the plan's
// PLAN_*_APPS plus the slots bought with coins (coins.Wallet.AppSlots).
//
// Two checks, both under PLAN_LIMITS_MODE (log: plan_limit.would_block only):
//   - checkAppLimit, before a new app is made (the first GET /dsl of a new
//     appId, POST /apps, duplicate, installing / joining from a link or the
//     catalog): a count, so iOS is told before the AI runs. It is outside
//     any transaction and only an early answer, never the guard.
//   - the commit that makes a blank app held (h.commit, store.Commit.HeldLimit):
//     counted in the same transaction, so two requests at once cannot both
//     take the last slot. Generating a blank and duplicating / installing /
//     joining (seedCopy: a blank at version 1, then this commit) all end here.
//
// Over the limit: 402 app_limit_reached with details {resource: apps, plan,
// held, limit, base, purchasedSlots, canBuySlot, slotCoins, upgrade, apps:
// [{id, name, mine, updatedAt}]} so iOS can offer to replace one (delete an
// own app / leave a joined one) or buy a slot.

// AppSlotSource is the coin wallet's slots (coins.Service).
type AppSlotSource interface {
	AppSlots(ctx context.Context, uid string) (coins.SlotInfo, error)
}

// WithAppSlots adds the slots of s to the held-app limit.
func WithAppSlots(s AppSlotSource) Option {
	return func(h *Handler) { h.slots = s }
}

// maxHeldList bounds details.apps.
const maxHeldList = 100

// heldLimit is the caller's held-app limit.
type heldLimit struct {
	plan plan.Plan
	// base is the plan's PLAN_*_APPS (0 = not counted).
	base      int
	purchased int
	// price of one slot when one can be bought now (0 = cannot).
	price   int64
	balance int64
}

func (l heldLimit) limit() int { return l.base + l.purchased }

// counted: there is a limit (the plan counts apps).
func (l heldLimit) counted() bool { return l.base > 0 }

func (h *Handler) heldLimitOf(ctx context.Context, uid string) heldLimit {
	t := *h.planLimits
	p := h.planOf(ctx, uid)
	l := heldLimit{plan: p, base: t.For(p).Apps}
	if h.slots == nil || !l.counted() {
		return l
	}
	s, err := h.slots.AppSlots(ctx, uid)
	if err != nil { // the plan's base still holds; a failed wallet read never lifts it
		slog.WarnContext(ctx, "appconfig: app slots read failed", slog.Any("error", err))
		return l
	}
	l.purchased, l.price, l.balance = s.Purchased, s.Price, s.Available
	return l
}

// checkAppLimit runs before a new app is put into the caller's list.
func (h *Handler) checkAppLimit(r *http.Request) error {
	c, ok := h.planCounter()
	if !ok {
		return nil
	}
	ctx, uid := r.Context(), uidOf(r)
	l := h.heldLimitOf(ctx, uid)
	if !l.counted() {
		return nil
	}
	held, err := c.CountApps(ctx, uid)
	if err != nil {
		slog.WarnContext(ctx, "appconfig: plan limit count failed", slog.String("resource", plan.Apps), slog.Any("error", err))
		return nil
	}
	if held < l.limit() || !h.planLimits.Mode.Stop(ctx, l.plan, plan.Apps, uid, held, l.limit()) {
		return nil
	}
	return h.appLimitErr(r, l, held)
}

// commit is store.CommitVersion for the caller, with the held-app check in
// the same transaction when the commit makes a blank app held.
func (h *Handler) commit(r *http.Request, appID string, c store.Commit) (*store.Version, error) {
	ctx, uid := r.Context(), uidOf(r)
	var l heldLimit
	if c.BaseVersion <= store.HeldVersion && h.heldEnforced() {
		if l = h.heldLimitOf(ctx, uid); l.counted() {
			c.HeldLimit = l.limit()
		}
	}
	v, err := h.store.CommitVersion(ctx, uid, appID, c)
	var he *store.HeldLimitError
	if errors.As(err, &he) {
		h.planLimits.Mode.Stop(ctx, l.plan, plan.Apps, uid, he.Held, he.Limit)
		return nil, h.appLimitErr(r, l, he.Held)
	}
	return v, err
}

// heldEnforced: the in-transaction check runs only when it would stop
// (enforce); log mode logs at checkAppLimit.
func (h *Handler) heldEnforced() bool {
	if _, ok := h.planCounter(); !ok {
		return false
	}
	m := h.planLimits.Mode
	return m != plan.ModeLog && m != plan.ModeOff
}

// heldApp is one item of details.apps.
type heldApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Mine: the caller created it (delete). false: a shared app it joined (leave).
	Mine      bool      `json:"mine"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// heldApps lists the caller's held apps, newest first (nil on a failure:
// iOS falls back to its own list).
func (h *Handler) heldApps(ctx context.Context, uid string) []heldApp {
	apps, err := h.store.ListApps(ctx, uid, maxHeldList, 0)
	if err != nil {
		slog.WarnContext(ctx, "appconfig: held apps list failed", slog.Any("error", err))
		return nil
	}
	out := make([]heldApp, 0, len(apps))
	for i := range apps {
		a := &apps[i]
		if a.CurrentVersion <= store.HeldVersion {
			continue
		}
		owner, err := h.appOwner(ctx, a.ID)
		if err != nil {
			slog.WarnContext(ctx, "appconfig: owner lookup failed", slog.String("appId", a.ID), slog.Any("error", err))
		}
		out = append(out, heldApp{ID: a.ID, Name: a.Name, Mine: owner == "" || owner == uid, UpdatedAt: a.UpdatedAt})
	}
	return out
}

// appLimitErr is 402 app_limit_reached.
func (h *Handler) appLimitErr(r *http.Request, l heldLimit, held int) error {
	t := *h.planLimits
	pro := t.Pro.Apps
	upgrade := l.plan == plan.Free && (pro <= 0 || pro > l.base)
	return apiErr(http.StatusPaymentRequired, httpx.CodeAppLimitReached, appLimitMessage(l.limit(), l.price > 0)).
		With("resource", plan.Apps).
		With("plan", string(l.plan)).
		With("held", held).
		With("limit", l.limit()).
		With("base", l.base).
		With("purchasedSlots", l.purchased).
		With("canBuySlot", l.price > 0).
		With("slotCoins", l.price).
		With("balance", l.balance).
		With("upgrade", upgrade).
		With("apps", h.heldApps(r.Context(), uidOf(r)))
}

func appLimitMessage(limit int, canBuy bool) string {
	msg := fmt.Sprintf("持てるアプリの数(%d 個)に達しました。使わないアプリを削除するか、共有のアプリから抜けると、新しく作ったり参加したりできます。", limit)
	if canBuy {
		msg += "コインで枠を 1 つ増やすこともできます。"
	}
	return msg
}
