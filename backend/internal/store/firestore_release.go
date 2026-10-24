package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Layout (architecture.md §3.3):
//
//	release_requests/{id}  {appId,ownerUid,name,dslVersion,dslJson,purpose,contact,status,createdAt,updatedAt}
//
// status is changed by hand in the Firestore console; the API only writes
// "received". Queries use equality filters only (ownerUid [+ appId]), which
// the automatic single-field indexes serve without a composite index.
const releaseCollection = "release_requests"

type releaseDoc struct {
	AppID     string    `firestore:"appId"`
	OwnerUID  string    `firestore:"ownerUid"`
	Name      string    `firestore:"name"`
	Version   int64     `firestore:"dslVersion"`
	DSLJSON   string    `firestore:"dslJson"`
	Purpose   string    `firestore:"purpose"`
	Contact   string    `firestore:"contact"`
	Status    string    `firestore:"status"`
	CreatedAt time.Time `firestore:"createdAt"`
	UpdatedAt time.Time `firestore:"updatedAt"`
}

func (d *releaseDoc) toRelease(id string) *ReleaseRequest {
	return &ReleaseRequest{ID: id, AppID: d.AppID, OwnerUID: d.OwnerUID, Name: d.Name, Version: int(d.Version),
		Purpose: d.Purpose, Contact: d.Contact, Status: d.Status, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

func (f *Firestore) releaseQuery(uid, appID string) firestore.Query {
	q := f.c.Collection(releaseCollection).Where("ownerUid", "==", uid)
	if appID != "" {
		q = q.Where("appId", "==", appID)
	}
	return q.Limit(MaxReleaseRequestsPerUser)
}

func readReleases(it *firestore.DocumentIterator) ([]ReleaseRequest, error) {
	defer it.Stop()
	out := []ReleaseRequest{}
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		var d releaseDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toRelease(snap.Ref.ID))
	}
	sortReleases(out)
	return out, nil
}

// CreateReleaseRequest implements Store. The open-request check and the
// create run in one transaction so a double tap cannot file two requests.
func (f *Firestore) CreateReleaseRequest(ctx context.Context, rr ReleaseRequest) (*ReleaseRequest, bool, error) {
	var (
		out     *ReleaseRequest
		created bool
	)
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		out, created = nil, false
		existing, err := readReleases(tx.Documents(f.releaseQuery(rr.OwnerUID, rr.AppID)))
		if err != nil {
			return err
		}
		for i := range existing {
			if ReleaseOpen(existing[i].Status) {
				out = &existing[i]
				return nil
			}
		}
		now := time.Now().UTC()
		id := NewID("rr_")
		d := releaseDoc{AppID: rr.AppID, OwnerUID: rr.OwnerUID, Name: rr.Name, Version: int64(rr.Version),
			DSLJSON: string(rr.DSL), Purpose: rr.Purpose, Contact: rr.Contact, Status: ReleaseReceived,
			CreatedAt: now, UpdatedAt: now}
		out, created = d.toRelease(id), true
		return tx.Create(f.c.Collection(releaseCollection).Doc(id), d)
	})
	if err != nil {
		return nil, false, err
	}
	return out, created, nil
}

// ListReleaseRequests implements Store.
func (f *Firestore) ListReleaseRequests(ctx context.Context, uid, appID string) ([]ReleaseRequest, error) {
	return readReleases(f.releaseQuery(uid, appID).Documents(ctx))
}
