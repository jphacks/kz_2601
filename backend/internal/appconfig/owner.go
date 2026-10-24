package appconfig

// Who may publish an appId (#153). GET /dsl seeds any appId into the caller's
// own tree (#47 / #93: opening an appId = joining it), so "the app is in my
// tree" does not mean "I made it". The creator is recorded in app_owners on
// the first real commit (generate / PUT / PATCH / restore / a new install
// copy); seeds and joined installs never claim.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// reservedAppIDs are what buggy clients send for a missing value (#154).
var reservedAppIDs = map[string]bool{"null": true, "undefined": true, "nil": true, "none": true, "nan": true, "true": true, "false": true}

func validAppID(s string) bool {
	return idRe.MatchString(s) && !reservedAppIDs[strings.ToLower(s)]
}

// appOwner returns appID's creator, "" when unknown. Apps created before
// app_owners existed fall back to the catalog listing's owner, then to the
// shared mirror's (apps_public) owner, and that answer is recorded.
func (h *Handler) appOwner(ctx context.Context, appID string) (string, error) {
	o, err := h.store.AppOwner(ctx, appID)
	if err == nil {
		return o, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	legacy := ""
	if c, err := h.store.GetCatalogApp(ctx, appID); err == nil {
		legacy = c.OwnerUID
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if legacy == "" {
		if p, err := h.store.GetPublicApp(ctx, appID); err == nil {
			legacy = p.OwnerUID
		} else if !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
	}
	if legacy == "" {
		return "", nil
	}
	return h.store.ClaimApp(ctx, legacy, appID)
}

// claim records the caller as appID's creator after a real commit, unless a
// creator is already known. Best effort: the commit itself already succeeded,
// and publish retries the claim.
func (h *Handler) claim(r *http.Request, appID string) {
	ctx := r.Context()
	o, err := h.appOwner(ctx, appID)
	if err == nil && o == "" {
		_, err = h.store.ClaimApp(ctx, uidOf(r), appID)
	}
	if err != nil {
		slog.Warn("appconfig: owner claim failed", slog.String("appId", appID), slog.Any("error", err))
	}
}

// requireCreator: 403 unless the caller created appID, 400 when the caller's
// copy is still blank. An appId nobody has claimed yet (a legacy app not
// committed since) is claimed by the caller, but never with a blank copy.
func (h *Handler) requireCreator(r *http.Request, appID string, app *dsl.App) error {
	ctx, uid := r.Context(), uidOf(r)
	o, err := h.appOwner(ctx, appID)
	if err != nil {
		return err
	}
	forbidden := apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the creator of this app can publish it")
	if o != "" && o != uid {
		return forbidden
	}
	if isBlank(app) {
		return blankErr("publishing")
	}
	if o == "" {
		if o, err = h.store.ClaimApp(ctx, uid, appID); err != nil {
			return err
		}
		if o != uid {
			return forbidden
		}
	}
	return nil
}

// isBlank: nothing generated yet (dsl.Initial is a lone voice input).
func isBlank(a *dsl.App) bool {
	return len(a.Entities) == 0 && len(componentTypes(a)) == 0
}

func blankErr(what string) error {
	return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "this app has no content yet; generate it before "+what)
}
