package appconfig

// Matching apps (#507): people in a shared app find peers by interest and
// connect only when both approve. The roles of entities come from the
// published DSL (apps_public/{appId}), never from the caller's own copy:
//   - match:"profile": one card per person; people who blocked each other
//     (either way, #321) do not see each other's.
//   - only people who declared they are 18 or older (matching_age.go).
//   - match:"matched": read only by the author and the people matched with
//     the author.
//
// Interests and matches are apps_public/{appId}/match_pairs (store.Matching).
// A decline is never shown to the sender: their interest stays "pending".
// docs/backend/features/matching.md.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// routes (matching #507)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/apps/{appId}/interests", h.listInterests)
		mux.HandleFunc("POST /v1/apps/{appId}/interests", h.sendInterest)
		mux.HandleFunc("POST /v1/apps/{appId}/interests/{uid}/accept", h.acceptInterest)
		mux.HandleFunc("DELETE /v1/apps/{appId}/interests/{uid}", h.declineInterest)
		mux.HandleFunc("GET /v1/apps/{appId}/matches", h.listMatches)
		mux.HandleFunc("DELETE /v1/apps/{appId}/matches/{uid}", h.unmatch)
	})
}

// InterestsPerDay caps sent interests per person per JST day (all apps).
const InterestsPerDay = 30

// details.reason values of matching errors.
const (
	reasonAgeUnconfirmed = "age_unconfirmed" // the 18+ confirmation is missing
	reasonBlocked        = "blocked"         // the caller blocked that person
	reasonNoMatching     = "no_matching"     // the app has no match:"profile" entity
)

// matchSpec is the matching part of a published DSL.
type matchSpec struct {
	profile *dsl.Entity
	matched map[string]bool
}

// publishedMatch reads the matching roles of appID's published DSL (nil, nil
// when the app is not published or has no matching).
func (h *Handler) publishedMatch(ctx context.Context, appID string) (*matchSpec, *store.PublicApp, error) {
	a, pub, err := h.publishedDSL(ctx, appID)
	if a == nil || err != nil {
		return nil, pub, err
	}
	if !a.HasMatching() {
		return nil, pub, nil
	}
	spec := &matchSpec{profile: a.ProfileEntity(), matched: map[string]bool{}}
	for _, id := range a.MatchedEntities() {
		spec.matched[id] = true
	}
	return spec, pub, nil
}

// matchApp resolves {appId} of a matching endpoint: the app must have a
// profile entity, must not be unpublished, and on scope:"shared" the caller
// must participate.
func (h *Handler) matchApp(r *http.Request) (string, *matchSpec, error) {
	appID, err := pathID(r, "appId")
	if err != nil {
		return "", nil, err
	}
	spec, pub, err := h.publishedMatch(r.Context(), appID)
	if err != nil {
		return "", nil, err
	}
	if spec == nil || spec.profile == nil {
		return "", nil, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this app has no matching").With("reason", reasonNoMatching)
	}
	if pub.Hidden {
		return "", nil, unpublishedErr("this shared app", pub.ModerationState)
	}
	if spec.profile.EffectiveScope() == dsl.ScopeShared {
		role, err := h.memberRole(r.Context(), appID, uidOf(r))
		if err != nil {
			return "", nil, err
		}
		if role == "" {
			return "", nil, notMember()
		}
	}
	return appID, spec, nil
}

// matchFilter returns which authors' records of entity eid the caller may
// read by the matching rules (nil = no rule; blocks: sharedFilter). Without
// the 18+ declaration only the caller's own.
func (h *Handler) matchFilter(r *http.Request, appID, eid string) (func(author string) bool, error) {
	spec, _, err := h.publishedMatch(r.Context(), appID)
	if err != nil || spec == nil {
		return nil, err
	}
	me := uidOf(r)
	isProfile := spec.profile != nil && spec.profile.ID == eid
	if !isProfile && !spec.matched[eid] {
		return nil, nil
	}
	adult, err := h.store.AgeConfirmed(r.Context(), me)
	if err != nil {
		return nil, err
	}
	switch {
	case !adult:
		return func(a string) bool { return a == me }, nil
	case isProfile:
		return nil, nil
	}
	ok, err := h.matchedWith(r, appID)
	if err != nil {
		return nil, err
	}
	return func(a string) bool { return a == me || ok[a] }, nil
}

// matchedWith returns the uids matched with the caller, minus blocks both ways.
func (h *Handler) matchedWith(r *http.Request, appID string) (map[string]bool, error) {
	ctx, me := r.Context(), uidOf(r)
	pairs, err := h.store.ListMatchPairs(ctx, appID, me)
	if err != nil {
		return nil, err
	}
	var others []string
	for i := range pairs {
		if pairs[i].Status == store.MatchMatched {
			others = append(others, pairs[i].Other(me))
		}
	}
	blocked, err := h.store.BlockedEither(ctx, me, others)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, o := range others {
		if !blocked[o] {
			out[o] = true
		}
	}
	return out, nil
}

