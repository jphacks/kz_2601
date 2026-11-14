package coins

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// The operations on a Store. Each is one Store.Update, so a wallet change
// and its ledger rows are written together, and two concurrent holds can
// never take more than the balance.

// stamp makes entry ids unique per write (a grant row of the same period may
// be written again after a subscription change) and sets their time.
func stamp(es []Entry, now time.Time) []Entry {
	for i := range es {
		if es[i].Kind == KindGrantInitial || es[i].Kind == KindGrantMonthly || (es[i].Kind == KindGrantExpire && !strings.HasPrefix(es[i].ID, "grant_expire_hold_")) {
			es[i].ID += "_" + strconv.FormatInt(now.UnixMilli(), 10)
		}
		es[i].At = now
	}
	return es
}

// applyGrants is refresh with the initial grant given once per person: the
// first account to claim one of g.InitialKeys gets it, in the same
// transaction; a key another account claimed means none for this one.
// Without keys the initial grant waits (nothing is marked).
func applyGrants(tx *Tx, uid string, g GrantPolicy, now time.Time) ([]Entry, error) {
	if !tx.Wallet.InitialGranted && !g.Anonymous && g.Initial > 0 {
		ok, err := claimInitial(tx, uid, g.InitialKeys, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			g.Initial = 0
		}
	}
	return refresh(&tx.Wallet, g, now), nil
}

// claimInitial reports whether uid may have the initial grant, recording
// its claims in tx when so.
func claimInitial(tx *Tx, uid string, keys []string, now time.Time) (bool, error) {
	if len(keys) == 0 || tx.Claims == nil {
		return false, nil
	}
	have, err := tx.Claims(keys)
	if err != nil {
		return false, err
	}
	owner := OwnerHash(uid)
	for _, k := range keys {
		if c, ok := have[k]; ok && c.Owner != owner {
			tx.Wallet.InitialGranted, tx.WalletDirty = true, true // already given to this person
			return false, nil
		}
	}
	for _, k := range keys {
		if _, ok := have[k]; ok {
			continue
		}
		if tx.NewClaims == nil {
			tx.NewClaims = map[string]InitialClaim{}
		}
		tx.NewClaims[k] = InitialClaim{Owner: owner, At: now}
	}
	return true, nil
}

// GetWallet applies the grants due at now and returns the wallet.
func GetWallet(ctx context.Context, st Store, uid string, g GrantPolicy, now time.Time) (Wallet, error) {
	now = storedTime(now)
	var out Wallet
	err := st.Update(ctx, Keys{UID: uid}, func(tx *Tx) error {
		es, err := applyGrants(tx, uid, g, now)
		if err != nil {
			return err
		}
		if len(es) > 0 {
			tx.Entries = stamp(es, now)
			tx.Wallet.UpdatedAt = now
			tx.WalletDirty = true
		}
		out = tx.Wallet
		return nil
	})
	return out, err
}

// ReserveHold takes h.Amount from uid's wallet into a new hold keyed by
// HoldID(uid, h.RequestID). A second call with the same request id returns
// the first hold (Existing) and takes nothing. Short balance is an
// *InsufficientError and writes nothing.
func ReserveHold(ctx context.Context, st Store, uid string, h Hold, g GrantPolicy, now time.Time) (Hold, Wallet, error) {
	now = storedTime(now)
	h.ID, h.UID = HoldID(uid, h.RequestID), uid
	var out Hold
	var w Wallet
	err := st.Update(ctx, Keys{UID: uid, HoldID: h.ID}, func(tx *Tx) error {
		if tx.Hold != nil {
			out, w = *tx.Hold, tx.Wallet
			out.Existing = true
			return nil
		}
		es, err := applyGrants(tx, uid, g, now)
		if err != nil {
			return err
		}
		parts, ok := take(&tx.Wallet, h.Amount)
		if !ok {
			return &InsufficientError{Required: h.Amount, Balance: tx.Wallet.Available(), Payer: "self"}
		}
		nh := h
		nh.Parts, nh.GrantPeriodEnd, nh.Status, nh.CreatedAt = parts, tx.Wallet.GrantPeriodEnd, HoldHeld, now
		nh.ExpireAt = storedTime(nh.ExpireAt)
		if tx.Wallet.Holds == nil {
			tx.Wallet.Holds = map[string]time.Time{}
		}
		tx.Wallet.Holds[nh.ID] = nh.ExpireAt
		tx.Wallet.UpdatedAt = now
		tx.WalletDirty, tx.Hold, tx.HoldDirty, tx.Entries = true, &nh, true, stamp(es, now)
		out, w = nh, tx.Wallet
		return nil
	})
	return out, w, err
}

