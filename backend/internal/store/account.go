package store

import (
	"context"
	"time"
)

// Account status written to users/{uid} by DeactivateUser.
const AccountDeleted = "deleted"

// Accounts is the account-deletion part of Store (DELETE /v1/me, #119).
// Deletion is logical here; AccountPurger (cmd/purge) erases the data
// PurgeAfter later (#458).
type Accounts interface {
	// DeactivateUser marks users/{uid} {status: "deleted", deletedAt} (the
	// first deletedAt is kept) and hides what others could reach:
	//
	//   - every users/{uid}/apps/{appId} gets status "archived"
	//   - shares/* owned by uid are revoked
	//   - public_apps/* (the "探す" listings) owned by uid are taken down
	//   - users/{uid}/devices/* are deleted (#337: pushes stop right away)
	//
	// Shared records uid posted (apps_public/...) are kept. Idempotent: the
	// mark is written first, so a retry after a partial failure finishes the rest.
	DeactivateUser(ctx context.Context, uid string) error
	// UserDeleted reports whether DeactivateUser has been called for uid
	// (still true after the account was purged).
	UserDeleted(ctx context.Context, uid string) (bool, error)
	// SeeUser is UserDeleted for a signed-in request (appconfig.RejectDeleted):
	// an active uid seen for the first time gets users/{uid}.firstSeenAt, set
	// once and never overwritten (#339 account age; re-sign-in keeps it).
	// Nothing is written into a deleted or purged account.
	SeeUser(ctx context.Context, uid string) (deleted bool, err error)
	// UserFirstSeen reads users/{uid}.firstSeenAt; known=false when absent
	// (MarkAccountPurged drops it with the rest of the document).
	UserFirstSeen(ctx context.Context, uid string) (at time.Time, known bool, err error)
}
