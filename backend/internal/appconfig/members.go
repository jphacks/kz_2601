package appconfig

// Participants of shared apps (#254). Records of scope:"shared" entities are
// read and written only by the app's creator (app_owners) and the members in
// apps_public/{appId}/members/{uid}; scope:"public" stays open to everyone.
// One joins by importing a share link issued by a participant. See
// architecture.md §3.4.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// LegacyMembersEnv: copies of an app created before this time (RFC3339) join
// it as members on first access (migration for apps shared before #254).
// Unset = no grandfathering.
const LegacyMembersEnv = "SHARED_MEMBERS_LEGACY_BEFORE"

// notMemberReason is details.reason on the 403, so clients can tell "join
// with a link" apart from "only the author can change this".
const notMemberReason = "not_member"

func legacyMembersBefore() time.Time {
	v := strings.TrimSpace(os.Getenv(LegacyMembersEnv))
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		slog.Error("appconfig: invalid "+LegacyMembersEnv+"; legacy members disabled", slog.String("value", v))
		return time.Time{}
	}
	return t
}

// hasSharedScope: any entity is scope:"shared" (public alone needs no membership).
func hasSharedScope(a *dsl.App) bool {
	for i := range a.Entities {
		if a.Entities[i].EffectiveScope() == dsl.ScopeShared {
			return true
		}
	}
	return false
}

// hasPublicScope: any entity is scope:"public".
func hasPublicScope(a *dsl.App) bool {
	for i := range a.Entities {
		if a.Entities[i].EffectiveScope() == dsl.ScopePublic {
			return true
		}
	}
	return false
}

func notMember() *apiError {
	return apiErr(http.StatusForbidden, httpx.CodeForbidden, "join this app with a share link to see its shared data").With("reason", notMemberReason)
}

// memberRole returns "owner", "member" or "" (not a participant).
func (h *Handler) memberRole(ctx context.Context, appID, uid string) (string, error) {
	o, err := h.appOwner(ctx, appID)
	if err != nil {
		return "", err
	}
	if o != "" && o == uid {
		return "owner", nil
	}
	if _, err := h.store.GetMember(ctx, appID, uid); err == nil {
		return "member", nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	return "", nil
}

// requireMember: 403 not_member unless the caller participates in appID.
// cur is the caller's own copy; a copy older than LegacyMembersEnv joins.
func (h *Handler) requireMember(r *http.Request, cur *store.App) error {
	ctx, uid := r.Context(), uidOf(r)
	role, err := h.memberRole(ctx, cur.ID, uid)
	if err != nil || role != "" {
		return err
	}
	if !h.legacyBefore.IsZero() && !cur.CreatedAt.IsZero() && cur.CreatedAt.Before(h.legacyBefore) {
		if _, err := h.store.AddMember(ctx, cur.ID, store.Member{UID: uid, Via: store.MemberViaLegacy}); err != nil {
			return err
		}
		slog.Info("appconfig: legacy member joined", slog.String("appId", cur.ID))
		return nil
	}
	return notMember()
}

// join adds uid to appID via a share issued by inviter, who must still
// participate (a removed member's old links stop working).
func (h *Handler) join(r *http.Request, appID, inviter, name string) error {
	ctx, uid := r.Context(), uidOf(r)
	role, err := h.memberRole(ctx, appID, uid)
	if err != nil {
		return err
	}
	if role == "owner" {
		return nil
	}
	if inviter != uid {
		ir, err := h.memberRole(ctx, appID, inviter)
		if err != nil {
			return err
		}
		if ir == "" {
			return apiErr(http.StatusForbidden, httpx.CodeForbidden, "the person who sent this link no longer participates in the app").With("reason", "inviter_left")
		}
	}
	if role == "" {
		ms, err := h.store.ListMembers(ctx, appID, store.MaxMembers)
		if err != nil {
			return err
		}
		if len(ms) >= store.MaxMembers {
			return apiErr(http.StatusForbidden, httpx.CodeForbidden, "this app has too many members").With("reason", "members_full")
		}
	}
	_, err = h.store.AddMember(ctx, appID, store.Member{UID: uid, DisplayName: name, Via: store.MemberViaShare, InvitedBy: inviter})
	return err
}

type memberOut struct {
	UID         string     `json:"uid"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"` // owner | member
	Via         string     `json:"via,omitempty"`
	JoinedAt    *time.Time `json:"joinedAt,omitempty"`
	You         bool       `json:"you"`
}

// listMembers: GET /v1/apps/{appId}/members (participants only).
func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	role, err := h.memberRole(ctx, id, uid)
	if err != nil {
		fail(w, err)
		return
	}
	if role == "" {
		fail(w, notMember())
		return
	}
	owner, err := h.appOwner(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	ms, err := h.store.ListMembers(ctx, id, store.MaxMembers)
	if err != nil {
		fail(w, err)
		return
	}
	items := make([]memberOut, 0, len(ms)+1)
	if owner != "" {
		items = append(items, memberOut{UID: owner, Role: "owner", You: owner == uid})
	}
	for i := range ms {
		m := &ms[i]
		if m.UID == owner {
			continue
		}
		items = append(items, memberOut{UID: m.UID, DisplayName: m.DisplayName, Role: "member", Via: m.Via, JoinedAt: &m.JoinedAt, You: m.UID == uid})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "role": role})
}

// removeMember: DELETE /v1/apps/{appId}/members/{uid}. "me" (or one's own
// uid) leaves; removing someone else is the creator's alone. The creator
// cannot leave (delete the app instead). Idempotent for "me".
func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	target := r.PathValue("uid")
	self := target == "me" || target == uid
	if self {
		target = uid
	} else if !idRe.MatchString(target) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid uid"))
		return
	}
	owner, err := h.appOwner(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	switch {
	case owner != "" && target == owner:
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "the creator cannot leave the app"))
		return
	case !self && owner != uid:
		fail(w, apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the creator of this app can remove members"))
		return
	}
	err = h.store.RemoveMember(ctx, id, target)
	if errors.Is(err, store.ErrNotFound) && self {
		err = nil
	}
	if err != nil {
		fail(w, mapStoreErr(err, "member"))
		return
	}
	slog.Info("appconfig: member removed", slog.String("appId", id), slog.Bool("self", self))
	w.WriteHeader(http.StatusNoContent)
}

// importName reads the optional {"displayName"} body of POST /shares/{token}/import.
func importName(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.ContentLength == 0 {
		return "", true
	}
	var body struct {
		DisplayName string `json:"displayName"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return "", false
	}
	name := strings.TrimSpace(body.DisplayName)
	if utf8.RuneCountInString(name) > maxAuthorNameLen {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "displayName too long"))
		return "", false
	}
	return name, true
}
