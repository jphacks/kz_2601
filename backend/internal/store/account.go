package store

import "context"

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
	//
	// Shared records uid posted (apps_public/...) are kept. Idempotent: the
	// mark is written first, so a retry after a partial failure finishes the rest.
	DeactivateUser(ctx context.Context, uid string) error
	// UserDeleted reports whether DeactivateUser has been called for uid
	// (still true after the account was purged).
	UserDeleted(ctx context.Context, uid string) (bool, error)
}
