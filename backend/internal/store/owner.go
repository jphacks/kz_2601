package store

import "context"

// Ownership records who created an appId (#153) and who installed a catalog
// listing (#154). Both are keyed globally, like public_apps.
//
//	app_owners/{appId}                {uid,createdAt}
//	public_apps/{id}/installs/{uid}   {appId,createdAt}
type Ownership interface {
	// ClaimApp records uid as appID's creator unless one is already recorded,
	// and returns the recorded creator (uid when this call claimed it).
	ClaimApp(ctx context.Context, uid, appID string) (owner string, err error)
	// AppOwner returns the recorded creator, ErrNotFound when none.
	AppOwner(ctx context.Context, appID string) (string, error)

	// GetInstall returns the appId uid got by installing listing id (ErrNotFound).
	GetInstall(ctx context.Context, id, uid string) (appID string, err error)
	// RecordInstall stores uid's install of listing id as appID. installs is
	// incremented only on uid's first install of id and only when count is set.
	RecordInstall(ctx context.Context, id, uid, appID string, count bool) (first bool, err error)
}
