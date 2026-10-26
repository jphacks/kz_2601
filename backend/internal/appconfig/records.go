package appconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Shared (scope: shared/public) records carry the poster.
const (
	defaultAuthorName = "匿名"
	maxAuthorNameLen  = 64
	authorField       = "author" // filterField for "my posts" on shared entities
)

type recordOut struct {
	ID         string         `json:"id"`
	Entity     string         `json:"entity"`
	Data       map[string]any `json:"data"`
	Author     string         `json:"author,omitempty"`
	AuthorName string         `json:"authorName,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

func toRecordOut(r *store.Record) recordOut {
	return recordOut{ID: r.ID, Entity: r.Entity, Data: r.Data, Author: r.Author, AuthorName: r.AuthorName, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func recordValue(r *store.Record, field string) any {
	switch field {
	case "createdAt":
		return r.CreatedAt.Format(time.RFC3339Nano)
	case "updatedAt":
		return r.UpdatedAt.Format(time.RFC3339Nano)
	}
	return r.Data[field]
}

// recordSet picks the collection for an entity: the caller's own tree for
// private entities, the app-wide apps_public tree for shared/public ones.
func recordSet(r *http.Request, cur *store.App, ent *dsl.Entity, eid string) store.RecordSet {
	if ent.IsShared() {
		return store.PublicRecords(cur.ID, eid)
	}
	return store.PrivateRecords(uidOf(r), cur.ID, eid)
}

// ownRecord loads a record and, on shared entities, requires the caller to be
// its author (403 otherwise). Private records are the caller's by construction.
// forUpdate lets anyone through on edit:members entities (#252); deleting
// stays the author's alone.
func (h *Handler) ownRecord(r *http.Request, rs store.RecordSet, ent *dsl.Entity, id string, forUpdate bool) (*store.Record, error) {
	rec, err := h.store.GetRecord(r.Context(), rs, id)
	if err != nil {
		return nil, mapStoreErr(err, "record")
	}
	if forUpdate && ent.MembersCanEdit() {
		return rec, nil
	}
	if ent.IsShared() && rec.Author != uidOf(r) {
		return nil, apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the author can modify this record").With("author", rec.Author)
	}
	return rec, nil
}

func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	p, err := parsePaging(r)
	if err != nil {
		fail(w, err)
		return
	}
	q := r.URL.Query()
	sortField, sortDir := q.Get("sortField"), q.Get("sortDir")
	if sortField == "" {
		sortField = "createdAt"
	}
	if sortDir == "" {
		sortDir = "desc"
	}
	if sortDir != "asc" && sortDir != "desc" {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "sortDir must be asc or desc"))
		return
	}
	if sortField != "createdAt" && sortField != "updatedAt" && ent.FieldByID(sortField) == nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown sortField").With("field", sortField))
		return
	}
	pred, err := buildFilter(ent, q.Get("filterField"), q.Get("filterOp"), q.Get("filterValue"))
	if err != nil {
		fail(w, err)
		return
	}

	all, err := h.store.ListRecords(r.Context(), recordSet(r, cur, ent, eid), store.MaxRecordsPerEntity+1)
	if err != nil {
		fail(w, mapStoreErr(err, "entity"))
		return
	}
	if len(all) > store.MaxRecordsPerEntity {
		fail(w, apiErr(http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge, "too many records to sort in memory"))
		return
	}
	items := make([]store.Record, 0, len(all))
	for i := range all {
		if pred == nil || pred(&all[i]) {
			items = append(items, all[i])
		}
	}
	if sortField != "createdAt" || sortDir != "desc" {
		sort.SliceStable(items, func(i, j int) bool {
			c, ok := compareValues(recordValue(&items[i], sortField), recordValue(&items[j], sortField))
			if !ok {
				return false
			}
			if sortDir == "desc" {
				return c > 0
			}
			return c < 0
		})
	}
	total := len(items)
	if p.offset < total {
		items = items[p.offset:]
	} else {
		items = nil
	}
	out := make([]recordOut, 0, min(len(items), p.size))
	for i := range items {
		if i == p.size {
			break
		}
		out = append(out, toRecordOut(&items[i]))
	}
	resp := map[string]any{"items": out}
	if nt := p.next(len(items)); nt != "" {
		resp["nextPageToken"] = nt
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// buildFilter compiles filterField/op/value into a predicate (nil = none).
// On shared entities filterField=author (eq/neq, value = uid) selects posts
// by one user, e.g. "my posts".
func buildFilter(ent *dsl.Entity, field, op, value string) (func(*store.Record) bool, error) {
	if field == "" && op == "" {
		return nil, nil
	}
	if field == authorField && ent.IsShared() {
		switch op {
		case "eq":
			return func(r *store.Record) bool { return r.Author == value }, nil
		case "neq":
			return func(r *store.Record) bool { return r.Author != value }, nil
		}
		return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "filterOp for author must be eq or neq")
	}
	f := ent.FieldByID(field)
	if f == nil {
		return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "unknown filterField").With("field", field)
	}
	switch op {
	case "today", "thisMonth":
		if f.Type != dsl.FieldDate && f.Type != dsl.FieldDatetime {
			return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, op+" requires a date/datetime field")
		}
		now := time.Now().In(jst)
		return func(r *store.Record) bool {
			d, ok := dateOf(r.Data[field])
			if !ok {
				return false
			}
			if op == "today" {
				return d.Year() == now.Year() && d.YearDay() == now.YearDay()
			}
			return d.Year() == now.Year() && d.Month() == now.Month()
		}, nil
	case "eq", "neq", "gt", "gte", "lt", "lte":
		want, err := parseFilterValue(f, value)
		if err != nil {
			return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		}
		return func(r *store.Record) bool {
			c, ok := compareValues(r.Data[field], want)
			if !ok {
				return op == "neq"
			}
			switch op {
			case "eq":
				return c == 0
			case "neq":
				return c != 0
			case "gt":
				return c > 0
			case "gte":
				return c >= 0
			case "lt":
				return c < 0
			}
			return c <= 0
		}, nil
	}
	return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "filterOp must be one of eq,neq,gt,gte,lt,lte,thisMonth,today")
}

func checkRecordSize(data map[string]any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if len(b) > maxRecordBody {
		return apiErr(http.StatusRequestEntityTooLarge, httpx.CodePayloadTooLarge, "record data exceeds 300KB")
	}
	return nil
}

func (h *Handler) createRecord(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Data       map[string]any `json:"data"`
		ClientID   string         `json:"clientId"`
		AuthorName string         `json:"authorName"` // shared entities only; "" -> 匿名
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if body.Data == nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "data is required"))
		return
	}
	data, err := normalizeData(ent, body.Data, true)
	if err != nil {
		fail(w, err)
		return
	}
	if err := checkRecordSize(data); err != nil {
		fail(w, err)
		return
	}
	if len(body.ClientID) > 256 {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "clientId too long"))
		return
	}
	rec := store.Record{ID: store.NewID("rec_"), Data: data, ClientID: body.ClientID}
	clientKey := body.ClientID
	if ent.IsShared() {
		rec.Author = uidOf(r)
		rec.AuthorName = strings.TrimSpace(body.AuthorName)
		if rec.AuthorName == "" {
			rec.AuthorName = defaultAuthorName
		}
		if utf8.RuneCountInString(rec.AuthorName) > maxAuthorNameLen {
			fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "authorName too long"))
			return
		}
		// the collection is shared, so two users' idempotency keys must not collide
		clientKey = rec.Author + "/" + body.ClientID
	}
	if body.ClientID != "" {
		rec.ID = store.IDFromClientID(clientKey)
	}
	out, _, err := h.store.CreateRecord(r.Context(), recordSet(r, cur, ent, eid), rec)
	if err != nil {
		fail(w, mapStoreErr(err, "entity"))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRecordOut(out))
}

func (h *Handler) getRecord(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	rid, err := pathID(r, "recordId")
	if err != nil {
		fail(w, err)
		return
	}
	rec, err := h.store.GetRecord(r.Context(), recordSet(r, cur, ent, eid), rid)
	if err != nil {
		fail(w, mapStoreErr(err, "record"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRecordOut(rec))
}

func (h *Handler) patchRecord(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	rid, err := pathID(r, "recordId")
	if err != nil {
		fail(w, err)
		return
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if len(body.Data) == 0 {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "data must have at least one key"))
		return
	}
	delta, err := normalizeData(ent, body.Data, false)
	if err != nil {
		fail(w, err)
		return
	}
	rs := recordSet(r, cur, ent, eid)
	existing, err := h.ownRecord(r, rs, ent, rid, true)
	if err != nil {
		fail(w, err)
		return
	}
	merged := make(map[string]any, len(existing.Data)+len(delta))
	for k, v := range existing.Data {
		merged[k] = v
	}
	for k, v := range delta {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	for i := range ent.Fields {
		if f := &ent.Fields[i]; f.Required && merged[f.ID] == nil {
			fail(w, invalidRecord(f.ID, "is required"))
			return
		}
	}
	if err := checkRecordSize(merged); err != nil {
		fail(w, err)
		return
	}
	rec, err := h.store.UpdateRecord(r.Context(), rs, rid, merged)
	if err != nil {
		fail(w, mapStoreErr(err, "record"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRecordOut(rec))
}

func (h *Handler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	rid, err := pathID(r, "recordId")
	if err != nil {
		fail(w, err)
		return
	}
	rs := recordSet(r, cur, ent, eid)
	if ent.IsShared() {
		if _, err := h.ownRecord(r, rs, ent, rid, false); err != nil {
			fail(w, err)
			return
		}
	}
	if err := h.store.DeleteRecord(r.Context(), rs, rid); err != nil {
		fail(w, mapStoreErr(err, "record"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAllRecords clears the entity. On shared entities only the caller's
// own posts are removed (nobody can wipe everyone's data).
func (h *Handler) deleteAllRecords(w http.ResponseWriter, r *http.Request) {
	cur, ent, eid, err := h.entityOf(r)
	if err != nil {
		fail(w, err)
		return
	}
	if !strings.EqualFold(r.URL.Query().Get("confirm"), "true") {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "confirm=true is required"))
		return
	}
	rs := recordSet(r, cur, ent, eid)
	if !ent.IsShared() {
		if err := h.store.DeleteAllRecords(r.Context(), rs); err != nil {
			fail(w, mapStoreErr(err, "entity"))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	all, err := h.store.ListRecords(r.Context(), rs, 0)
	if err != nil {
		fail(w, mapStoreErr(err, "entity"))
		return
	}
	uid := uidOf(r)
	for i := range all {
		if all[i].Author != uid {
			continue
		}
		if err := h.store.DeleteRecord(r.Context(), rs, all[i].ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			fail(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
