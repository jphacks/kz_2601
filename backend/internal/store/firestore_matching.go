package store

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	matchPairsCollection = "match_pairs" // under apps_public/{appId}
	matchQuotaCollection = "match_quota"
)

type matchPairDoc struct {
	UIDs       []string          `firestore:"uids"`
	From       string            `firestore:"from"`
	Status     string            `firestore:"status"`
	DeclinedBy string            `firestore:"declinedBy,omitempty"`
	Profiles   map[string]string `firestore:"profiles"`
	CreatedAt  time.Time         `firestore:"createdAt"`
	UpdatedAt  time.Time         `firestore:"updatedAt"`
	MatchedAt  time.Time         `firestore:"matchedAt,omitempty"`
}

func (f *Firestore) matchPairsCol(appID string) *firestore.CollectionRef {
	return f.publicRef(appID).Collection(matchPairsCollection)
}

func matchPairFrom(s *firestore.DocumentSnapshot) (*MatchPair, error) {
	var d matchPairDoc
	if err := s.DataTo(&d); err != nil {
		return nil, err
	}
	p := &MatchPair{ID: s.Ref.ID, From: d.From, Status: d.Status, DeclinedBy: d.DeclinedBy, Profiles: d.Profiles,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, MatchedAt: d.MatchedAt}
	if len(d.UIDs) == 2 {
		p.UIDs = [2]string{d.UIDs[0], d.UIDs[1]}
	}
	if p.Profiles == nil {
		p.Profiles = map[string]string{}
	}
	return p, nil
}

// ChangeMatchPair implements Matching.
func (f *Firestore) ChangeMatchPair(ctx context.Context, appID, a, b string, fn func(*MatchPair) (*MatchPair, error)) (*MatchPair, error) {
	id := MatchPairID(a, b)
	ref := f.matchPairsCol(appID).Doc(id)
	var out *MatchPair
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		out = nil
		var cur *MatchPair
		snap, err := tx.Get(ref)
		switch {
		case err == nil:
			if cur, err = matchPairFrom(snap); err != nil {
				return err
			}
		case !isNotFound(err):
			return err
		}
		next, err := fn(cur)
		if err != nil {
			return err
		}
		if next == nil {
			out = cur
			return nil
		}
		next = next.clone()
		next.ID, next.UIDs = id, SortedPair(a, b)
		now := Now()
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
		next.UpdatedAt = now
		next.MatchedAt = next.MatchedAt.Truncate(time.Microsecond) // fn may set it from time.Now
		out = next
		return tx.Set(ref, matchPairDoc{UIDs: next.UIDs[:], From: next.From, Status: next.Status, DeclinedBy: next.DeclinedBy,
			Profiles: next.Profiles, CreatedAt: next.CreatedAt, UpdatedAt: next.UpdatedAt, MatchedAt: next.MatchedAt})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListMatchPairs implements Matching (array-contains: automatic single-field index).
func (f *Firestore) ListMatchPairs(ctx context.Context, appID, uid string) ([]MatchPair, error) {
	q := f.matchPairsCol(appID).Where("uids", "array-contains", uid).Limit(MaxMatchPairs)
	var out []MatchPair
	err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		p, err := matchPairFrom(s)
		if err == nil {
			out = append(out, *p)
		}
		return err
	})
	sortMatchPairs(out)
	return out, err
}

// TakeMatchQuota implements Matching.
func (f *Firestore) TakeMatchQuota(ctx context.Context, uid, day string, limit int) error {
	ref := f.c.Collection(matchQuotaCollection).Doc(uid)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		var q quotaDoc
		if snap, err := tx.Get(ref); err == nil {
			if err := snap.DataTo(&q); err != nil {
				return err
			}
		} else if !isNotFound(err) {
			return err
		}
		if q.Day != day {
			q = quotaDoc{Day: day}
		}
		if limit > 0 && q.N >= int64(limit) {
			return ErrRateLimited
		}
		q.N++
		return tx.Set(ref, q)
	})
}

