package coins

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// App slots (#630, owner decision 2026-11-10): one more app a user may hold
// past their plan's PLAN_*_APPS, bought with COIN_PRICE_APP_SLOT coins, one
// at a time and for good (never expires). The count is wallet.appSlots; the
// held-app limit of appconfig is the plan's base plus it (appconfig
// app_slots.go). Buying needs COIN_BILLING_MODE=enforce and a price > 0; in
// log mode the slots bought before still count, in off mode nothing is read.

// OpAppSlot is the op of a slot purchase (ledger rows and its idempotency hold).
const OpAppSlot = "app_slot"

// EventSlotBought is logged for each slot bought.
const EventSlotBought = "coins.slot_bought"

// ErrSlotsOff: slots cannot be bought (mode is not enforce or the price is 0).
var ErrSlotsOff = errors.New("app slots cannot be bought")

// SlotPurchase is POST /v1/me/coins/slot.
type SlotPurchase struct {
	// Bought is false when this request id had already bought its slot (a
	// retry): nothing was spent again.
	Bought bool `json:"bought"`
	// AppSlots is the slots owned after the call.
	AppSlots int64 `json:"appSlots"`
	Coins    int64 `json:"coins"`
	Wallet   View  `json:"wallet"`
}

// SlotsBuyable reports whether BuySlot can succeed at all (enforce, price > 0).
func (s *Service) SlotsBuyable() bool {
	return s.Mode() == ModeEnforce && s.Config.SlotPrice > 0
}

// BuySlot spends Config.SlotPrice coins for one app slot of uid, once per
// requestID: the coins (grant → initial → purchased), a spend_slot ledger row
// and wallet.appSlots+1 in one transaction, with a settled hold keyed by
// HoldID(uid, "app_slot:"+requestID) as the idempotency record (kept
// SettledRetention). Errors: ErrSlotsOff, *InsufficientError.
func (s *Service) BuySlot(ctx context.Context, uid string, anonymous bool, requestID string) (SlotPurchase, error) {
	if !s.SlotsBuyable() {
		return SlotPurchase{}, ErrSlotsOff
	}
	if requestID == "" {
		return SlotPurchase{}, errors.New("coins: a slot purchase needs a request id")
	}
	g, err := s.policy(ctx, uid, anonymous)
	if err != nil {
		return SlotPurchase{}, err
	}
	price := s.Config.SlotPrice
	now := storedTime(s.now())
	id := HoldID(uid, OpAppSlot+":"+requestID)
	var out SlotPurchase
	var w Wallet
	err = s.Store.Update(ctx, Keys{UID: uid, HoldID: id}, func(tx *Tx) error {
		es := stamp(refresh(&tx.Wallet, g, now), now)
		if tx.Hold != nil { // a retry of a purchase that went through
			if len(es) > 0 {
				tx.Entries, tx.Wallet.UpdatedAt, tx.WalletDirty = es, now, true
			}
			w = tx.Wallet
			out = SlotPurchase{AppSlots: w.AppSlots, Coins: tx.Hold.Charged}
			return nil
		}
		if tx.Wallet.Available() < price {
			return &InsufficientError{Required: price, Balance: tx.Wallet.Available(), Payer: "self"}
		}
		d := spend(&tx.Wallet, price)
		tx.Wallet.AppSlots++
		tx.Wallet.UpdatedAt = now
		es = append(es, Entry{ID: "slot_" + id, Kind: KindSpendSlot, Amount: -price,
			Delta: Split{Grant: -d.Grant, Initial: -d.Initial, Purchased: -d.Purchased}, Op: OpAppSlot, RequestID: requestID, At: now})
		tx.Hold = &Hold{UID: uid, Op: OpAppSlot, RequestID: requestID, Amount: price, Parts: d, Status: HoldCommitted,
			Charged: price, CreatedAt: now, SettledAt: now, DeleteAt: now.Add(SettledRetention)}
		tx.Entries, tx.WalletDirty, tx.HoldDirty = es, true, true
		w = tx.Wallet
		out = SlotPurchase{Bought: true, AppSlots: w.AppSlots, Coins: price}
		return nil
	})
	if err != nil {
		return SlotPurchase{}, err
	}
	out.Wallet = s.view(w)
	if out.Bought {
		s.log().InfoContext(ctx, EventSlotBought, slog.String("event", EventSlotBought),
			slog.String("uid_hash", httpx.UIDHash(uid)), slog.Int64("appSlots", out.AppSlots), slog.Int64("coins", price))
	}
	return out, nil
}

// SlotInfo is what the held-app limit of appconfig needs from the wallet.
type SlotInfo struct {
	// Purchased is wallet.appSlots (0 in off mode: nothing is read).
	Purchased int
	// Price is COIN_PRICE_APP_SLOT when a slot can be bought now (enforce,
	// price > 0), else 0.
	Price int64
	// Available is the coins a purchase can take (enforce only).
	Available int64
}

// AppSlots reads uid's slots (one wallet read; no grant is applied).
func (s *Service) AppSlots(ctx context.Context, uid string) (SlotInfo, error) {
	if s.Mode() == ModeOff {
		return SlotInfo{}, nil
	}
	var w Wallet
	err := s.Store.Update(ctx, Keys{UID: uid}, func(tx *Tx) error {
		w = tx.Wallet
		return nil
	})
	if err != nil {
		return SlotInfo{}, err
	}
	out := SlotInfo{Purchased: int(w.AppSlots)}
	if s.SlotsBuyable() {
		out.Price, out.Available = s.Config.SlotPrice, w.Available()
	}
	return out, nil
}
