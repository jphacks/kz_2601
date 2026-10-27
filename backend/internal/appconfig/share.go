package appconfig

// Share links (#49) and the public catalog (#50). Both hand out a frozen DSL
// snapshot that the receiver copies into their own tree (installCopy); records
// are never copied. See architecture.md §3.2.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Catalog field limits.
const (
	maxListingName = 60
	maxListingDesc = 300
	maxTags        = 8
	maxTagLen      = 20
	maxCardIcons   = 8

	// ImportScheme is the iOS URL scheme (ios/Metamon/Resources/Info.plist).
	ImportScheme = "tailorit://import/"
)

func mapShareErr(err error, what string) error {
	if errors.Is(err, store.ErrForbidden) {
		return apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the owner can change this "+what)
	}
	return mapStoreErr(err, what)
}

// newShareToken is 24 random bytes (192 bit) in base64url: unguessable and
// within idRe.
func newShareToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// newAppID matches what iOS generates for appIds (lowercase UUID v4).
func newAppID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// webBase is where GET /s/{token} is served: SHARE_WEB_BASE_URL, else this host.
func webBase(r *http.Request) string {
	if v := strings.TrimSuffix(os.Getenv("SHARE_WEB_BASE_URL"), "/"); v != "" {
		return v
	}
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	} else if r.TLS == nil && (strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// componentTypes lists distinct non-container component types in DSL order.
func componentTypes(a *dsl.App) []string {
	seen := map[dsl.ComponentType]bool{}
	out := []string{}
	var walk func([]dsl.Component)
	walk = func(cs []dsl.Component) {
		for i := range cs {
			c := &cs[i]
			if dsl.ContainerTypes[c.Type] {
				walk(c.Children)
				continue
			}
			if c.Type == dsl.CompVoiceInput || seen[c.Type] || len(out) >= maxCardIcons {
				continue
			}
			seen[c.Type] = true
			out = append(out, string(c.Type))
		}
	}
	for i := range a.Screens {
		walk(a.Screens[i].Components)
	}
	return out
}

// ---- share links ------------------------------------------------------------

type shareOut struct {
	Token      string    `json:"token"`
	AppID      string    `json:"appId"`
	Name       string    `json:"name"`
	DSLVersion int       `json:"dslVersion"`
	URL        string    `json:"url"`
	WebURL     string    `json:"webUrl"`
	Revoked    bool      `json:"revoked"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toShareOut(r *http.Request, s *store.Share) shareOut {
	return shareOut{Token: s.Token, AppID: s.AppID, Name: s.Name, DSLVersion: s.Version,
		URL: ImportScheme + s.Token, WebURL: webBase(r) + "/s/" + s.Token, Revoked: s.Revoked, CreatedAt: s.CreatedAt}
}

// createShare: POST /v1/apps/{appId}/share. Only apps in the caller's own tree
// can be shared. Unlike publish this is not creator-only: a share is keyed by
// a fresh token (nothing global to squat) and members of a shared app may
// invite others to it (#118). A blank copy (e.g. a bare GET /dsl seed) is 400.
func (h *Handler) createShare(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	app, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	parsed, err := h.parsedDSL(uidOf(r), app)
	if err != nil {
		fail(w, err)
		return
	}
	if isBlank(parsed) {
		fail(w, blankErr("sharing"))
		return
	}
	tok, err := newShareToken()
	if err != nil {
		fail(w, err)
		return
	}
	s := store.Share{Token: tok, AppID: id, OwnerUID: uidOf(r), Name: app.Name, Version: app.CurrentVersion,
		DSL: app.DSL, CreatedAt: time.Now().UTC()}
	if err := h.store.CreateShare(r.Context(), s); err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toShareOut(r, &s))
}

// liveShare loads a share that has not been revoked (404 otherwise).
func (h *Handler) liveShare(r *http.Request) (*store.Share, error) {
	tok, err := pathID(r, "token")
	if err != nil {
		return nil, err
	}
	s, err := h.store.GetShare(r.Context(), tok)
	if err != nil {
		return nil, mapStoreErr(err, "share")
	}
	if s.Revoked {
		return nil, apiErr(http.StatusNotFound, httpx.CodeNotFound, "share has been revoked")
	}
	return s, nil
}

// getShare: GET /v1/shares/{token} — preview before importing.
func (h *Handler) getShare(w http.ResponseWriter, r *http.Request) {
	s, err := h.liveShare(r)
	if err != nil {
		fail(w, err)
		return
	}
	out := map[string]any{"token": s.Token, "name": s.Name, "dslVersion": s.Version, "mine": s.OwnerUID == uidOf(r)}
	if a, err := dsl.Parse(s.DSL); err == nil {
		out["description"] = a.App.Description
		out["components"] = componentTypes(a)
		out["sharedData"] = a.HasSharedEntity()
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// revokeShare: DELETE /v1/shares/{token} (owner only, idempotent).
func (h *Handler) revokeShare(w http.ResponseWriter, r *http.Request) {
	tok, err := pathID(r, "token")
	if err != nil {
		fail(w, err)
		return
	}
	s, err := h.store.GetShare(r.Context(), tok)
	if err != nil {
		fail(w, mapStoreErr(err, "share"))
		return
	}
	if s.OwnerUID != uidOf(r) {
		fail(w, mapShareErr(store.ErrForbidden, "share"))
		return
	}
	if err := h.store.RevokeShare(r.Context(), tok); err != nil {
		fail(w, mapStoreErr(err, "share"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// importShare: POST /v1/shares/{token}/import.
func (h *Handler) importShare(w http.ResponseWriter, r *http.Request) {
	s, err := h.liveShare(r)
	if err != nil {
		fail(w, err)
		return
	}
	res, err := h.installCopy(r, s.AppID, s.DSL, "シェアから追加")
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, res)
}

// installResult is AppWithDSL plus how the copy relates to the source.
type installResult struct {
	appWithDSL
	// Joined: the DSL has shared/public entities, so the copy keeps the source
	// appId and sees the same apps_public records (#93). Otherwise a new appId.
	Joined bool `json:"joined"`
	// Created is false when the caller already had that (joined) appId.
	Created bool `json:"created"`
}

// installCopy puts srcDSL into the caller's tree. Private-only apps get a new
// appId (an independent copy, records start empty). Apps with shared/public
// entities keep srcAppID: shared records are keyed by appId alone, so a new id
// would hand the receiver an empty board instead of the community they were
// invited to — the same "install" #93 already performs for GET /dsl.
//
// The copy is written as version 2 on top of the initial version 1 so iOS
// does not treat it as a blank, never-generated app (AppSummary.isBlank).
func (h *Handler) installCopy(r *http.Request, srcAppID string, srcDSL json.RawMessage, note string) (*installResult, error) {
	ctx, uid := r.Context(), uidOf(r)
	app, err := dsl.Parse(srcDSL)
	if err != nil {
		return nil, invalidDSL(err)
	}
	joined := app.HasSharedEntity()
	id := newAppID()
	if joined {
		id = srcAppID
		if cur, err := h.store.GetApp(ctx, uid, id); err == nil {
			if cur, err = h.reactivate(r, cur); err != nil {
				return nil, err
			}
			return &installResult{appWithDSL: appWithDSL{appSummary: summary(cur), DSL: cur.DSL}, Joined: true}, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	v, err := h.seedCopy(r, id, compact(srcDSL), app.App.Name, &store.Source{Kind: "import", Mode: "full", Summary: note})
	if err != nil {
		return nil, err
	}
	if !joined {
		h.claim(r, id) // a fresh appId: the installer is its creator
	}
	cur, err := h.store.GetApp(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	slog.Info("appconfig: installed copy", slog.String("from", srcAppID), slog.String("appId", id), slog.Bool("joined", joined), slog.Int("version", v.Version))
	return &installResult{appWithDSL: appWithDSL{appSummary: summary(cur), DSL: cur.DSL}, Joined: joined, Created: true}, nil
}

// reactivate un-archives an app the caller is getting again.
func (h *Handler) reactivate(r *http.Request, cur *store.App) (*store.App, error) {
	if cur.Status != "archived" {
		return cur, nil
	}
	active := "active"
	return h.store.UpdateAppMeta(r.Context(), uidOf(r), cur.ID, nil, &active)
}

// ---- public catalog ---------------------------------------------------------

type listingOut struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	AuthorName  string    `json:"authorName"`
	Components  []string  `json:"components"`
	DSLVersion  int       `json:"dslVersion"`
	Installs    int       `json:"installs"`
	Mine        bool      `json:"mine"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func toListingOut(r *http.Request, c *store.CatalogApp) listingOut {
	tags, comps := c.Tags, c.Components
	if tags == nil {
		tags = []string{}
	}
	if comps == nil {
		comps = []string{}
	}
	return listingOut{ID: c.ID, Name: c.Name, Description: c.Description, Tags: tags, AuthorName: c.AuthorName,
		Components: comps, DSLVersion: c.Version, Installs: c.Installs, Mine: c.OwnerUID == uidOf(r),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func tooLong(s string, n int) bool { return utf8.RuneCountInString(s) > n }

// publishApp: POST /v1/apps/{appId}/publish (create or update the listing).
// Creator only (owner.go, #153); a blank app is 400.
func (h *Handler) publishApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		AuthorName  string   `json:"authorName"`
	}
	if r.ContentLength != 0 && !httpx.DecodeBody(w, r, &body) {
		return
	}
	app, err := h.store.GetApp(r.Context(), uidOf(r), id)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	parsed, err := h.parsedDSL(uidOf(r), app)
	if err != nil {
		fail(w, err)
		return
	}
	if err := h.requireCreator(r, id, parsed); err != nil {
		fail(w, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = parsed.App.Name
	}
	desc := strings.TrimSpace(body.Description)
	if desc == "" {
		desc = parsed.App.Description
	}
	author := strings.TrimSpace(body.AuthorName)
	if author == "" {
		author = defaultAuthorName
	}
	var tags []string
	for _, t := range body.Tags {
		if t = strings.TrimPrefix(strings.TrimSpace(t), "#"); t != "" {
			tags = append(tags, t)
		}
	}
	switch {
	case tooLong(name, maxListingName):
		err = fmt.Errorf("name must be at most %d characters", maxListingName)
	case tooLong(desc, maxListingDesc):
		err = fmt.Errorf("description must be at most %d characters", maxListingDesc)
	case tooLong(author, maxAuthorNameLen):
		err = fmt.Errorf("authorName must be at most %d characters", maxAuthorNameLen)
	case len(tags) > maxTags:
		err = fmt.Errorf("at most %d tags", maxTags)
	}
	for _, t := range tags {
		if tooLong(t, maxTagLen) {
			err = fmt.Errorf("each tag must be at most %d characters", maxTagLen)
		}
	}
	if err != nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, err.Error()))
		return
	}
	c, err := h.store.PutCatalogApp(r.Context(), uidOf(r), store.CatalogApp{
		ID: id, Name: name, Description: desc, Tags: tags, AuthorName: author,
		Components: componentTypes(parsed), Version: app.CurrentVersion, DSL: app.DSL,
	})
	if err != nil {
		fail(w, mapShareErr(err, "listing"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toListingOut(r, c))
}

// unpublishApp: DELETE /v1/apps/{appId}/publish (owner only).
func (h *Handler) unpublishApp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	if err := h.store.DeleteCatalogApp(r.Context(), uidOf(r), id); err != nil {
		fail(w, mapShareErr(err, "listing"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func matches(c *store.CatalogApp, q string) bool {
	if q == "" {
		return true
	}
	hay := []string{c.Name, c.Description, c.AuthorName}
	hay = append(hay, c.Tags...)
	for _, s := range hay {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

// listPublicApps: GET /v1/public-apps?q=&sort=popular|new&pageSize=. The
// catalog is small, so search is an in-memory substring match over at most
// store.MaxCatalogApps listings (no full-text index).
func (h *Handler) listPublicApps(w http.ResponseWriter, r *http.Request) {
	p, err := parsePaging(r)
	if err != nil {
		fail(w, err)
		return
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy != "" && sortBy != "popular" && sortBy != "new" {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "sort must be popular or new"))
		return
	}
	all, err := h.store.ListCatalogApps(r.Context(), store.MaxCatalogApps)
	if err != nil {
		fail(w, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	hits := make([]store.CatalogApp, 0, len(all))
	for i := range all {
		if matches(&all[i], q) {
			hits = append(hits, all[i])
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := &hits[i], &hits[j]
		if sortBy != "new" && a.Installs != b.Installs {
			return a.Installs > b.Installs
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
	start := min(p.offset, len(hits))
	end := min(start+p.size, len(hits))
	items := make([]listingOut, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, toListingOut(r, &hits[i]))
	}
	resp := map[string]any{"items": items}
	if nt := p.next(len(hits) - start); nt != "" {
		resp["nextPageToken"] = nt
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// installPublicApp: POST /v1/public-apps/{id}/install. Installing again
// returns the copy made the first time (created:false) instead of another
// one, and installs counts each uid once (never the publisher) (#154).
func (h *Handler) installPublicApp(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	c, err := h.store.GetCatalogApp(ctx, id)
	if err != nil {
		fail(w, mapStoreErr(err, "public app"))
		return
	}
	if prev, err := h.store.GetInstall(ctx, id, uid); err == nil {
		if cur, err := h.store.GetApp(ctx, uid, prev); err == nil {
			if cur, err = h.reactivate(r, cur); err != nil {
				fail(w, err)
				return
			}
			httpx.WriteJSON(w, http.StatusCreated, installResult{appWithDSL: appWithDSL{appSummary: summary(cur), DSL: cur.DSL}, Joined: prev == c.ID})
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			fail(w, err)
			return
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		fail(w, err)
		return
	}
	res, err := h.installCopy(r, c.ID, c.DSL, "「探す」から追加")
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.store.RecordInstall(ctx, id, uid, res.ID, res.Created && c.OwnerUID != uid); err != nil {
		slog.Warn("appconfig: install not recorded", slog.String("id", id), slog.Any("error", err))
	}
	httpx.WriteJSON(w, http.StatusCreated, res)
}
