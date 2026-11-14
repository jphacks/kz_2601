package coins

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Collection names (package doc).
const (
	usersCollection         = "users"
	coinsCollection         = "coins"
	walletDoc               = "wallet"
	ledgerCollection        = "coinLedger"
	holdsCollection         = "coinReservations"
	transactionsCollection  = "coin_transactions"
	initialGrantsCollection = "coin_initial_grants"
)

// Firestore is the production Store (same database as appconfig).
type Firestore struct {
	c *firestore.Client
}

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST).
func NewFirestore(c *firestore.Client) *Firestore { return &Firestore{c: c} }

func (f *Firestore) walletRef(uid string) *firestore.DocumentRef {
	return f.c.Collection(usersCollection).Doc(uid).Collection(coinsCollection).Doc(walletDoc)
}

func (f *Firestore) ledger(uid string) *firestore.CollectionRef {
	return f.c.Collection(usersCollection).Doc(uid).Collection(ledgerCollection)
}

func (f *Firestore) holdRef(id string) *firestore.DocumentRef {
	return f.c.Collection(holdsCollection).Doc(id)
}

func (f *Firestore) purchaseRef(id string) *firestore.DocumentRef {
	return f.c.Collection(transactionsCollection).Doc(id)
}

// Update implements Store with one Firestore transaction (all reads first).
func (f *Firestore) Update(ctx context.Context, k Keys, fn func(*Tx) error) error {
	refs := []*firestore.DocumentRef{f.walletRef(k.UID)}
	holdAt, purchaseAt := -1, -1
	if k.HoldID != "" {
		holdAt = len(refs)
		refs = append(refs, f.holdRef(k.HoldID))
	}
	if k.TransactionID != "" {
		purchaseAt = len(refs)
		refs = append(refs, f.purchaseRef(k.TransactionID))
	}
	return f.c.RunTransaction(ctx, func(_ context.Context, ftx *firestore.Transaction) error {
		snaps, err := ftx.GetAll(refs)
		if err != nil {
			return err
		}
		tx := &Tx{Claims: func(keys []string) (map[string]InitialClaim, error) {
			crefs := make([]*firestore.DocumentRef, len(keys))
			for i, key := range keys {
				crefs[i] = f.c.Collection(initialGrantsCollection).Doc(key)
			}
			cs, err := ftx.GetAll(crefs)
			if err != nil {
				return nil, err
			}
			out := map[string]InitialClaim{}
			for i, s := range cs {
				if !s.Exists() {
					continue
				}
				var c InitialClaim
				if err := s.DataTo(&c); err != nil {
					return nil, err
				}
				out[keys[i]] = c
			}
			return out, nil
		}}
		if snaps[0].Exists() {
			if err := snaps[0].DataTo(&tx.Wallet); err != nil {
				return err
			}
		}
		if holdAt >= 0 && snaps[holdAt].Exists() {
			var h Hold
			if err := snaps[holdAt].DataTo(&h); err != nil {
				return err
			}
			h.ID = k.HoldID
			tx.Hold = &h
		}
		if purchaseAt >= 0 && snaps[purchaseAt].Exists() {
			var p Purchase
			if err := snaps[purchaseAt].DataTo(&p); err != nil {
				return err
			}
			p.TransactionID = k.TransactionID
			tx.Purchase = &p
		}
		if err := fn(tx); err != nil {
			return err
		}
		if tx.WalletDirty {
			if err := ftx.Set(refs[0], tx.Wallet); err != nil {
				return err
			}
		}
		if tx.HoldDirty && tx.Hold != nil && holdAt >= 0 {
			if err := ftx.Set(refs[holdAt], *tx.Hold); err != nil {
				return err
			}
		}
		if tx.PurchaseDirty && tx.Purchase != nil && purchaseAt >= 0 {
			if err := ftx.Set(refs[purchaseAt], *tx.Purchase); err != nil {
				return err
			}
		}
		for _, e := range tx.Entries {
			if err := ftx.Set(f.ledger(k.UID).Doc(e.ID), e); err != nil {
				return err
			}
		}
		for key, c := range tx.NewClaims {
			if err := ftx.Set(f.c.Collection(initialGrantsCollection).Doc(key), c); err != nil {
				return err
			}
		}
		return nil
	})
}

// Expired implements Store. Only held holds carry expireAt, so this is a
// single-field query (no composite index).
func (f *Firestore) Expired(ctx context.Context, now time.Time, limit int) ([]HoldRef, error) {
	q := f.c.Collection(holdsCollection).Where("expireAt", "<=", now).OrderBy("expireAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	out := []HoldRef{}
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		uid, _ := s.Data()["uid"].(string)
		out = append(out, HoldRef{UID: uid, ID: s.Ref.ID})
	}
}

// Ledger implements Store (newest first, then id descending).
func (f *Firestore) Ledger(ctx context.Context, uid string, limit int, cursor string) ([]Entry, string, error) {
	q := f.ledger(uid).OrderBy("at", firestore.Desc).OrderBy(firestore.DocumentID, firestore.Desc)
	if cursor != "" {
		at, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		q = q.StartAfter(at, id)
	}
	it := q.Limit(limit + 1).Documents(ctx)
	defer it.Stop()
	out := []Entry{}
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, "", nil
		}
		if err != nil {
			return nil, "", err
		}
		if len(out) == limit {
			return out, encodeCursor(out[len(out)-1]), nil
		}
		var e Entry
		if err := s.DataTo(&e); err != nil {
			return nil, "", err
		}
		e.ID = s.Ref.ID
		out = append(out, e)
	}
}

// PurgeUser implements Store.
func (f *Firestore) PurgeUser(ctx context.Context, uid string) (int, error) {
	var refs []*firestore.DocumentRef
	collect := func(it *firestore.DocumentIterator) error {
		defer it.Stop()
		for {
			s, err := it.Next()
			if errors.Is(err, iterator.Done) {
				return nil
			}
			if err != nil {
				return err
			}
			refs = append(refs, s.Ref)
		}
	}
	if err := collect(f.ledger(uid).Documents(ctx)); err != nil {
		return 0, err
	}
	if err := collect(f.c.Collection(holdsCollection).Where("uid", "==", uid).Documents(ctx)); err != nil {
		return 0, err
	}
	refs = append(refs, f.walletRef(uid))
	n := 0
	for _, r := range refs {
		if _, err := r.Delete(ctx); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
