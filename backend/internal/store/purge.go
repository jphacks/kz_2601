package store

import "context"

// PurgeResult is what PurgeApp physically deleted.
type PurgeResult struct {
	Shares   int  // shares/{token} uid issued for the app
	Listing  bool // public_apps/{appId} (+ installs/*)
	Mirror   bool // apps_public/{appId} (+ entities/*/records/*)
	Versions int  // users/{uid}/apps/{appId}/versions/*
	Records  int  // users/{uid}/apps/{appId}/entities/*/records/*
}

// Purger physically deletes an app (#180). Firestore does not delete
// subcollections with their parent, so implementations walk them explicitly.
type Purger interface {
	// PurgeApp deletes users/{uid}/apps/{appID} and everything under it,
	// after what uid made public for it: uid's share links, uid's catalog
	// listing (+ installs) and uid's apps_public mirror (+ shared records).
	// Only documents whose ownerUid is uid are touched: another uid's listing
	// or mirror and other users' trees stay (installed copies have their own
	// appId; members who joined the appId keep their own copy).
	// The user's tree goes last, so a failed purge can be retried.
	// ErrNotFound when uid has no such app.
	PurgeApp(ctx context.Context, uid, appID string) (PurgeResult, error)
}
