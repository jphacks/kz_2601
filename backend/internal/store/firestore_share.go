package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Layout (architecture.md §3.2):
//
//	shares/{token}        {appId,ownerUid,name,dslVersion,dslJson,revoked,createdAt}
//	public_apps/{appId}   {ownerUid,name,description,tags,authorName,components,dslVersion,dslJson,installs,createdAt,updatedAt}
const (
	sharesCollection  = "shares"
	catalogCollection = "public_apps"
)

type shareDoc struct {
	AppID     string    `firestore:"appId"`
	OwnerUID  string    `firestore:"ownerUid"`
	Name      string    `firestore:"name"`
	Version   int64     `firestore:"dslVersion"`
	DSLJSON   string    `firestore:"dslJson"`
	Revoked   bool      `firestore:"revoked"`
	CreatedAt time.Time `firestore:"createdAt"`
}

type catalogDoc struct {
	OwnerUID    string    `firestore:"ownerUid"`
	Name        string    `firestore:"name"`
	Description string    `firestore:"description"`
	Tags        []string  `firestore:"tags"`
	AuthorName  string    `firestore:"authorName"`
	Components  []string  `firestore:"components"`
	Version     int64     `firestore:"dslVersion"`
	DSLJSON     string    `firestore:"dslJson"`
	Installs    int64     `firestore:"installs"`
	CreatedAt   time.Time `firestore:"createdAt"`
	UpdatedAt   time.Time `firestore:"updatedAt"`
}

func (d *catalogDoc) toCatalog(id string, withDSL bool) *CatalogApp {
	c := &CatalogApp{ID: id, OwnerUID: d.OwnerUID, Name: d.Name, Description: d.Description, Tags: d.Tags,
		AuthorName: d.AuthorName, Components: d.Components, Version: int(d.Version), Installs: int(d.Installs),
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	if withDSL {
		c.DSL = json.RawMessage(d.DSLJSON)
	}
	return c
}

func (f *Firestore) shareRef(token string) *firestore.DocumentRef {
	return f.c.Collection(sharesCollection).Doc(token)
}

func (f *Firestore) catalogRef(id string) *firestore.DocumentRef {
	return f.c.Collection(catalogCollection).Doc(id)
}

// CreateShare implements Store.
func (f *Firestore) CreateShare(ctx context.Context, s Share) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	_, err := f.shareRef(s.Token).Create(ctx, shareDoc{AppID: s.AppID, OwnerUID: s.OwnerUID, Name: s.Name,
		Version: int64(s.Version), DSLJSON: string(s.DSL), Revoked: s.Revoked, CreatedAt: s.CreatedAt})
	if status.Code(err) == codes.AlreadyExists {
		return ErrAlreadyExists
	}
	return err
}

// GetShare implements Store.
func (f *Firestore) GetShare(ctx context.Context, token string) (*Share, error) {
	snap, err := f.shareRef(token).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d shareDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return &Share{Token: token, AppID: d.AppID, OwnerUID: d.OwnerUID, Name: d.Name, Version: int(d.Version),
		DSL: json.RawMessage(d.DSLJSON), Revoked: d.Revoked, CreatedAt: d.CreatedAt}, nil
}

// RevokeShare implements Store.
func (f *Firestore) RevokeShare(ctx context.Context, token string) error {
	_, err := f.shareRef(token).Update(ctx, []firestore.Update{{Path: "revoked", Value: true}})
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}

// PutCatalogApp implements Store.
func (f *Firestore) PutCatalogApp(ctx context.Context, uid string, c CatalogApp) (*CatalogApp, error) {
	ref := f.catalogRef(c.ID)
	var out *CatalogApp
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		now := time.Now().UTC()
		d := catalogDoc{OwnerUID: uid, Name: c.Name, Description: c.Description, Tags: c.Tags, AuthorName: c.AuthorName,
			Components: c.Components, Version: int64(c.Version), DSLJSON: string(c.DSL), CreatedAt: now, UpdatedAt: now}
		snap, err := tx.Get(ref)
		switch {
		case err == nil:
			var old catalogDoc
			if err := snap.DataTo(&old); err != nil {
				return err
			}
			if old.OwnerUID != uid {
				return ErrForbidden
			}
			d.CreatedAt, d.Installs = old.CreatedAt, old.Installs
		case !isNotFound(err):
			return err
		}
		out = d.toCatalog(c.ID, false)
		return tx.Set(ref, d)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetCatalogApp implements Store.
func (f *Firestore) GetCatalogApp(ctx context.Context, id string) (*CatalogApp, error) {
	snap, err := f.catalogRef(id).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d catalogDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toCatalog(id, true), nil
}

// DeleteCatalogApp implements Store.
func (f *Firestore) DeleteCatalogApp(ctx context.Context, uid, id string) error {
	ref := f.catalogRef(id)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var d catalogDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		if d.OwnerUID != uid {
			return ErrForbidden
		}
		return tx.Delete(ref)
	})
}

// ListCatalogApps implements Store. installs desc uses the automatic
// single-field index; ties are broken in the handler.
func (f *Firestore) ListCatalogApps(ctx context.Context, limit int) ([]CatalogApp, error) {
	q := f.c.Collection(catalogCollection).OrderBy("installs", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	var out []CatalogApp
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var d catalogDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toCatalog(snap.Ref.ID, false))
	}
	return out, nil
}

// IncrementInstalls implements Store.
func (f *Firestore) IncrementInstalls(ctx context.Context, id string) error {
	_, err := f.catalogRef(id).Update(ctx, []firestore.Update{{Path: "installs", Value: firestore.Increment(1)}})
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}
