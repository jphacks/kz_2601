package appconfig

// Follows (#270): people follow each other inside one shared app, and a list
// shows only the posts of the people the caller follows. The relation is the
// same for every app (store.Follows: apps_public/{appId}/follows/{uid}); the
// DSL only reads it (filter value {"myFollows": true}, action follow/unfollow).
//
//	GET    /v1/apps/{appId}/follows        -> {uids: [...]} whom the caller follows (oldest first)
//	PUT    /v1/apps/{appId}/follows/{uid}  -> {following: true,  count}
//	DELETE /v1/apps/{appId}/follows/{uid}  -> {following: false, count}
//	GET    .../records?filterField=author&filterOp=in|nin&filterValue=myFollows
//
// Who may follow: anyone who can read the app's shared data (scope:"shared"
// = its participants). Following is private to the follower: nobody is told,
// and there is no follower list. docs/backend/features/follows.md.

import (
	"errors"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// routes (#270)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/apps/{appId}/follows", h.listFollows)
		mux.HandleFunc("PUT /v1/apps/{appId}/follows/{uid}", h.putFollow)
		mux.HandleFunc("DELETE /v1/apps/{appId}/follows/{uid}", h.deleteFollow)
	})
}

// myFollowsValue is the filterValue that stands for the caller's follows.
const myFollowsValue = "myFollows"

// details.reason values of follow errors.
const (
	reasonNoSharedData = "no_shared_data" // the app has no published shared/public entity
	reasonFollowSelf   = "follow_self"
	reasonFollowsFull  = "follows_full"
	reasonNotInApp     = "not_in_app" // a scope:"shared" app: the person does not participate
)

// followApp resolves {appId} of a follows route. onlyMembers reports that
// the app's shared data is participants-only (no scope:"public" entity).
func (h *Handler) followApp(r *http.Request) (appID string, onlyMembers bool, err error) {
	if appID, err = pathID(r, "appId"); err != nil {
		return "", false, err
	}
	a, pub, err := h.publishedDSL(r.Context(), appID)
	if err != nil {
		return "", false, err
	}
	if a == nil || !hasSharedEntity(a) {
		return "", false, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this app has no shared data to follow people in").With("reason", reasonNoSharedData)
	}
	if pub.Hidden {
		return "", false, unpublishedErr("this shared app", pub.ModerationState)
	}
	if onlyMembers = !hasPublicScope(a); onlyMembers {
		role, err := h.memberRole(r.Context(), appID, uidOf(r))
		if err != nil {
			return "", false, err
		}
		if role == "" {
			return "", false, notMember()
		}
	}
	return appID, onlyMembers, nil
}

func hasSharedEntity(a *dsl.App) bool {
	for i := range a.Entities {
		if a.Entities[i].IsShared() {
			return true
		}
	}
	return false
}

func (h *Handler) listFollows(w http.ResponseWriter, r *http.Request) {
	appID, _, err := h.followApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	uids, err := h.store.ListFollows(r.Context(), appID, uidOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	if uids == nil {
		uids = []string{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"uids": uids})
}

func (h *Handler) putFollow(w http.ResponseWriter, r *http.Request)    { h.setFollow(w, r, true) }
func (h *Handler) deleteFollow(w http.ResponseWriter, r *http.Request) { h.setFollow(w, r, false) }

// setFollow is idempotent both ways. Unfollowing always works (even someone
// who left the app), so a stale entry can be taken back.
func (h *Handler) setFollow(w http.ResponseWriter, r *http.Request, on bool) {
	appID, onlyMembers, err := h.followApp(r)
	if err != nil {
		fail(w, err)
		return
	}
	target, err := pathID(r, "uid")
	if err != nil {
		fail(w, err)
		return
	}
	me := uidOf(r)
	if on && target == me {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "you cannot follow yourself").With("reason", reasonFollowSelf))
		return
	}
	if on && onlyMembers {
		role, err := h.memberRole(r.Context(), appID, target)
		if err != nil {
			fail(w, err)
			return
		}
		if role == "" {
			fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this person does not take part in the app").With("reason", reasonNotInApp))
			return
		}
	}
	n, err := h.store.SetFollow(r.Context(), appID, me, target, on)
	if errors.Is(err, store.ErrFollowsFull) {
		fail(w, apiErr(http.StatusForbidden, httpx.CodeForbidden, "you follow too many people in this app").With("reason", reasonFollowsFull).With("max", store.MaxFollows))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"following": on, "count": n})
}

// recordFilter is buildFilter plus "posts of people I follow" on shared
// entities: filterField=author, filterOp=in (nin = not followed),
// filterValue=myFollows. The follows are one document, so the filter runs in
// memory like the others (no Firestore `in` query and its 30-value cap).
func (h *Handler) recordFilter(r *http.Request, appID string, ent *dsl.Entity, field, op, value string) (func(*store.Record) bool, error) {
	if field != authorField || !ent.IsShared() || (op != "in" && op != "nin") {
		return buildFilter(ent, field, op, value)
	}
	if value != myFollowsValue {
		return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "filterOp in / nin for author takes filterValue "+myFollowsValue)
	}
	uids, err := h.store.ListFollows(r.Context(), appID, uidOf(r))
	if err != nil {
		return nil, err
	}
	followed := make(map[string]bool, len(uids))
	for _, u := range uids {
		followed[u] = true
	}
	want := op == "in"
	return func(rec *store.Record) bool { return followed[rec.Author] == want }, nil
}
