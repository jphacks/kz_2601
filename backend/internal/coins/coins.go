// Package coins is the coin wallet and ledger (#630,
// docs/decisions/630-token-billing.md). Every LLM call is paid from the
// payer's coins: Reserve holds the call's upper bound before it runs, Commit
// charges the actual cost (never more than the hold) and Release returns the
// whole hold when the call failed. A hold nobody settles expires and is
// returned the next time the payer's wallet is read (Service.Wallet, or the
// next Reserve). No scheduled job calls Service.Sweep yet: until one does, an
// expired hold stays in Reserved (not spendable, not lost) until that read.
//
//	users/{uid}/coins/wallet          Wallet: balance by source, held coins, the open holds
//	users/{uid}/coinLedger/{entryId}  Entry: one row per balance change (same transaction)
//	coinReservations/{holdId}         Hold: keyed by uid + X-Request-Id (idempotent)
//	coin_transactions/{transactionId} Purchase: one App Store consumable credited once
//
// Sources and the spend order: Grant (Tailor It Pro's monthly coins, gone at
// GrantPeriodEnd) → Initial (the one-time grant of a signed-in account) →
// Purchased (bought, never expires; App Store guideline 3.1.1).
//
// Everything is behind COIN_BILLING_MODE (off | log | enforce, default off):
// off touches nothing, log keeps the wallet (grants, purchases) and only logs
// what a call would cost, enforce holds and charges. Prices are never in this
// repo: COIN_PER_USD, COIN_RESERVE_*, COIN_PACKS, ... come from env
// (Config, docs/infra/runtime-config.md).
//
// Part 2 (#630) hooks the AI paths through Biller (Service implements it):
// generate / edit / chat / text call Reserve where ratelimit runs today,
// Commit with the cost_tally total and Release on a failure that refunds
// (ratelimit/refund.go). Part 3 (server allowance overflow, overflow.go) buys
// allowance blocks for the runner (BuyOverflow); the 402 codes are WriteError.
package coins

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Ledger kinds (Entry.Kind).
const (
	KindGrantInitial  = "grant_initial"
	KindGrantMonthly  = "grant_monthly"
	KindGrantExpire   = "grant_expire"
	KindPurchase      = "purchase"
	KindSpendAI       = "spend_ai"
	KindSpendOverflow = "spend_overflow" // part 3
	KindSpendSlot     = "spend_slot"     // one app slot (slots.go)
	KindSpendCall     = "spend_call"     // call minutes (#447, internal/calls)
	KindRefundFailed  = "refund_failed"  // a committed charge given back (part 2)
	KindRefundApple   = "refund_apple"   // App Store REFUND of a pack (part 4)
	KindAdjust        = "adjust"         // support
)

// Hold statuses.
const (
	HoldHeld      = "held"
	HoldCommitted = "committed"
	HoldReleased  = "released"
	HoldExpired   = "expired"
)

// Errors.
var (
	// ErrOwnedByOther: the App Store transaction was credited to another uid.
	ErrOwnedByOther = errors.New("transaction was credited to another account")
	// ErrNotHeld: Commit on a hold that was released or expired (the coins
	// went back; the call is not charged).
	ErrNotHeld = errors.New("coin hold is not held")
	// ErrOff: COIN_BILLING_MODE=off.
	ErrOff = errors.New("coin billing is off")
	// ErrAnonymous: anonymous accounts cannot buy coins (consumables are not
	// restored by StoreKit; sign in first).
	ErrAnonymous = errors.New("anonymous accounts cannot buy coins")
	// ErrUnknownOp: Reserve with an op Config does not know.
	ErrUnknownOp = errors.New("unknown coin op")
	// ErrBadCursor: a ledger cursor this package did not make.
	ErrBadCursor = errors.New("bad ledger cursor")
)

// InsufficientError is 402 coins_insufficient: the payer has fewer
// available coins than the hold needs.
type InsufficientError struct {
	Required int64
	Balance  int64
	// Payer is "self" or "creator" (a shared app's creator pays, part 2).
	Payer string
}

func (e *InsufficientError) Error() string {
	return fmt.Sprintf("coins insufficient: required %d, balance %d", e.Required, e.Balance)
}

