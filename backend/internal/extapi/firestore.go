package extapi

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collection names.
const (
	usersCollection       = "users"
	connectionsCollection = "connections"
	statesCollection      = "oauthStates"
)

// Firestore is ConnectionStore and StateStore on Firestore (same database
// as appconfig; only the runner reads or writes these documents).
//
//	users/{uid}/connections/{provider}  Connection (deleteAt: TTL)
//	oauthStates/{state}                 OAuthState (expireAt: TTL)
type Firestore struct{ c *firestore.Client }

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST).
func NewFirestore(c *firestore.Client) *Firestore { return &Firestore{c: c} }

var (
	_ ConnectionStore = (*Firestore)(nil)
	_ StateStore      = (*Firestore)(nil)
)

func (f *Firestore) conns(uid string) *firestore.CollectionRef {
	return f.c.Collection(usersCollection).Doc(uid).Collection(connectionsCollection)
}

// Get implements ConnectionStore.
func (f *Firestore) Get(ctx context.Context, uid, provider string) (Connection, bool, error) {
	s, err := f.conns(uid).Doc(provider).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}
	var c Connection
	if err := s.DataTo(&c); err != nil {
		return Connection{}, false, err
	}
	return c, true, nil
}

// Put implements ConnectionStore.
func (f *Firestore) Put(ctx context.Context, uid string, c Connection) error {
	if c.Provider == "" {
		return errors.New("extapi: connection without provider")
	}
	_, err := f.conns(uid).Doc(c.Provider).Set(ctx, c)
	return err
}

// Delete implements ConnectionStore.
func (f *Firestore) Delete(ctx context.Context, uid, provider string) error {
	_, err := f.conns(uid).Doc(provider).Delete(ctx)
	return err
}

// List implements ConnectionStore.
func (f *Firestore) List(ctx context.Context, uid string) ([]Connection, error) {
	it := f.conns(uid).OrderBy(firestore.DocumentID, firestore.Asc).Documents(ctx)
	defer it.Stop()
	var out []Connection
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var c Connection
		if err := s.DataTo(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
}

// Due implements ConnectionStore (a collection group query on deleteAt;
// its single-field index on the connections group is a human's step).
func (f *Firestore) Due(ctx context.Context, now time.Time, limit int) ([]ConnRef, error) {
	q := f.c.CollectionGroup(connectionsCollection).Where("deleteAt", "<", now).OrderBy("deleteAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	var out []ConnRef
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		user := s.Ref.Parent.Parent
		if user == nil {
			continue
		}
		out = append(out, ConnRef{UID: user.ID, Provider: s.Ref.ID})
	}
}

// PutState implements StateStore.
func (f *Firestore) PutState(ctx context.Context, state string, s OAuthState) error {
	_, err := f.c.Collection(statesCollection).Doc(state).Create(ctx, s)
	return err
}

// TakeState implements StateStore in one transaction (a state works once).
func (f *Firestore) TakeState(ctx context.Context, state string) (OAuthState, bool, error) {
	var out OAuthState
	var found bool
	ref := f.c.Collection(statesCollection).Doc(state)
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		found = false
		s, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.DataTo(&out); err != nil {
			return err
		}
		found = true
		return tx.Delete(ref)
	})
	return out, found, err
}
