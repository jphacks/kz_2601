package loop

import (
	"context"
	"time"
)

// SourceChecker tells which feedback documents still exist (Firestore).
type SourceChecker interface {
	FeedbackExists(ctx context.Context, ids []string) (map[string]bool, error)
}

// UserChecker tells whether an account is deleted (store.Accounts.UserDeleted).
type UserChecker interface {
	UserDeleted(ctx context.Context, uid string) (bool, error)
}

// SweepResult counts one Sweep.
type SweepResult struct {
	Expired int // documents past expiresAt
	Orphans int // in-app copies whose feedback was erased
	Deleted int // items and notices of deleted accounts
}

// Sweep keeps the loop's copies within the rules of #367 decision 8 and 9:
// everything goes at expiresAt (90 days, never later than the source), an
// in-app copy goes as soon as its feedback/{id} is erased (180 days, or the
// account purge), and the items and notices of an account that was deleted
// go at once (no waiting for the 30-day purge). It never touches feedback/
// or any other collection.
func Sweep(ctx context.Context, s Store, src SourceChecker, users UserChecker, now time.Time, dryRun bool) (SweepResult, error) {
	var res SweepResult
	n, err := s.PurgeExpired(ctx, now, 500, dryRun)
	if err != nil {
		return res, err
	}
	res.Expired = n
	items, err := s.ListItems(ctx, ItemQuery{Limit: 500})
	if err != nil {
		return res, err
	}
	var sourceIDs []string
	bySource := map[string]string{}
	uids := map[string]bool{}
	// gone: items a real run has deleted by the time a later step looks at
	// them, so a dry run does not count them twice.
	gone := map[string]bool{}
	for _, it := range items {
		if dryRun && it.ExpiresAt.Before(now) {
			gone[it.ID] = true
			continue
		}
		if it.Source == SourceInApp {
			sourceIDs = append(sourceIDs, it.SourceID)
			bySource[it.SourceID] = it.ID
		}
		if it.UID != "" {
			uids[it.UID] = true
		}
	}
	if src != nil && len(sourceIDs) > 0 {
		exists, err := src.FeedbackExists(ctx, sourceIDs)
		if err != nil {
			return res, err
		}
		var orphans []string
		for _, sid := range sourceIDs {
			if !exists[sid] {
				orphans = append(orphans, bySource[sid])
				gone[bySource[sid]] = true
			}
		}
		res.Orphans = len(orphans)
		if !dryRun && len(orphans) > 0 {
			if err := s.DeleteItems(ctx, orphans); err != nil {
				return res, err
			}
		}
	}
	if users == nil {
		return res, nil
	}
	for uid := range uids {
		deleted, err := users.UserDeleted(ctx, uid)
		if err != nil {
			return res, err
		}
		if !deleted {
			continue
		}
		its, ns, err := s.ItemsByUID(ctx, uid)
		if err != nil {
			return res, err
		}
		if dryRun {
			for _, id := range its {
				if !gone[id] {
					res.Deleted++
				}
			}
			res.Deleted += len(ns)
			continue
		}
		res.Deleted += len(its) + len(ns)
		if err := s.DeleteItems(ctx, its); err != nil {
			return res, err
		}
		if err := s.DeleteNotices(ctx, ns); err != nil {
			return res, err
		}
	}
	return res, nil
}
