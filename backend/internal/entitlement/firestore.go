package entitlement

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collection names (package doc).
const (
	entitlementsCollection = "entitlements"
	proDoc                 = "pro"
	bindingsCollection     = "appstore_transactions"
	playBindingsCollection = "googleplay_purchases"
)

// Firestore is the production Store (same database as appconfig).
type Firestore struct {
	c   *firestore.Client
	now func() time.Time
}

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST).
func NewFirestore(c *firestore.Client) *Firestore { return &Firestore{c: c, now: time.Now} }

func (f *Firestore) proRef(uid string) *firestore.DocumentRef {
	return f.c.Collection("users").Doc(uid).Collection(entitlementsCollection).Doc(proDoc)
}

// bindingRef is appstore_transactions/{otid}, or googleplay_purchases/{hash}
// for a PlayKey.
func (f *Firestore) bindingRef(otid string) *firestore.DocumentRef {
	if h, ok := strings.CutPrefix(otid, playKeyPrefix); ok {
		return f.c.Collection(playBindingsCollection).Doc(h)
	}
	return f.c.Collection(bindingsCollection).Doc(otid)
}

func notFound(err error) bool { return status.Code(err) == codes.NotFound }

func proFrom(s *firestore.DocumentSnapshot) (*Pro, error) {
	var p Pro
	if err := s.DataTo(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func proData(p Pro) map[string]any {
	m := map[string]any{
		"productId":             p.ProductID,
		"originalTransactionId": p.OriginalTransactionID,
		"transactionId":         p.TransactionID,
		"environment":           p.Environment,
		"expiresAt":             p.ExpiresAt,
		"updatedAt":             p.UpdatedAt,
	}
	if p.Revoked() {
		m["revokedAt"] = p.RevokedAt
	}
	if !p.GraceExpiresAt.IsZero() {
		m["graceExpiresAt"] = p.GraceExpiresAt
	}
	if p.WillRenew != nil {
		m["willRenew"] = *p.WillRenew
	}
	if !p.SignedAt.IsZero() {
		m["signedAt"] = p.SignedAt
	}
	if p.Platform != "" {
		m["platform"] = p.Platform
	}
	return m
}

// storedPrecision truncates p's times to the microseconds Firestore keeps, so
// what Apply returns equals what Get reads back.
func storedPrecision(p Pro) Pro {
	for _, t := range []*time.Time{&p.ExpiresAt, &p.RevokedAt, &p.GraceExpiresAt, &p.UpdatedAt, &p.SignedAt} {
		*t = t.Truncate(time.Microsecond)
	}
	return p
}

// Get implements Store.
func (f *Firestore) Get(ctx context.Context, uid string) (*Pro, error) {
	s, err := f.proRef(uid).Get(ctx)
	if notFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return proFrom(s)
}

// Owner implements Store.
func (f *Firestore) Owner(ctx context.Context, otid string) (string, error) {
	s, err := f.bindingRef(otid).Get(ctx)
	if notFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	uid, _ := s.Data()["uid"].(string)
	return uid, nil
}

// Apply implements Store: one transaction reads the binding and the Pro
// document, then writes both (or fails with ErrOwnedByOther).
func (f *Firestore) Apply(ctx context.Context, uid string, p Pro, transferFrom string) (*Pro, error) {
	bref, pref := f.bindingRef(p.OriginalTransactionID), f.proRef(uid)
	p = storedPrecision(p)
	var out Pro
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snaps, err := tx.GetAll([]*firestore.DocumentRef{bref, pref})
		if err != nil {
			return err
		}
		owner := ""
		if snaps[0].Exists() {
			owner, _ = snaps[0].Data()["uid"].(string)
		}
		if !owns(owner, uid, transferFrom) {
			return ErrOwnedByOther
		}
		var cur *Pro
		if snaps[1].Exists() {
			if cur, err = proFrom(snaps[1]); err != nil {
				return err
			}
		}
		now := f.now().UTC().Truncate(time.Microsecond)
		binding := map[string]any{"uid": uid, "productId": p.ProductID, "environment": p.Environment, "updatedAt": now}
		if owner != uid {
			binding["boundAt"] = now
		}
		if err := tx.Set(bref, binding, firestore.MergeAll); err != nil {
			return err
		}
		var write bool
		if out, write = merge(cur, p, now); write {
			return tx.Set(pref, proData(out))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// PurgeUser implements Store.
func (f *Firestore) PurgeUser(ctx context.Context, uid string) (int, error) {
	refs := []*firestore.DocumentRef{}
	collect := func(it *firestore.DocumentRefIterator) error {
		for {
			r, err := it.Next()
			if errors.Is(err, iterator.Done) {
				return nil
			}
			if err != nil {
				return err
			}
			refs = append(refs, r)
		}
	}
	if err := collect(f.c.Collection("users").Doc(uid).Collection(entitlementsCollection).DocumentRefs(ctx)); err != nil {
		return 0, err
	}
	for _, col := range []string{bindingsCollection, playBindingsCollection} {
		if err := collectOwned(ctx, f.c.Collection(col), uid, &refs); err != nil {
			return 0, err
		}
	}
	n := 0
	for _, r := range refs {
		if _, err := r.Delete(ctx); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// collectOwned appends the bindings in col whose uid is uid.
func collectOwned(ctx context.Context, col *firestore.CollectionRef, uid string, refs *[]*firestore.DocumentRef) error {
	it := col.Where("uid", "==", uid).Documents(ctx)
	defer it.Stop()
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		*refs = append(*refs, s.Ref)
	}
}
