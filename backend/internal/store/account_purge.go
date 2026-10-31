package store

import (
	"context"
	"errors"
	"time"
)

// AccountPurged is the users/{uid} status once PurgeAccount and the Firebase
// Auth deletion are done. The document stays as a tombstone {status, purgedAt}
// without deletedAt, so the uid keeps getting 410 and DeletedAccounts never
// returns it again.
const AccountPurged = "purged"

// PurgeAfter is how long a deleted account is kept before its data is erased
// (privacy policy §4: "30 日以内に消去").
const PurgeAfter = 30 * 24 * time.Hour

// ErrNotDeleted is returned by PurgeAccount for a uid that has not called
// DELETE /v1/me (a guard: active accounts are never purged).
var ErrNotDeleted = errors.New("account is not deleted")

// DeletedAccount is one candidate for PurgeAccount.
type DeletedAccount struct {
	UID       string
	DeletedAt time.Time
}

// AccountPurgeResult counts what PurgeAccount deleted or changed.
type AccountPurgeResult struct {
	Apps          int // users/{uid}/apps/* erased (with versions and private records)
	KeptMirrors   int // uid's apps_public mirrors kept because other people take part
	Shares        int // shares/* issued by uid
	Listings      int // public_apps/* of uid (with installs)
	Releases      int // release_requests/* of uid (they hold a contact address)
	Owners        int // app_owners/* claimed by uid
	Installs      int // public_apps/*/installs/{uid}
	Memberships   int // apps_public/*/members/{uid}
	Marks         int // apps_public/.../marks/*/by/{uid} (counts are decremented)
	PostsScrubbed int // uid's posts in shared apps whose authorName was cleared
	Reports       int // reports/* filed by uid or about uid (#321), with report_quota/{uid}
	Blocks        int // users/{uid}/blocks/*
}

// AccountPurger physically erases accounts that DELETE /v1/me soft-deleted (#458).
//
// Kept on purpose (privacy policy §4: "ほかの人と共有しているアプリへの投稿は残ります"):
//   - uid's posts in apps_public/* (shared/public records) stay, but their
//     authorName is cleared; author keeps the uid, which no longer maps to a
//     Firebase user.
//   - an apps_public mirror owned by uid stays when someone else is a member
//     or has posted there (it is those people's shared space), and so does its
//     app_owners entry. Without others it is erased with the app.
type AccountPurger interface {
	// DeletedAccounts returns accounts soft-deleted before cutoff, oldest
	// first, up to limit (purged tombstones are not returned).
	DeletedAccounts(ctx context.Context, cutoff time.Time, limit int) ([]DeletedAccount, error)
	// PurgeAccount erases uid's data. Idempotent: a retry after a partial
	// failure finishes the rest. users/{uid} itself is left for
	// MarkAccountPurged. ErrNotDeleted when uid is not deleted.
	PurgeAccount(ctx context.Context, uid string) (AccountPurgeResult, error)
	// MarkAccountPurged turns users/{uid} into the purged tombstone.
	MarkAccountPurged(ctx context.Context, uid string) error
}