func recordAuthors(rs []store.Record) []string {
	out := make([]string, 0, len(rs))
	for i := range rs {
		out = append(out, rs[i].Author)
	}
	return out
}

func othersOf(uids []string, me string) []string {
	seen := map[string]bool{me: true}
	var out []string
	for _, u := range uids {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// ---- views --------------------------------------------------------------------

type interestOut struct {
	UID             string     `json:"uid"`
	ProfileRecordID string     `json:"profileRecordId,omitempty"`
	Status          string     `json:"status"` // pending | matched
	UpdatedAt       time.Time  `json:"updatedAt"`
	MatchedAt       *time.Time `json:"matchedAt,omitempty"`
}

// pairView is how me sees p: box incoming / outgoing / match, or "" (not
// shown). A decline or block by the other side still reads as outgoing
// pending for the sender, so nobody learns they were turned down.
func pairView(p *store.MatchPair, me string) string {
	switch {
	case p.Status == store.MatchMatched:
		return "match"
	case p.Status == store.MatchPending && p.From == me:
		return "outgoing"
	case p.Status == store.MatchPending:
		return "incoming"
	case p.Status == store.MatchDeclined && p.From == me && p.DeclinedBy != me && p.MatchedAt.IsZero():
		return "outgoing"
	}
	return ""
}

func toInterestOut(p *store.MatchPair, me string) interestOut {
	o := p.Other(me)
	out := interestOut{UID: o, ProfileRecordID: p.Profiles[o], Status: "pending", UpdatedAt: p.UpdatedAt}
	if pairView(p, me) == "match" {
		out.Status = "matched"
		t := p.MatchedAt
		out.MatchedAt = &t
	}
	return out
}

// pairsIn lists the caller's pairs in box, without people blocked either way.
func (h *Handler) pairsIn(r *http.Request, appID, box string) ([]interestOut, error) {
	ctx, me := r.Context(), uidOf(r)
	pairs, err := h.store.ListMatchPairs(ctx, appID, me)
	if err != nil {
		return nil, err
	}
	var hit []*store.MatchPair
	var others []string
	for i := range pairs {
		if pairView(&pairs[i], me) == box {
			hit = append(hit, &pairs[i])
			others = append(others, pairs[i].Other(me))
		}
	}
	blocked, err := h.store.BlockedEither(ctx, me, others)
	if err != nil {
		return nil, err
	}
	items := make([]interestOut, 0, len(hit))
	for _, p := range hit {
		if !blocked[p.Other(me)] {
			items = append(items, toInterestOut(p, me))
		}
	}
	return items, nil
}

// ---- handlers -----------------------------------------------------------------

func ageUnconfirmed() *apiError {
	return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "confirm that you are 18 or older to connect with people").With("reason", reasonAgeUnconfirmed)
}

// sendInterest: POST /v1/apps/{appId}/interests
//
//	{"recordId":"<their profile>","fromRecordId":"<my profile>","ageConfirmed":true}
//
// When they already sent one to the caller, this is the approval and the
// pair is matched.
func (h *Handler) sendInterest(w http.ResponseWriter, r *http.Request) {
	appID, spec, err := h.matchApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		RecordID     string `json:"recordId"`
		FromRecordID string `json:"fromRecordId"`
		AgeConfirmed bool   `json:"ageConfirmed"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if err := h.requireAge(r, body.AgeConfirmed); err != nil {
		fail(w, err)
		return
	}
	if !idRe.MatchString(body.RecordID) || !idRe.MatchString(body.FromRecordID) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "recordId and fromRecordId are required"))
		return
	}
	ctx, me := r.Context(), uidOf(r)
	rs := store.PublicRecords(appID, spec.profile.ID)
	theirs, err := h.store.GetRecord(ctx, rs, body.RecordID)
	if err != nil || theirs.Author == "" {
		fail(w, mapStoreErr(store.ErrNotFound, "profile"))
		return
	}
	target := theirs.Author
	if target == me {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "cannot send an interest to yourself"))
		return
	}
	mine, err := h.store.GetRecord(ctx, rs, body.FromRecordID)
	if err != nil || mine.Author != me {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "fromRecordId must be your own profile").With("field", "fromRecordId"))
		return
	}
	blocked, err := h.store.BlockedEither(ctx, me, []string{target})
	if err != nil {
		fail(w, err)
		return
	}
	if blocked[target] {
		mineBlocks, err := h.blockedBy(r)
		if err != nil {
			fail(w, err)
			return
		}
		if mineBlocks[target] {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unblock this person first").With("reason", reasonBlocked))
			return
		}
	}
	day := time.Now().In(ratelimit.JST).Format(time.DateOnly)
	if err := h.store.TakeMatchQuota(ctx, me, day, InterestsPerDay); errors.Is(err, store.ErrRateLimited) {
		fail(w, apiErr(http.StatusTooManyRequests, httpx.CodeRateLimited, "too many interests today"))
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	theyBlocked := blocked[target]
	p, err := h.store.ChangeMatchPair(ctx, appID, me, target, func(cur *store.MatchPair) (*store.MatchPair, error) {
		fresh := func() *store.MatchPair {
			p := &store.MatchPair{From: me, Status: store.MatchPending, Profiles: map[string]string{me: body.FromRecordID, target: body.RecordID}}
			if cur != nil {
				p.CreatedAt = cur.CreatedAt
			}
			if theyBlocked { // looks sent to the caller; never reaches them
				p.Status, p.DeclinedBy = store.MatchDeclined, target
			}
			return p
		}
		switch {
		case cur == nil:
			return fresh(), nil
		case cur.Status == store.MatchPending && cur.From == target && !theyBlocked:
			cur.Status, cur.MatchedAt = store.MatchMatched, store.Now()
			cur.Profiles[me] = body.FromRecordID
			return cur, nil
		case cur.Status == store.MatchDeclined && cur.DeclinedBy == me:
			return fresh(), nil // the caller changes their mind
		case cur.Status == store.MatchDeclined && cur.From == target && cur.DeclinedBy == target && cur.MatchedAt.IsZero():
			return fresh(), nil // they withdrew their own interest
		}
		return nil, nil // already sent / matched / declined by them
	})
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toInterestOut(p, me))
}

// listInterests: GET /v1/apps/{appId}/interests?box=incoming|outgoing.
func (h *Handler) listInterests(w http.ResponseWriter, r *http.Request) {
	appID, _, err := h.matchApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := h.requireAge(r, false); err != nil {
		fail(w, err)
		return
	}
	box := r.URL.Query().Get("box")
	if box == "" {
		box = "incoming"
	}
	if box != "incoming" && box != "outgoing" {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "box must be incoming or outgoing"))
		return
	}
	items, err := h.pairsIn(r, appID, box)
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// errNoInterest: the pair is not in the state the call needs (404).
var errNoInterest = apiErr(http.StatusNotFound, httpx.CodeNotFound, "no such interest")

// otherOf reads {uid} and refuses people blocked either way as "not found".
func (h *Handler) otherOf(r *http.Request) (string, error) {
	other := r.PathValue("uid")
	if !idRe.MatchString(other) || other == uidOf(r) {
		return "", apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid uid")
	}
	blocked, err := h.store.BlockedEither(r.Context(), uidOf(r), []string{other})
	if err != nil {
		return "", err
	}
	if blocked[other] {
		return "", errNoInterest
	}
	return other, nil
}

// acceptInterest: POST /v1/apps/{appId}/interests/{uid}/accept {"ageConfirmed":true}.
func (h *Handler) acceptInterest(w http.ResponseWriter, r *http.Request) {
	appID, _, err := h.matchApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		AgeConfirmed bool `json:"ageConfirmed"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if err := h.requireAge(r, body.AgeConfirmed); err != nil {
		fail(w, err)
		return
	}
	other, err := h.otherOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	me := uidOf(r)
	p, err := h.store.ChangeMatchPair(r.Context(), appID, me, other, func(cur *store.MatchPair) (*store.MatchPair, error) {
		if cur == nil || cur.Status != store.MatchPending || cur.From != other {
			return nil, errNoInterest
		}
		cur.Status, cur.MatchedAt = store.MatchMatched, store.Now()
		return cur, nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toInterestOut(p, me))
}

// declineInterest: DELETE /v1/apps/{appId}/interests/{uid}. Declines their
// pending interest or withdraws the caller's own; the other side is not told.
// Idempotent.
func (h *Handler) declineInterest(w http.ResponseWriter, r *http.Request) {
	h.endPair(w, r, store.MatchPending, false)
}

// unmatch: DELETE /v1/apps/{appId}/matches/{uid}. Either side ends a match;
// the matched-only records stop showing both ways.
func (h *Handler) unmatch(w http.ResponseWriter, r *http.Request) {
	h.endPair(w, r, store.MatchMatched, true)
}

// endPair turns a pair in state `from` into declined by the caller. strict:
// 404 when it is not in that state (else 204 regardless).
func (h *Handler) endPair(w http.ResponseWriter, r *http.Request, from string, strict bool) {
	appID, _, err := h.matchApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	other := r.PathValue("uid")
	if !idRe.MatchString(other) || other == uidOf(r) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid uid"))
		return
	}
	me := uidOf(r)
	_, err = h.store.ChangeMatchPair(r.Context(), appID, me, other, func(cur *store.MatchPair) (*store.MatchPair, error) {
		if cur == nil || cur.Status != from {
			if strict {
				return nil, errNoInterest
			}
			return nil, nil
		}
		cur.Status, cur.DeclinedBy = store.MatchDeclined, me
		return cur, nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listMatches: GET /v1/apps/{appId}/matches.
func (h *Handler) listMatches(w http.ResponseWriter, r *http.Request) {
	appID, _, err := h.matchApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := h.requireAge(r, false); err != nil {
		fail(w, err)
		return
	}
	items, err := h.pairsIn(r, appID, "match")
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
