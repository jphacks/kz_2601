package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Matching apps (#507): one document per pair of people in a shared app
// records who sent an interest and whether both approved.
//
//	apps_public/{appId}/match_pairs/{pairId} {uids,from,status,declinedBy,profiles,createdAt,updatedAt,matchedAt}
//	match_quota/{uid}                         {day,n}  daily cap on sent interests
//
// docs/backend/features/matching.md.

// MatchPair.Status values.
const (
	MatchPending  = "pending"  // From sent an interest; the other has not answered
	MatchMatched  = "matched"  // both approved
	MatchDeclined = "declined" // DeclinedBy declined, withdrew from a match, or blocked
)

// MaxMatchPairs bounds the pairs read for one person in one app.
const MaxMatchPairs = 500

// MatchPair is apps_public/{appId}/match_pairs/{ID}.
type MatchPair struct {
	ID         string
	UIDs       [2]string // sorted
	From       string    // who sent the first interest
	Status     string
	DeclinedBy string
	// Profiles maps each uid to the id of their profile record.
	Profiles  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
	MatchedAt time.Time // zero until matched
}

// MatchPairID is the same for (a, b) and (b, a).
func MatchPairID(a, b string) string {
	if b < a {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte(a + "\x00" + b))
	return "mp_" + hex.EncodeToString(sum[:16])
}

// SortedPair orders two uids as MatchPair.UIDs.
func SortedPair(a, b string) [2]string {
	if b < a {
		return [2]string{b, a}
	}
	return [2]string{a, b}
}

// Other returns the uid of the pair that is not uid.
func (p *MatchPair) Other(uid string) string {
	if p.UIDs[0] == uid {
		return p.UIDs[1]
	}
	return p.UIDs[0]
}

func (p *MatchPair) clone() *MatchPair {
	cp := *p
	cp.Profiles = make(map[string]string, len(p.Profiles))
	for k, v := range p.Profiles {
		cp.Profiles[k] = v
	}
	return &cp
}

// Matching stores interests and matches of matching apps (#507).
type Matching interface {
	// ChangeMatchPair reads the pair of a and b in appID (nil when absent),
	// lets fn decide, and writes what fn returns in one transaction. fn
	// returning nil keeps the pair as it is. The result is the stored pair
	// (nil when there is none).
	ChangeMatchPair(ctx context.Context, appID, a, b string, fn func(cur *MatchPair) (*MatchPair, error)) (*MatchPair, error)
	// ListMatchPairs returns up to MaxMatchPairs pairs that include uid, newest first.
	ListMatchPairs(ctx context.Context, appID, uid string) ([]MatchPair, error)
	// TakeMatchQuota counts one sent interest of uid on day (a "2006-01-02"
	// key). ErrRateLimited once limit were sent that day (limit 0 = no cap).
	TakeMatchQuota(ctx context.Context, uid, day string, limit int) error
	// BlockedEither returns the uids among others that uid blocked or that
	// blocked uid (#321 blocks, both ways).
	BlockedEither(ctx context.Context, uid string, others []string) (map[string]bool, error)
}

// matchEntityIDs lists the profile and matched entities of a published DSL.
// The account purge erases the leaver's records there instead of keeping
// them like other posts: a profile and contact info describe the person.
func matchEntityIDs(raw json.RawMessage) []string {
	var a dsl.App
	if len(raw) == 0 || json.Unmarshal(raw, &a) != nil {
		return nil
	}
	return a.MatchEntities()
}
