package store

// Test data cleanup (#293, cmd/stg-cleanup): erases what cmd/smoke made and
// what named stg test users made. Targets are only
//   - appIds matching SmokeAppIDRe (the exact shape cmd/smoke generates), and
//   - uids the operator lists explicitly.
// There is no broader selector on purpose: nothing here can reach a real
// user's data unless its uid is passed in.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"cloud.google.com/go/firestore"
)

// SmokeAppIDRe matches the appIds cmd/smoke makes: smoke-<unix seconds> and
// its -training / -walk / -probe / -noauth variants.
var SmokeAppIDRe = regexp.MustCompile(`^smoke-[0-9]{9,11}(-(training|walk|probe|noauth))?$`)

// ErrNotSmoke: the appId is not one cmd/smoke makes.
var ErrNotSmoke = errors.New("not a smoke appId")

// smokeIDLow/High bound the document-ID range that holds every "smoke-..." id
// ('.' is the byte after '-'), so queries read only candidates.
const smokeIDLow, smokeIDHigh = "smoke-", "smoke."

// SmokeApp is one users/{uid}/apps/{appId} that cmd/smoke made (or a copy
// someone opened with GET /dsl). Docs counts the documents its deletion
// removes, per collection id ("apps" = the app document itself).
type SmokeApp struct {
	UID   string
	AppID string
	Docs  map[string]int
}

// SmokeData is what FindSmokeData found.
type SmokeData struct {
	Apps   []SmokeApp
	Owners []string // app_owners/{appId} entries for smoke appIds (with or without an app)
}

// FindSmokeData lists smoke apps in every users/{uid}/apps and the matching
// app_owners entries. Read only. Queries skip "missing" app documents (only
// subcollections left); cmd/smoke never leaves that shape. users is listed with DocumentRefs, so uids
// whose users/{uid} document does not exist (anonymous smoke users) count.
func (f *Firestore) FindSmokeData(ctx context.Context) (SmokeData, error) {
	var out SmokeData
	err := eachRef(f.c.Collection("users").DocumentRefs(ctx), func(u *firestore.DocumentRef) error {
		q := smokeRange(u.Collection("apps"))
		return each(ctx, q.Select(), func(s *firestore.DocumentSnapshot) error {
			app := s.Ref
			if !SmokeAppIDRe.MatchString(app.ID) {
				return nil
			}
			counts := map[string]int{}
			if _, err := collectTree(ctx, app, nil, counts); err != nil {
				return err
			}
			counts["apps"]++
			out.Apps = append(out.Apps, SmokeApp{UID: u.ID, AppID: app.ID, Docs: counts})
			return nil
		})
	})
	if err != nil {
		return out, err
	}
	q := smokeRange(f.c.Collection(ownersCollection))
	err = each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		if SmokeAppIDRe.MatchString(s.Ref.ID) {
			out.Owners = append(out.Owners, s.Ref.ID)
		}
		return nil
	})
	return out, err
}

// smokeRange is col's documents whose id starts with "smoke-".
func smokeRange(col *firestore.CollectionRef) firestore.Query {
	return col.Where(firestore.DocumentID, ">=", col.Doc(smokeIDLow)).Where(firestore.DocumentID, "<", col.Doc(smokeIDHigh))
}

// PurgeSmokeApp deletes one smoke app (PurgeApp: the user's tree and what
// that uid published for it). Refuses an appId that is not a smoke id.
func (f *Firestore) PurgeSmokeApp(ctx context.Context, uid, appID string) (PurgeResult, error) {
	if !SmokeAppIDRe.MatchString(appID) {
		return PurgeResult{}, ErrNotSmoke
	}
	res, err := f.PurgeApp(ctx, uid, appID)
	if errors.Is(err, ErrNotFound) {
		err = nil // already gone (a retry)
	}
	return res, err
}

// DeleteSmokeOwner deletes app_owners/{appId} for a smoke appId.
func (f *Firestore) DeleteSmokeOwner(ctx context.Context, appID string) error {
	if !SmokeAppIDRe.MatchString(appID) {
		return ErrNotSmoke
	}
	_, err := f.ownerRef(appID).Delete(ctx)
	return err
}

// TestUserSummary is what PurgeTestUser would remove for one uid (main
// collections; PurgeAccount's result has the exact counts).
type TestUserSummary struct {
	UID    string
	Exists bool   // users/{uid} document exists
	Status string // users/{uid}.status ("" = active)
	Docs   map[string]int
}