// BlockedEither implements Matching: uid's own block list, plus
// users/{other}/blocks/{uid} read in chunks.
func (f *Firestore) BlockedEither(ctx context.Context, uid string, others []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(others) == 0 {
		return out, nil
	}
	mine, err := f.ListBlocks(ctx, uid)
	if err != nil {
		return nil, err
	}
	blocked := BlockedSet(mine)
	var refs []*firestore.DocumentRef
	for _, o := range others {
		if blocked[o] {
			out[o] = true
			continue
		}
		refs = append(refs, f.blockRef(o, uid))
	}
	for start := 0; start < len(refs); start += markedByChunk {
		snaps, err := f.c.GetAll(ctx, refs[start:min(start+markedByChunk, len(refs))])
		if err != nil {
			return nil, err
		}
		for _, s := range snaps {
			if s.Exists() {
				out[s.Ref.Parent.Parent.ID] = true
			}
		}
	}
	return out, nil
}

// ConfirmAge implements Matching (merged into users/{uid}). The declaration
// is written first: a profile left unmarked by a failure is still shown,
// through the author's declaration (AgeConfirmedAmong).
func (f *Firestore) ConfirmAge(ctx context.Context, uid string) error {
	if _, err := f.userRef(uid).Set(ctx, map[string]any{"ageConfirmedAt": Now()}, firestore.MergeAll); err != nil {
		return err
	}
	return eachRef(f.userRef(uid).Collection("apps").DocumentRefs(ctx), func(app *firestore.DocumentRef) error {
		return f.markProfilesAgeConfirmed(ctx, app.ID, uid)
	})
}

// markProfilesAgeConfirmed sets ownerAgeConfirmed on uid's unmarked records
// in the profile entity of appID's published DSL.
func (f *Firestore) markProfilesAgeConfirmed(ctx context.Context, appID, uid string) error {
	pub, err := f.GetPublicApp(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	eid := profileEntityID(pub.DSL)
	if eid == "" {
		return nil
	}
	return each(ctx, f.recordsCol(PublicRecords(appID, eid)).Where("author", "==", uid), func(s *firestore.DocumentSnapshot) error {
		if v, _ := s.DataAt("ownerAgeConfirmed"); v == true {
			return nil
		}
		_, err := s.Ref.Update(ctx, []firestore.Update{{Path: "ownerAgeConfirmed", Value: true}})
		if isNotFound(err) {
			return nil // deleted meanwhile
		}
		return err
	})
}

// AgeConfirmedAmong implements Matching: users/{uid} read in chunks.
func (f *Firestore) AgeConfirmedAmong(ctx context.Context, uids []string) (map[string]bool, error) {
	out := map[string]bool{}
	refs := make([]*firestore.DocumentRef, 0, len(uids))
	for _, u := range uids {
		refs = append(refs, f.userRef(u))
	}
	for start := 0; start < len(refs); start += markedByChunk {
		snaps, err := f.c.GetAll(ctx, refs[start:min(start+markedByChunk, len(refs))])
		if err != nil {
			return nil, err
		}
		for _, s := range snaps {
			if s.Exists() && ageConfirmedIn(s) {
				out[s.Ref.ID] = true
			}
		}
	}
	return out, nil
}

func ageConfirmedIn(s *firestore.DocumentSnapshot) bool {
	v, _ := s.DataAt("ageConfirmedAt")
	at, ok := v.(time.Time)
	return ok && !at.IsZero()
}

// AgeConfirmed implements Matching.
func (f *Firestore) AgeConfirmed(ctx context.Context, uid string) (bool, error) {
	snap, err := f.userRef(uid).Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ageConfirmedIn(snap), nil
}

// purgeMatching erases uid's pairs in appID and uid's records in the
// profile / matched entities of the published DSL (account purge).
func (f *Firestore) purgeMatching(ctx context.Context, appID, uid string, res *AccountPurgeResult) error {
	n, err := f.deleteQuery(ctx, f.matchPairsCol(appID).Where("uids", "array-contains", uid), nil)
	res.MatchPairs += n
	if err != nil {
		return err
	}
	pub, err := f.GetPublicApp(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range matchEntityIDs(pub.DSL) {
		err := each(ctx, f.recordsCol(PublicRecords(appID, e)).Where("author", "==", uid), func(s *firestore.DocumentSnapshot) error {
			if _, err := f.deleteTree(ctx, s.Ref); err != nil {
				return err
			}
			res.MatchRecords++
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
