package store

import "context"

// Accounts is the account-deletion part of Store (DELETE /v1/me, #119).
type Accounts interface {
	// DeleteUserData physically deletes everything uid owns:
	//
	//   - users/{uid} and all its subcollections (apps, versions, records)
	//   - shared records uid posted (apps_public/*/entities/*/records with author == uid);
	//     other users' posts in the same app are kept
	//   - apps_public/{appId} documents owned by uid (the published DSL only:
	//     other users' records in its subcollections are kept)
	//   - shares/* and public_apps/* owned by uid
	//
	// It is idempotent: calling it again (also after a partial failure) deletes
	// whatever is left and returns nil when nothing is.
	DeleteUserData(ctx context.Context, uid string) error
}