// SummarizeTestUser counts uid's data. Read only.
func (f *Firestore) SummarizeTestUser(ctx context.Context, uid string) (TestUserSummary, error) {
	sum := TestUserSummary{UID: uid, Docs: map[string]int{}}
	user := f.userRef(uid)
	snap, err := user.Get(ctx)
	switch {
	case err == nil:
		sum.Exists = true
		sum.Status, _ = snap.Data()["status"].(string)
	case !isNotFound(err):
		return sum, err
	}
	if _, err := collectTree(ctx, user, nil, sum.Docs); err != nil {
		return sum, err
	}
	queries := []struct {
		name  string
		q     firestore.Query
		dedup bool
	}{
		{sharesCollection, f.c.Collection(sharesCollection).Where("ownerUid", "==", uid), false},
		{catalogCollection, f.c.Collection(catalogCollection).Where("ownerUid", "==", uid), false},
		{publicCollection, f.c.Collection(publicCollection).Where("ownerUid", "==", uid), false},
		{releaseCollection, f.c.Collection(releaseCollection).Where("ownerUid", "==", uid), false},
		{feedbackCollection, f.c.Collection(feedbackCollection).Where("uid", "==", uid), false},
		{ownersCollection, f.c.Collection(ownersCollection).Where("uid", "==", uid), false},
		{reportsCollection, f.c.Collection(reportsCollection).Where("reporterUid", "==", uid), true},
		{reportsCollection, f.c.Collection(reportsCollection).Where("targetUid", "==", uid), true},
		{"appstore_transactions", f.c.Collection("appstore_transactions").Where("uid", "==", uid), false},
	}
	seen := map[string]bool{}
	for _, c := range queries {
		err := each(ctx, c.q, func(s *firestore.DocumentSnapshot) error {
			if c.dedup {
				if seen[s.Ref.Path] {
					return nil
				}
				seen[s.Ref.Path] = true
			}
			sum.Docs[c.name]++
			return nil
		})
		if err != nil {
			return sum, err
		}
	}
	for _, col := range []string{reportQuotaCollection, matchQuotaCollection} {
		if _, err := f.c.Collection(col).Doc(uid).Get(ctx); err == nil {
			sum.Docs[col]++
		} else if !isNotFound(err) {
			return sum, err
		}
	}
	return sum, nil
}

// PurgeTestUser erases a test user's data as an account purge would
// (soft-delete mark -> PurgeAccount), then deletes users/{uid} itself instead
// of leaving the purged tombstone, so the uid can be used again as a fresh
// user. Entitlements and App Store bindings are the caller's (entitlement
// package) and must be purged before the final delete is retried.
// Idempotent.
func (f *Firestore) PurgeTestUser(ctx context.Context, uid string) (AccountPurgeResult, error) {
	if err := f.markDeleted(ctx, uid); err != nil {
		return AccountPurgeResult{}, err
	}
	return f.PurgeAccount(ctx, uid)
}

// DeleteUserTree deletes users/{uid} and anything left under it. Called by
// cmd/stg-cleanup after PurgeTestUser and the entitlement purge.
func (f *Firestore) DeleteUserTree(ctx context.Context, uid string) (int, error) {
	counts, err := f.deleteTree(ctx, f.userRef(uid))
	n := 0
	for _, c := range counts {
		n += c
	}
	return n, err
}

// StgRateLimitIDRe matches the counters internal/ratelimit writes with
// APP_ENV=stg (rateLimits/stg_<yyyy-mm-dd>_{uid_<uid>|ip_<hash>|global}).
var StgRateLimitIDRe = regexp.MustCompile(`^stg_[0-9]{4}-[0-9]{2}-[0-9]{2}_(uid_.+|ip_[0-9a-f]+|global)$`)

const rateLimitsCollection = "rateLimits" // ratelimit.DefaultCollection

// FindStgRateLimits lists the stg rate-limit counters. Read only.
func (f *Firestore) FindStgRateLimits(ctx context.Context) ([]string, error) {
	col := f.c.Collection(rateLimitsCollection)
	q := col.Where(firestore.DocumentID, ">=", col.Doc("stg_")).Where(firestore.DocumentID, "<", col.Doc("stg`")).Select()
	var ids []string
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		if StgRateLimitIDRe.MatchString(s.Ref.ID) {
			ids = append(ids, s.Ref.ID)
		}
		return nil
	})
	return ids, err
}

// DeleteStgRateLimits deletes the listed stg counters (others are refused).
func (f *Firestore) DeleteStgRateLimits(ctx context.Context, ids []string) error {
	refs := make([]*firestore.DocumentRef, 0, len(ids))
	for _, id := range ids {
		if !StgRateLimitIDRe.MatchString(id) {
			return fmt.Errorf("%q: not a stg rate-limit counter", id)
		}
		refs = append(refs, f.c.Collection(rateLimitsCollection).Doc(id))
	}
	return f.bulkDelete(ctx, refs)
}

// SortedKeys returns m's keys in order (stable output for logs and tests).
func SortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