// Wallet is users/{uid}/coins/wallet.
type Wallet struct {
	Purchased      int64     `json:"purchased" firestore:"purchased"`
	Grant          int64     `json:"grant" firestore:"grant"`
	GrantPeriodEnd time.Time `json:"grantPeriodEnd,omitzero" firestore:"grantPeriodEnd,omitempty"`
	Initial        int64     `json:"initial" firestore:"initial"`
	InitialGranted bool      `json:"-" firestore:"initialGranted"`
	// Reserved is the sum of the open holds (already taken from the sources).
	Reserved int64 `json:"reserved" firestore:"reserved"`
	// Holds are the open holds (hold id -> expireAt), so a wallet read finds
	// expired ones without a query.
	Holds map[string]time.Time `json:"-" firestore:"holds,omitempty"`
	// Overflow is the owner's opt-in to pay past the server allowance with
	// coins (PUT /v1/me/coins/settings; part 3 enforces it).
	Overflow Overflow `json:"-" firestore:"overflow"`
	// AppSlots are the app slots bought (slots.go): never expire, never refunded.
	AppSlots  int64     `json:"-" firestore:"appSlots,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero" firestore:"updatedAt,omitempty"`
}

// Available is what a new hold can take.
func (w Wallet) Available() int64 { return w.Purchased + w.Grant + w.Initial }

// Total is Available plus Reserved; it equals the sum of the ledger.
func (w Wallet) Total() int64 { return w.Available() + w.Reserved }

// Split is an amount by source (Entry deltas, Hold parts).
type Split struct {
	Grant     int64 `json:"grant" firestore:"grant"`
	Initial   int64 `json:"initial" firestore:"initial"`
	Purchased int64 `json:"purchased" firestore:"purchased"`
}

// Sum is the total of the parts.
func (s Split) Sum() int64 { return s.Grant + s.Initial + s.Purchased }

// Entry is users/{uid}/coinLedger/{id}. Amount is signed and equals
// Delta.Sum(); the ids are deterministic so a retried write is the same row.
type Entry struct {
	ID            string    `json:"id" firestore:"-"`
	Kind          string    `json:"kind" firestore:"kind"`
	Amount        int64     `json:"amount" firestore:"amount"`
	Delta         Split     `json:"-" firestore:"delta"`
	Op            string    `json:"op,omitempty" firestore:"op,omitempty"`
	AppID         string    `json:"appId,omitempty" firestore:"appId,omitempty"`
	RequestID     string    `json:"-" firestore:"requestId,omitempty"`
	TransactionID string    `json:"transactionId,omitempty" firestore:"transactionId,omitempty"`
	ProductID     string    `json:"productId,omitempty" firestore:"productId,omitempty"`
	USD           float64   `json:"-" firestore:"usd,omitempty"` // never in the API (would reveal COIN_PER_USD)
	At            time.Time `json:"at" firestore:"at"`
}

// Hold is coinReservations/{id}.
type Hold struct {
	ID        string `json:"id" firestore:"-"`
	UID       string `json:"-" firestore:"uid"`
	Op        string `json:"op" firestore:"op"`
	AppID     string `json:"appId,omitempty" firestore:"appId,omitempty"`
	RequestID string `json:"-" firestore:"requestId"`
	Amount    int64  `json:"amount" firestore:"amount"`
	// Parts is where Amount was taken from (returned to the same sources).
	Parts Split `json:"-" firestore:"parts"`
	// GrantPeriodEnd of the wallet when held: a grant part is not returned
	// after the period ended.
	GrantPeriodEnd time.Time `json:"-" firestore:"grantPeriodEnd,omitempty"`
	Status         string    `json:"status" firestore:"status"`
	Charged        int64     `json:"charged" firestore:"charged"`
	CreatedAt      time.Time `json:"-" firestore:"createdAt"`
	// ExpireAt is set only while held (Store.Expired queries it; settled holds drop it).
	ExpireAt  time.Time `json:"-" firestore:"expireAt,omitempty"`
	SettledAt time.Time `json:"-" firestore:"settledAt,omitempty"`
	// DeleteAt is for a Firestore TTL policy on settled holds (kept a while
	// so a retried request id still finds its result).
	DeleteAt time.Time `json:"-" firestore:"deleteAt,omitempty"`

	// Existing: Reserve found this hold for the same uid + request id (a
	// retry); nothing was taken again. Not stored.
	Existing bool `json:"-" firestore:"-"`
	// DryRun: COIN_BILLING_MODE=log; nothing was taken. Not stored.
	DryRun bool `json:"-" firestore:"-"`
}

// Purchase is coin_transactions/{transactionId}.
type Purchase struct {
	UID           string    `firestore:"uid"`
	TransactionID string    `firestore:"-"`
	ProductID     string    `firestore:"productId"`
	Coins         int64     `firestore:"amount"`
	Environment   string    `firestore:"environment"`
	At            time.Time `firestore:"at"`
}

// GrantPolicy is what the grants of a wallet should be at a moment.
type GrantPolicy struct {
	// Initial is COIN_INITIAL_GRANT, credited once to a non-anonymous account.
	Initial   int64
	Anonymous bool
	// Monthly is COIN_PRO_MONTHLY; ProPeriodEnd is the active Pro
	// subscription's expiresAt (zero = not Pro).
	Monthly      int64
	ProPeriodEnd time.Time
}

