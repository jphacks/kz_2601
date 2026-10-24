package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ownersCollection = "app_owners"

type ownerDoc struct {
	UID       string    `firestore:"uid"`
	CreatedAt time.Time `firestore:"createdAt"`
}

type installDoc struct {
	AppID     string    `firestore:"appId"`
	CreatedAt time.Time `firestore:"createdAt"`
}

func (f *Firestore) ownerRef(appID string) *firestore.DocumentRef {
	return f.c.Collection(ownersCollection).Doc(appID)
}

func (f *Firestore) installRef(id, uid string) *firestore.DocumentRef {
	return f.catalogRef(id).Collection("installs").Doc(uid)
}

// ClaimApp implements Store. Create is atomic, so concurrent claims agree.
func (f *Firestore) ClaimApp(ctx context.Context, uid, appID string) (string, error) {
	_, err := f.ownerRef(appID).Create(ctx, ownerDoc{UID: uid, CreatedAt: time.Now().UTC()})
	if err == nil {
		return uid, nil
	}
	if status.Code(err) != codes.AlreadyExists {
		return "", err
	}
	return f.AppOwner(ctx, appID)
}

// AppOwner implements Store.
func (f *Firestore) AppOwner(ctx context.Context, appID string) (string, error) {
	snap, err := f.ownerRef(appID).Get(ctx)
	if isNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var d ownerDoc
	if err := snap.DataTo(&d); err != nil {
		return "", err
	}
	return d.UID, nil
}

// GetInstall implements Store.
func (f *Firestore) GetInstall(ctx context.Context, id, uid string) (string, error) {
	snap, err := f.installRef(id, uid).Get(ctx)
	if isNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var d installDoc
	if err := snap.DataTo(&d); err != nil {
		return "", err
	}
	return d.AppID, nil
}

// RecordInstall implements Store. The install marker and the counter move in
// one transaction, so concurrent installs by the same uid count once.
func (f *Firestore) RecordInstall(ctx context.Context, id, uid, appID string, count bool) (bool, error) {
	ref, cat := f.installRef(id, uid), f.catalogRef(id)
	var first bool
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		first = false
		_, err := tx.Get(ref)
		switch {
		case err == nil:
			return tx.Update(ref, []firestore.Update{{Path: "appId", Value: appID}})
		case !isNotFound(err):
			return err
		}
		first = true
		bump := false
		if count {
			if _, err := tx.Get(cat); err == nil {
				bump = true
			} else if !isNotFound(err) {
				return err
			}
		}
		if err := tx.Set(ref, installDoc{AppID: appID, CreatedAt: time.Now().UTC()}); err != nil {
			return err
		}
		if bump {
			return tx.Update(cat, []firestore.Update{{Path: "installs", Value: firestore.Increment(1)}})
		}
		return nil
	})
	return first, err
}
