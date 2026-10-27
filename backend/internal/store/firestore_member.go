package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type memberDoc struct {
	DisplayName string    `firestore:"displayName,omitempty"`
	Via         string    `firestore:"via"`
	InvitedBy   string    `firestore:"invitedBy,omitempty"`
	JoinedAt    time.Time `firestore:"joinedAt"`
}

func (f *Firestore) membersCol(appID string) *firestore.CollectionRef {
	return f.publicRef(appID).Collection("members")
}

func memberFrom(snap *firestore.DocumentSnapshot) (*Member, error) {
	var d memberDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return &Member{UID: snap.Ref.ID, DisplayName: d.DisplayName, Via: d.Via, InvitedBy: d.InvitedBy, JoinedAt: d.JoinedAt}, nil
}

// AddMember implements Store. One transaction, so a concurrent re-join keeps
// the first JoinedAt.
func (f *Firestore) AddMember(ctx context.Context, appID string, m Member) (bool, error) {
	ref := f.membersCol(appID).Doc(m.UID)
	var created bool
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		created = false
		_, err := tx.Get(ref)
		switch {
		case err == nil:
			if m.DisplayName == "" {
				return nil
			}
			return tx.Update(ref, []firestore.Update{{Path: "displayName", Value: m.DisplayName}})
		case !isNotFound(err):
			return err
		}
		created = true
		joined := m.JoinedAt
		if joined.IsZero() {
			joined = time.Now().UTC()
		}
		return tx.Create(ref, memberDoc{DisplayName: m.DisplayName, Via: m.Via, InvitedBy: m.InvitedBy, JoinedAt: joined})
	})
	return created, err
}

// GetMember implements Store.
func (f *Firestore) GetMember(ctx context.Context, appID, uid string) (*Member, error) {
	snap, err := f.membersCol(appID).Doc(uid).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return memberFrom(snap)
}

// ListMembers implements Store (single-field order: no composite index).
func (f *Firestore) ListMembers(ctx context.Context, appID string, limit int) ([]Member, error) {
	q := f.membersCol(appID).OrderBy("joinedAt", firestore.Asc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	var out []Member
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		m, err := memberFrom(snap)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
}

// RemoveMember implements Store.
func (f *Firestore) RemoveMember(ctx context.Context, appID, uid string) error {
	_, err := f.membersCol(appID).Doc(uid).Delete(ctx, firestore.Exists)
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}
