package store

import (
	"context"

	"cloud.google.com/go/firestore"
)

// ProfileAgeBackfill counts one BackfillProfileAgeFlags run.
type ProfileAgeBackfill struct {
	Apps       int // published apps with a match:"profile" entity
	Profiles   int // profile records read
	Marked     int // already carried ownerAgeConfirmed
	ToMark     int // authors declared, flag missing (set unless dry run)
	Undeclared int // authors have not declared: left as they are (hidden)
}

// profilesToMark picks the records whose author declared but that do not
// carry OwnerAgeConfirmed yet. declared holds the declared authors.
func profilesToMark(recs []Record, declared map[string]bool, c *ProfileAgeBackfill) []string {
	var ids []string
	for i := range recs {
		switch {
		case recs[i].OwnerAgeConfirmed:
			c.Marked++
		case recs[i].Author != "" && declared[recs[i].Author]:
			c.ToMark++
			ids = append(ids, recs[i].ID)
		default:
			c.Undeclared++
		}
	}
	return ids
}

// unmarkedAuthors lists the distinct authors of records without the flag.
func unmarkedAuthors(recs []Record) []string {
	seen := map[string]bool{}
	var out []string
	for i := range recs {
		a := recs[i].Author
		if !recs[i].OwnerAgeConfirmed && a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// BackfillProfileAgeFlags sets ownerAgeConfirmed on the profile records of
// every published matching app whose author has made the 18+ declaration
// (records made before the flag existed). Records of authors who have not
// declared are left without it. dryRun counts only. Running it again finds
// nothing new to set. cmd/matching-age-backfill.
func (f *Firestore) BackfillProfileAgeFlags(ctx context.Context, dryRun bool) (ProfileAgeBackfill, error) {
	var c ProfileAgeBackfill
	err := each(ctx, f.c.Collection(publicCollection).Query, func(s *firestore.DocumentSnapshot) error {
		var d publicAppDoc
		if err := s.DataTo(&d); err != nil {
			return err
		}
		eid := profileEntityID(d.toPublicApp(s.Ref.ID).DSL)
		if eid == "" {
			return nil
		}
		c.Apps++
		rs := PublicRecords(s.Ref.ID, eid)
		recs, err := f.ListRecords(ctx, rs, 0)
		if err != nil {
			return err
		}
		c.Profiles += len(recs)
		declared, err := f.AgeConfirmedAmong(ctx, unmarkedAuthors(recs))
		if err != nil {
			return err
		}
		for _, id := range profilesToMark(recs, declared, &c) {
			if dryRun {
				continue
			}
			_, err := f.recRef(rs, id).Update(ctx, []firestore.Update{{Path: "ownerAgeConfirmed", Value: true}})
			if err != nil && !isNotFound(err) {
				return err
			}
		}
		return nil
	})
	return c, err
}
