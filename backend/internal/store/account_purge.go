package store

import (
	"context"
	"errors"
	"log/slog"
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
//
// One field per feature, each with its comment on the line above (no
// trailing comments: gofmt would re-align the whole block on every new field).
// A new count also goes into LogAttrs.
type AccountPurgeResult struct {
	// users/{uid}/apps/* erased (with versions and private records)
	Apps int
	// uid's apps_public mirrors kept because other people take part
	KeptMirrors int
	// shares/* issued by uid
	Shares int
	// public_apps/* of uid (with installs)
	Listings int
	// release_requests/* of uid (they hold a contact address)
	Releases int
	// app_owners/* claimed by uid
	Owners int
	// public_apps/*/installs/{uid}
	Installs int
	// apps_public/*/members/{uid}
	Memberships int
	// apps_public/.../marks/*/by/{uid} (counts are decremented)
	Marks int
	// uid's posts in shared apps whose authorName was cleared
	PostsScrubbed int
	// feedback/* sent by uid (#319)
	Feedback int
	// reports/* filed by uid or about uid (#321), with report_quota/{uid}
	Reports int
	// users/{uid}/blocks/*
	Blocks int
	// apps_public/*/match_pairs/* that include uid (#507), with match_quota/{uid}
	MatchPairs int
	// uid's records in match profile / matched entities (#507: erased, not kept as posts)
	MatchRecords int
}

// LogAttrs returns the counts as slog attrs, one per line, for the
// "purge: account purged" log. The keys are read by the Cloud Logging queries
// in docs/infra/account-purge.md: append new ones, never rename.
func (r AccountPurgeResult) LogAttrs() []any {
	return []any{
		slog.Int("apps", r.Apps),
		slog.Int("keptMirrors", r.KeptMirrors),
		slog.Int("shares", r.Shares),
		slog.Int("listings", r.Listings),
		slog.Int("releases", r.Releases),
		slog.Int("owners", r.Owners),
		slog.Int("installs", r.Installs),
		slog.Int("memberships", r.Memberships),
		slog.Int("marks", r.Marks),
		slog.Int("postsScrubbed", r.PostsScrubbed),
		slog.Int("reports", r.Reports),
		slog.Int("blocks", r.Blocks),
		slog.Int("feedback", r.Feedback),
		slog.Int("matchPairs", r.MatchPairs),
		slog.Int("matchRecords", r.MatchRecords),
	}
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
//
// Not kept: uid's records in matching apps' profile / matched entities and
// uid's match pairs (#507). They describe the person, not a shared post.
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
