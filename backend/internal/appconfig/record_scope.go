package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// publishedDSL decodes appID's published DSL (apps_public). nil, nil, nil
// when the app is not published.
func (h *Handler) publishedDSL(ctx context.Context, appID string) (*dsl.App, *store.PublicApp, error) {
	pub, err := h.store.GetPublicApp(ctx, appID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	key := "pub/" + appID + "/" + strconv.Itoa(pub.Version) + "/" + pub.UpdatedAt.Format(time.RFC3339Nano)
	h.mu.Lock()
	a, ok := h.cache[key]
	h.mu.Unlock()
	if !ok {
		// decode only: an older mirror that today's rules would reject must
		// not break every shared record list
		a = &dsl.App{}
		if err := json.Unmarshal(pub.DSL, a); err != nil {
			return nil, pub, err
		}
		h.mu.Lock()
		if len(h.cache) > 256 {
			h.cache = map[string]*dsl.App{}
		}
		h.cache[key] = a
		h.mu.Unlock()
	}
	return a, pub, nil
}

// notPublishedReason is details.reason when a shared entity of the caller's
// copy is not part of the creator's published DSL.
const notPublishedReason = "not_published"

// sharingRules decides the sharing settings (scope, edit, marks) of one
// app's entities for the caller. Records of shared entities live once per
// app, so they follow the DSL the app's creator published (apps_public),
// not the caller's copy; the copy still supplies the field definitions.
// The creator's own copy decides for the creator (publish skips a DSL with
// no shared entity, so the mirror may lag behind it).
type sharingRules struct {
	creator bool     // the caller created the app
	pub     *dsl.App // the creator's published DSL; nil = none usable
}

// sharingRulesFor loads the rules for the caller on app cur.
func (h *Handler) sharingRulesFor(r *http.Request, cur *store.App) (*sharingRules, error) {
	ctx := r.Context()
	owner, err := h.appOwner(ctx, cur.ID)
	if err != nil {
		return nil, err
	}
	if owner != "" && owner == uidOf(r) {
		return &sharingRules{creator: true}, nil
	}
	a, pub, err := h.publishedDSL(ctx, cur.ID)
	if err != nil {
		return nil, err
	}
	if a == nil || owner == "" || pub.OwnerUID != owner {
		return &sharingRules{}, nil // only the creator's mirror counts
	}
	return &sharingRules{pub: a}, nil
}

// entity returns own (an entity of the caller's copy) with the published
// sharing settings. A shared entity the published DSL lacks is refused.
func (s *sharingRules) entity(own *dsl.Entity) (*dsl.Entity, error) {
	if s.creator {
		return own, nil
	}
	var pe *dsl.Entity
	if s.pub != nil {
		pe = s.pub.EntityByID(own.ID)
	}
	if pe == nil {
		if own.IsShared() {
			return nil, apiErr(http.StatusForbidden, httpx.CodeForbidden, "this shared data is not available in the app").With("reason", notPublishedReason)
		}
		return own, nil
	}
	c := *own
	c.Scope, c.Edit, c.Marks = pe.Scope, pe.Edit, pe.Marks
	return &c, nil
}

// hasSharedScope reports whether any entity is scope:"shared" for the
// caller, counting both the copy and the published DSL.
func (s *sharingRules) hasSharedScope(own *dsl.App) bool {
	return hasSharedScope(own) || (!s.creator && s.pub != nil && hasSharedScope(s.pub))
}

// withSharing is sharingRulesFor(...).entity(own) for one entity.
func (h *Handler) withSharing(r *http.Request, cur *store.App, own *dsl.Entity) (*dsl.Entity, error) {
	s, err := h.sharingRulesFor(r, cur)
	if err != nil {
		return nil, err
	}
	return s.entity(own)
}
