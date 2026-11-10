package coins

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"
)

// Memory is the Store for tests and local runs without Firestore.
type Memory struct {
	mu        sync.Mutex
	wallets   map[string]Wallet
	ledgers   map[string]map[string]Entry // uid -> id -> entry
	holds     map[string]Hold
	purchases map[string]Purchase
}

// NewMemory returns an empty store.
func NewMemory() *Memory {
	return &Memory{wallets: map[string]Wallet{}, ledgers: map[string]map[string]Entry{}, holds: map[string]Hold{}, purchases: map[string]Purchase{}}
}

// Update implements Store.
func (m *Memory) Update(_ context.Context, k Keys, fn func(*Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx := &Tx{Wallet: m.wallets[k.UID]}
	tx.Wallet.Holds = maps.Clone(tx.Wallet.Holds)
	if k.HoldID != "" {
		if h, ok := m.holds[k.HoldID]; ok {
			tx.Hold = &h
		}
	}
	if k.TransactionID != "" {
		if p, ok := m.purchases[k.TransactionID]; ok {
			tx.Purchase = &p
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	if tx.WalletDirty {
		m.wallets[k.UID] = tx.Wallet
	}
	if tx.HoldDirty && tx.Hold != nil {
		m.holds[k.HoldID] = *tx.Hold
	}
	if tx.PurchaseDirty && tx.Purchase != nil {
		p := *tx.Purchase
		m.purchases[k.TransactionID] = p
	}
	if len(tx.Entries) > 0 && m.ledgers[k.UID] == nil {
		m.ledgers[k.UID] = map[string]Entry{}
	}
	for _, e := range tx.Entries {
		m.ledgers[k.UID][e.ID] = e
	}
	return nil
}

// Expired implements Store.
func (m *Memory) Expired(_ context.Context, now time.Time, limit int) ([]HoldRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var hs []Hold
	for _, h := range m.holds {
		if h.Status == HoldHeld && !h.ExpireAt.After(now) {
			hs = append(hs, h)
		}
	}
	slices.SortFunc(hs, func(a, b Hold) int { return a.ExpireAt.Compare(b.ExpireAt) })
	out := []HoldRef{}
	for _, h := range hs {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, HoldRef{UID: h.UID, ID: h.ID})
	}
	return out, nil
}

// Ledger implements Store.
func (m *Memory) Ledger(_ context.Context, uid string, limit int, cursor string) ([]Entry, string, error) {
	var after *Entry
	if cursor != "" {
		at, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		after = &Entry{At: at, ID: id}
	}
	m.mu.Lock()
	es := slices.Collect(maps.Values(m.ledgers[uid]))
	m.mu.Unlock()
	slices.SortFunc(es, func(a, b Entry) int {
		if before(a, b) {
			return -1
		}
		return 1
	})
	out := []Entry{}
	for _, e := range es {
		if after != nil && !before(*after, e) {
			continue
		}
		if len(out) == limit {
			return out, encodeCursor(out[len(out)-1]), nil
		}
		out = append(out, e)
	}
	return out, "", nil
}

// PurgeUser implements Store.
func (m *Memory) PurgeUser(_ context.Context, uid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.ledgers[uid])
	delete(m.ledgers, uid)
	if _, ok := m.wallets[uid]; ok {
		delete(m.wallets, uid)
		n++
	}
	for id, h := range m.holds {
		if h.UID == uid {
			delete(m.holds, id)
			n++
		}
	}
	return n, nil
}
