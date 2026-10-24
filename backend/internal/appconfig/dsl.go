package appconfig

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// dslEnvelope mirrors DSLEnvelope.
type dslEnvelope struct {
	AppID     string          `json:"appId"`
	Version   int             `json:"version"`
	DSL       json.RawMessage `json:"dsl"`
	Source    *store.Source   `json:"source,omitempty"`
	CreatedAt *time.Time      `json:"createdAt,omitempty"`
}

// versionSummary mirrors VersionSummary.
type versionSummary struct {
	AppID         string        `json:"appId"`
	Version       int           `json:"version"`
	ParentVersion int           `json:"parentVersion,omitempty"`
	Source        *store.Source `json:"source,omitempty"`
	Patch         []dsl.PatchOp `json:"patch,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
}

func toSummary(v *store.Version) versionSummary {
	return versionSummary{AppID: v.AppID, Version: v.Version, ParentVersion: v.ParentVersion, Source: v.Source, Patch: v.Patch, CreatedAt: v.CreatedAt}
}

func (h *Handler) getDSL(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	app, err := h.ensureApp(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(app.CurrentVersion))
	w.Header().Set("Cache-Control", "no-cache")
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		for _, part := range strings.Split(inm, ",") {
			if v, ok := parseETag(part); ok && v == app.CurrentVersion {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, dslEnvelope{AppID: app.ID, Version: app.CurrentVersion, DSL: app.DSL})
}

func normalizeSource(src *store.Source, mode string) *store.Source {
	if src == nil {
		src = &store.Source{Kind: "manual"}
	}
	if src.Kind == "" {
		src.Kind = "manual"
	}
	if src.Mode == "" {
		src.Mode = mode
	}
	return src
}

func (h *Handler) putDSL(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	base, err := requireIfMatch(r)
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		DSL    json.RawMessage `json:"dsl"`
		Source *store.Source   `json:"source"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if len(body.DSL) == 0 {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "dsl is required"))
		return
	}
	app, err := dsl.Parse(body.DSL)
	if err != nil {
		fail(w, invalidDSL(err))
		return
	}
	if _, err := h.ensureApp(r, id); err != nil {
		fail(w, err)
		return
	}
	raw := compact(body.DSL)
	v, err := h.store.CommitVersion(r.Context(), uidOf(r), id, store.Commit{
		BaseVersion: base, DSL: raw, Name: app.App.Name, Source: normalizeSource(body.Source, "full"),
	})
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	h.publish(r, id, app, raw, v.Version)
	w.Header().Set("ETag", etag(v.Version))
	httpx.WriteJSON(w, http.StatusCreated, toSummary(v))
}

func (h *Handler) patchDSL(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	base, err := requireIfMatch(r)
	if err != nil {
		fail(w, err)
		return
	}
	var ops []dsl.PatchOp
	var src *store.Source
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json-patch+json") {
		if !httpx.DecodeBody(w, r, &ops) {
			return
		}
	} else {
		var body struct {
			Patch  []dsl.PatchOp `json:"patch"`
			Source *store.Source `json:"source"`
		}
		if !httpx.DecodeBody(w, r, &body) {
			return
		}
		ops, src = body.Patch, body.Source
	}
	cur, err := h.ensureApp(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	if cur.CurrentVersion != base {
		fail(w, mapStoreErr(&store.ConflictError{Current: cur.CurrentVersion}, "app"))
		return
	}
	next, app, err := dsl.ApplyPatch(cur.DSL, ops)
	if err != nil {
		fail(w, invalidDSL(err))
		return
	}
	v, err := h.store.CommitVersion(r.Context(), uidOf(r), id, store.Commit{
		BaseVersion: base, DSL: next, Name: app.App.Name, Patch: ops, Source: normalizeSource(src, "patch"),
	})
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	h.publish(r, id, app, next, v.Version)
	w.Header().Set("ETag", etag(v.Version))
	httpx.WriteJSON(w, http.StatusCreated, dslEnvelope{AppID: id, Version: v.Version, DSL: v.DSL, Source: v.Source, CreatedAt: &v.CreatedAt})
}

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	p, err := parsePaging(r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.ensureApp(r, id); err != nil {
		fail(w, err)
		return
	}
	vs, err := h.store.ListVersions(r.Context(), uidOf(r), id, p.size+1, p.offset)
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	items := make([]versionSummary, 0, len(vs))
	for i := range vs {
		if i == p.size {
			break
		}
		items = append(items, toSummary(&vs[i]))
	}
	resp := map[string]any{"items": items}
	if nt := p.next(len(vs)); nt != "" {
		resp["nextPageToken"] = nt
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func versionParam(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || n < 1 {
		return 0, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "version must be a positive integer")
	}
	return n, nil
}

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	n, err := versionParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	v, err := h.store.GetVersion(r.Context(), uidOf(r), id, n)
	if err != nil {
		fail(w, mapStoreErr(err, "version"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dslEnvelope{AppID: id, Version: v.Version, DSL: v.DSL, Source: v.Source, CreatedAt: &v.CreatedAt})
}

func (h *Handler) restoreVersion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	n, err := versionParam(r)
	if err != nil {
		fail(w, err)
		return
	}
	cur, err := h.ensureApp(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	target, err := h.store.GetVersion(r.Context(), uidOf(r), id, n)
	if err != nil {
		fail(w, mapStoreErr(err, "version"))
		return
	}
	app, err := dsl.Parse(target.DSL)
	if err != nil {
		fail(w, invalidDSL(err))
		return
	}
	v, err := h.store.CommitVersion(r.Context(), uidOf(r), id, store.Commit{
		BaseVersion: cur.CurrentVersion, DSL: target.DSL, Name: app.App.Name,
		Source: &store.Source{Kind: "restore", Mode: "full", RestoredFrom: n},
	})
	if err != nil {
		fail(w, mapStoreErr(err, "app"))
		return
	}
	h.publish(r, id, app, target.DSL, v.Version)
	w.Header().Set("ETag", etag(v.Version))
	httpx.WriteJSON(w, http.StatusCreated, toSummary(v))
}

func (h *Handler) listEntities(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	cur, err := h.ensureApp(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	app, err := h.parsedDSL(uidOf(r), cur)
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"version": cur.CurrentVersion, "items": app.Entities})
}

func (h *Handler) getEntity(w http.ResponseWriter, r *http.Request) {
	_, ent, _, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ent)
}

// entityOf resolves appId/entityId from the path against the current DSL.
func (h *Handler) entityOf(r *http.Request) (*store.App, *dsl.Entity, string, error) {
	id, err := pathID(r, "appId")
	if err != nil {
		return nil, nil, "", err
	}
	eid, err := pathID(r, "entityId")
	if err != nil {
		return nil, nil, "", err
	}
	cur, err := h.ensureApp(r, id)
	if err != nil {
		return nil, nil, "", err
	}
	app, err := h.parsedDSL(uidOf(r), cur)
	if err != nil {
		return nil, nil, "", err
	}
	ent := app.EntityByID(eid)
	if ent == nil {
		return nil, nil, "", apiErr(http.StatusNotFound, httpx.CodeNotFound, "entity "+strconv.Quote(eid)+" not found in current dsl")
	}
	return cur, ent, eid, nil
}
