package appconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

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

// sharingOf returns ent with the sharing settings (scope, edit, marks) of
// the published DSL, which the app's publisher decides. The publisher and
// apps nobody published keep their own. An entity the published DSL does not
// share stays private.
func sharingOf(ent *dsl.Entity, pub *dsl.App, publisher, uid string) *dsl.Entity {
	if pub == nil || publisher == uid {
		return ent
	}
	c := *ent
	c.Scope, c.Edit, c.Marks = dsl.ScopePrivate, "", nil
	if p := pub.EntityByID(ent.ID); p != nil && p.IsShared() {
		c.Scope, c.Edit, c.Marks = p.EffectiveScope(), p.Edit, p.Marks
	}
	return &c
}

// withSharing applies sharingOf for the caller of r.
func (h *Handler) withSharing(r *http.Request, appID string, ent *dsl.Entity) (*dsl.Entity, error) {
	a, pub, err := h.publishedDSL(r.Context(), appID)
	if err != nil {
		return nil, err
	}
	if pub == nil {
		return ent, nil
	}
	return sharingOf(ent, a, pub.OwnerUID, uidOf(r)), nil
}
