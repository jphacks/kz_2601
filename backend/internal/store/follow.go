package store

import (
	"context"
	"errors"
)

// Follows (#270): whom each person follows inside one shared app. One
// document per follower keeps "my follows" a single read; the list is
// filtered in memory (no Firestore `in` query, so its 30-value cap does not
// apply) and bounded by MaxFollows.
//
//	apps_public/{appId}/follows/{uid} {uids: [followed uid, ...] (oldest first), updatedAt}
//
// docs/backend/features/follows.md.
type Follows interface {
	// SetFollow adds (on) or removes target in uid's follows of appID and
	// returns how many people uid follows afterwards. Idempotent.
	// ErrFollowsFull when adding past MaxFollows.
	SetFollow(ctx context.Context, appID, uid, target string, on bool) (int, error)
	// ListFollows returns whom uid follows in appID, oldest first (nil when none).
	ListFollows(ctx context.Context, appID, uid string) ([]string, error)
}

// MaxFollows bounds whom one person follows in one app (one document).
const MaxFollows = 500

// ErrFollowsFull is SetFollow adding past MaxFollows.
var ErrFollowsFull = errors.New("store: too many follows")

// nextFollows applies one SetFollow to cur. changed is false for a no-op.
func nextFollows(cur []string, target string, on bool) (next []string, changed bool, err error) {
	at := -1
	for i, u := range cur {
		if u == target {
			at = i
			break
		}
	}
	switch {
	case on && at >= 0, !on && at < 0:
		return cur, false, nil
	case on && len(cur) >= MaxFollows:
		return cur, false, ErrFollowsFull
	case on:
		return append(append([]string{}, cur...), target), true, nil
	}
	return append(append([]string{}, cur[:at]...), cur[at+1:]...), true, nil
}