// HoldID keys a hold by uid and request id (a client-chosen X-Request-Id
// must not collide with another user's).
func HoldID(uid, requestID string) string {
	sum := sha256.Sum256([]byte(uid + "\x00" + requestID))
	return hex.EncodeToString(sum[:20])
}

// SettledRetention is how long settled holds are kept for retries (DeleteAt).
const SettledRetention = 7 * 24 * time.Hour

// refresh applies the grants due at now to w and returns their ledger rows:
// the initial grant once, the end of a monthly grant period, a new period.
func refresh(w *Wallet, g GrantPolicy, now time.Time) []Entry {
	var out []Entry
	if !w.InitialGranted && !g.Anonymous && g.Initial > 0 {
		w.Initial += g.Initial
		w.InitialGranted = true
		out = append(out, Entry{ID: "grant_initial", Kind: KindGrantInitial, Amount: g.Initial, Delta: Split{Initial: g.Initial}, At: now})
	}
	expire := func() {
		if w.Grant > 0 {
			out = append(out, Entry{ID: "grant_expire_" + periodKey(w.GrantPeriodEnd), Kind: KindGrantExpire, Amount: -w.Grant, Delta: Split{Grant: -w.Grant}, At: now})
			w.Grant = 0
		}
	}
	if !w.GrantPeriodEnd.IsZero() && !now.Before(w.GrantPeriodEnd) {
		expire()
	}
	end := g.ProPeriodEnd.UTC().Truncate(time.Microsecond)
	if g.Monthly > 0 && end.After(now) && !end.Equal(w.GrantPeriodEnd) {
		expire()
		w.Grant = g.Monthly
		w.GrantPeriodEnd = end
		out = append(out, Entry{ID: "grant_monthly_" + periodKey(end), Kind: KindGrantMonthly, Amount: g.Monthly, Delta: Split{Grant: g.Monthly}, At: now})
	}
	return out
}

func periodKey(t time.Time) string { return fmt.Sprintf("%d", t.UnixMilli()) }

// take moves amount from w's sources (grant → initial → purchased) to
// Reserved. ok is false (w unchanged) when Available is short.
func take(w *Wallet, amount int64) (Split, bool) {
	if amount > w.Available() {
		return Split{}, false
	}
	var p Split
	rest := amount
	p.Grant, rest = min(w.Grant, rest), rest-min(w.Grant, rest)
	p.Initial, rest = min(w.Initial, rest), rest-min(w.Initial, rest)
	p.Purchased = rest
	w.Grant -= p.Grant
	w.Initial -= p.Initial
	w.Purchased -= p.Purchased
	w.Reserved += amount
	return p, true
}

// settle closes h on w: charge (≤ h.Amount) is spent in the spend order and
// the rest of each part goes back to its source. A grant part whose period
// ended is not returned (a grant_expire row). It returns the ledger rows.
func settle(w *Wallet, h *Hold, charge int64, now time.Time) []Entry {
	charge = max(0, min(charge, h.Amount))
	var spent Split
	rest := charge
	spent.Grant, rest = min(h.Parts.Grant, rest), rest-min(h.Parts.Grant, rest)
	spent.Initial, rest = min(h.Parts.Initial, rest), rest-min(h.Parts.Initial, rest)
	spent.Purchased = rest
	back := Split{Grant: h.Parts.Grant - spent.Grant, Initial: h.Parts.Initial - spent.Initial, Purchased: h.Parts.Purchased - spent.Purchased}

	var out []Entry
	w.Reserved -= h.Amount
	delete(w.Holds, h.ID)
	if back.Grant > 0 {
		if h.GrantPeriodEnd.Equal(w.GrantPeriodEnd) && now.Before(w.GrantPeriodEnd) {
			w.Grant += back.Grant
		} else {
			out = append(out, Entry{ID: "grant_expire_hold_" + h.ID, Kind: KindGrantExpire, Amount: -back.Grant, Delta: Split{Grant: -back.Grant}, Op: h.Op, AppID: h.AppID, At: now})
		}
	}
	w.Initial += back.Initial
	w.Purchased += back.Purchased
	if charge > 0 {
		kind := KindSpendAI
		if h.Op == OpCall {
			kind = KindSpendCall
		}
		out = append(out, Entry{ID: "spend_" + h.ID, Kind: kind, Amount: -charge,
			Delta: Split{Grant: -spent.Grant, Initial: -spent.Initial, Purchased: -spent.Purchased},
			Op:    h.Op, AppID: h.AppID, RequestID: h.RequestID, At: now})
	}
	h.Charged = charge
	h.SettledAt = now
	h.ExpireAt = time.Time{}
	h.DeleteAt = now.Add(SettledRetention)
	return out
}