// Settlement closes a hold: Commit charges Charge (capped at the hold) and
// records USD on the spend row; otherwise the whole hold goes back and the
// hold ends as Status (released / expired).
type Settlement struct {
	Commit bool
	Charge int64
	USD    float64
	Status string
}

// SettleHold closes uid's hold id. Committing a committed hold returns it
// (idempotent); committing a released / expired / unknown hold is
// ErrNotHeld; releasing a closed or unknown hold does nothing.
func SettleHold(ctx context.Context, st Store, uid, id string, s Settlement, now time.Time) (Hold, Wallet, error) {
	now = storedTime(now)
	var out Hold
	var w Wallet
	err := st.Update(ctx, Keys{UID: uid, HoldID: id}, func(tx *Tx) error {
		w = tx.Wallet
		if tx.Hold == nil {
			if s.Commit {
				return ErrNotHeld
			}
			return nil
		}
		h := *tx.Hold
		if h.Status != HoldHeld {
			out = h
			if s.Commit && h.Status != HoldCommitted {
				return ErrNotHeld
			}
			return nil
		}
		charge, status := int64(0), s.Status
		if s.Commit {
			charge, status = s.Charge, HoldCommitted
		}
		if status == "" {
			status = HoldReleased
		}
		es := settle(&tx.Wallet, &h, charge, now)
		for i := range es {
			if es[i].Kind == KindSpendAI || es[i].Kind == KindSpendExt {
				es[i].USD = s.USD
			}
		}
		h.Status = status
		tx.Wallet.UpdatedAt = now
		tx.WalletDirty, tx.Hold, tx.HoldDirty, tx.Entries = true, &h, true, stamp(es, now)
		out, w = h, tx.Wallet
		return nil
	})
	return out, w, err
}

// ExtendHold moves uid's held hold id to expire at expireAt (never earlier).
// A closed or unknown hold is left as is.
func ExtendHold(ctx context.Context, st Store, uid, id string, expireAt time.Time) error {
	expireAt = storedTime(expireAt)
	return st.Update(ctx, Keys{UID: uid, HoldID: id}, func(tx *Tx) error {
		h := tx.Hold
		if h == nil || h.Status != HoldHeld || !expireAt.After(h.ExpireAt) {
			return nil
		}
		nh := *h
		nh.ExpireAt = expireAt
		if tx.Wallet.Holds == nil {
			tx.Wallet.Holds = map[string]time.Time{}
		}
		tx.Wallet.Holds[id] = expireAt
		tx.WalletDirty, tx.Hold, tx.HoldDirty = true, &nh, true
		return nil
	})
}

// CreditPurchase adds p.Coins to uid's purchased coins once per
// p.TransactionID. credited is false when it was already credited to uid;
// another uid is ErrOwnedByOther.
func CreditPurchase(ctx context.Context, st Store, uid string, p Purchase, g GrantPolicy, now time.Time) (w Wallet, credited bool, err error) {
	now = storedTime(now)
	p.UID, p.At = uid, now
	err = st.Update(ctx, Keys{UID: uid, TransactionID: p.TransactionID}, func(tx *Tx) error {
		credited = false
		if tx.Purchase != nil {
			w = tx.Wallet
			if tx.Purchase.UID != uid {
				return ErrOwnedByOther
			}
			return nil
		}
		es, err := applyGrants(tx, uid, g, now)
		if err != nil {
			return err
		}
		tx.Wallet.Purchased += p.Coins
		es = append(stamp(es, now), Entry{ID: "purchase_" + p.TransactionID, Kind: KindPurchase, Amount: p.Coins,
			Delta: Split{Purchased: p.Coins}, TransactionID: p.TransactionID, ProductID: p.ProductID, At: now})
		tx.Wallet.UpdatedAt = now
		np := p
		tx.WalletDirty, tx.Purchase, tx.PurchaseDirty, tx.Entries = true, &np, true, es
		w, credited = tx.Wallet, true
		return nil
	})
	return w, credited, err
}

// storedTime is t in UTC at the microseconds Firestore keeps, so what an
// operation returns equals what is read back.
func storedTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Microsecond)
}

// Ledger cursors: "<at unix micro>_<entry id>".
func encodeCursor(e Entry) string {
	return strconv.FormatInt(e.At.UnixMicro(), 10) + "_" + e.ID
}

func decodeCursor(c string) (time.Time, string, error) {
	at, id, ok := strings.Cut(c, "_")
	us, err := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || id == "" {
		return time.Time{}, "", ErrBadCursor
	}
	return time.UnixMicro(us).UTC(), id, nil
}

// before reports whether a sorts after b in the ledger order (newest first,
// then id descending).
func before(a, b Entry) bool {
	if !a.At.Equal(b.At) {
		return a.At.After(b.At)
	}
	return a.ID > b.ID
}
