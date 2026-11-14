package appconfig

// Roles and record rules on shared data (#272 #273, docs/dsl/README.md §3.13).
//
//   - Admins: the app's creator always, plus members with Member.Admin. An
//     admin makes another member an admin (PATCH /apps/{appId}/members/{uid}).
//   - field.editableBy "admins": only admins set or change the field. An
//     admin may change those fields on anyone's record (and nothing else
//     unless entity.edit allows it).
//   - entity.deleteBy: author (default) / admins / authorOrAdmins.
//   - type member: the uid of a participant of the shared data.
//   - entity.unique: 409 duplicate_record when another record has the same
//     values (store.UniqueRecords keeps it in a transaction).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("PATCH /v1/apps/{appId}/members/{uid}", h.patchMember)
	})
}

// Reasons in details.reason, so iOS can say what happened.
const (
	adminsOnlyReason = "admins_only" // 403: only the app's admins may do this
	duplicateReason  = "duplicate"   // 409: same values as another record
)

func adminsOnly(field string) *apiError {
	e := apiErr(http.StatusForbidden, httpx.CodeForbidden, "only the admins of this app can do this").With("reason", adminsOnlyReason)
	if field != "" {
		e = e.With("field", field)
	}
	return e
}

// isAdmin: the creator, or a member made admin. Not a participant = false.
func (h *Handler) isAdmin(ctx context.Context, appID, uid string) (bool, error) {
	o, err := h.appOwner(ctx, appID)
	if err != nil {
		return false, err
	}
	if o != "" && o == uid {
		return true, nil
	}
	m, err := h.store.GetMember(ctx, appID, uid)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.Admin, nil
}

// patchMember: PATCH /v1/apps/{appId}/members/{uid} {"admin": bool}. Only an
// admin may call it; the creator stays an admin (400). An admin may also
// step down ("me").
func (h *Handler) patchMember(w http.ResponseWriter, r *http.Request) {
	ctx, uid := r.Context(), uidOf(r)
	id, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	target := r.PathValue("uid")
	if target == "me" {
		target = uid
	} else if !idRe.MatchString(target) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid uid"))
		return
	}
	var body struct {
		Admin *bool `json:"admin"`
	}
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if body.Admin == nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "admin is required"))
		return
	}
	admin, err := h.isAdmin(ctx, id, uid)
	if err != nil {
		fail(w, err)
		return
	}
	if !admin {
		role, err := h.memberRole(ctx, id, uid)
		if err != nil {
			fail(w, err)
			return
		}
		if role == "" {
			fail(w, notMember())
			return
		}
		fail(w, adminsOnly(""))
		return
	}
	owner, err := h.appOwner(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if owner != "" && target == owner {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "the creator of the app is always an admin"))
		return
	}
	if err := h.store.SetMemberAdmin(ctx, id, target, *body.Admin); err != nil {
		fail(w, mapStoreErr(err, "member"))
		return
	}
	slog.Info("appconfig: member admin changed", slog.String("appId", id), slog.Bool("admin", *body.Admin), slog.Bool("self", target == uid))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"uid": target, "admin": *body.Admin})
}

// emptyOrDefault: a value a non-admin may send for an admin-only field on
// create (iOS forms send the field's default or zero value).
func emptyOrDefault(f *dsl.Field, v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		if x == "" {
			return true
		}
	case bool:
		if !x {
			return true
		}
	}
	return sameValue(v, defaultFor(f))
}

// checkRecordRules checks a create (prev nil, data = the new record) or a
// PATCH (prev = stored record, data = the delta) on a shared/public entity:
// member values must be participants, admin-only fields change only by admins.
func (h *Handler) checkRecordRules(r *http.Request, cur *store.App, ent *dsl.Entity, prev *store.Record, data map[string]any) error {
	if !ent.IsShared() {
		return nil
	}
	ctx, uid := r.Context(), uidOf(r)
	var changed string
	for i := range ent.Fields {
		f := &ent.Fields[i]
		v, ok := data[f.ID]
		if !ok {
			continue
		}
		if f.Type == dsl.FieldMember && v != nil && (prev == nil || !sameValue(prev.Data[f.ID], v)) {
			s, _ := v.(string)
			role, err := h.memberRole(ctx, cur.ID, s)
			if err != nil {
				return err
			}
			if role == "" {
				return invalidRecord(f.ID, "is not a participant of this app")
			}
		}
		if !f.AdminOnly() || changed != "" {
			continue
		}
		if (prev == nil && !emptyOrDefault(f, v)) || (prev != nil && !sameValue(prev.Data[f.ID], v)) {
			changed = f.ID
		}
	}
	if changed == "" {
		return nil
	}
	admin, err := h.isAdmin(ctx, cur.ID, uid)
	if err != nil || admin {
		return err
	}
	return adminsOnly(changed)
}

// recordForPatch is ownRecord for PATCH plus the admin override: an admin
// may change the admin-only fields of a record they cannot otherwise edit.
func (h *Handler) recordForPatch(r *http.Request, rs store.RecordSet, cur *store.App, ent *dsl.Entity, id string, delta map[string]any) (*store.Record, error) {
	rec, err := h.ownRecord(r, rs, ent, id, true)
	var ae *apiError
	if err == nil || !ent.HasAdminOnlyFields() || !errors.As(err, &ae) || ae.status != http.StatusForbidden {
		return rec, err
	}
	stored, gerr := h.store.GetRecord(r.Context(), rs, id)
	if gerr != nil {
		return nil, mapStoreErr(gerr, "record")
	}
	changed := ""
	for k, v := range delta {
		if sameValue(stored.Data[k], v) {
			continue
		}
		if f := ent.FieldByID(k); f == nil || !f.AdminOnly() {
			return nil, err
		}
		changed = k
	}
	admin, aerr := h.isAdmin(r.Context(), cur.ID, uidOf(r))
	switch {
	case aerr != nil:
		return nil, aerr
	case !admin && changed != "":
		return nil, adminsOnly(changed) // say why rather than "only the author"
	case !admin:
		return nil, err
	}
	return stored, nil
}

// checkDelete enforces entity.deleteBy on a shared/public record.
func (h *Handler) checkDelete(r *http.Request, rs store.RecordSet, cur *store.App, ent *dsl.Entity, id string) error {
	by := ent.EffectiveDeleteBy()
	if by == dsl.DeleteByAuthor {
		_, err := h.ownRecord(r, rs, ent, id, false)
		return err
	}
	rec, err := h.store.GetRecord(r.Context(), rs, id)
	if err != nil {
		return mapStoreErr(err, "record")
	}
	if !canRead(ent, rec, uidOf(r)) { // #271
		return recordNotFound()
	}
	if by == dsl.DeleteByAuthorOrAdmins && rec.Author == uidOf(r) {
		return nil
	}
	admin, err := h.isAdmin(r.Context(), cur.ID, uidOf(r))
	if err != nil || admin {
		return err
	}
	return adminsOnly("")
}

// deleteAllScope says what DELETE .../records clears on a shared entity: all
// records (an admin where admins may delete) or the caller's own; an error
// when the caller may delete none (deleteBy admins, not an admin).
func (h *Handler) deleteAllScope(r *http.Request, cur *store.App, ent *dsl.Entity) (all bool, err error) {
	by := ent.EffectiveDeleteBy()
	if by == dsl.DeleteByAuthor {
		return false, nil
	}
	admin, err := h.isAdmin(r.Context(), cur.ID, uidOf(r))
	switch {
	case err != nil:
		return false, err
	case admin:
		return true, nil
	case by == dsl.DeleteByAdmins:
		return false, adminsOnly("")
	}
	return false, nil
}

// uniqueClaim hashes the record's values of entity.unique. A missing value
// claims nothing (empty fields are not compared).
func uniqueClaim(ent *dsl.Entity, data map[string]any, author string) store.UniqueClaim {
	if len(ent.Unique) == 0 {
		return store.UniqueClaim{}
	}
	match := make(map[string]any, len(ent.Unique))
	vals := make([]any, 0, len(ent.Unique))
	for _, id := range ent.Unique {
		var v any
		if id == dsl.RecordAuthor {
			if author != "" {
				v = author
			}
			id = store.UniqueAuthorKey
		} else {
			v = data[id]
		}
		if v == nil || v == "" {
			return store.UniqueClaim{}
		}
		match[id] = v
		if n, ok := asFloat(v); ok {
			v = n // 1 and 1.0 hash the same
		}
		vals = append(vals, v)
	}
	b, err := json.Marshal(append([]any{ent.ID, ent.Unique}, vals...))
	if err != nil {
		return store.UniqueClaim{}
	}
	sum := sha256.Sum256(b)
	return store.UniqueClaim{Key: hex.EncodeToString(sum[:20]), Match: match}
}

func duplicateRecord(ent *dsl.Entity) *apiError {
	return apiErr(http.StatusConflict, httpx.CodeDuplicateRecord, "another record already has the same values").
		With("reason", duplicateReason).With("fields", ent.Unique)
}

// createRecordKept is createRecordOutbox keeping entity.unique (#273).
func (h *Handler) createRecordKept(r *http.Request, cur *store.App, ent *dsl.Entity, rs store.RecordSet, rec store.Record) (*store.Record, bool, error) {
	if len(ent.Unique) == 0 {
		return h.createRecordOutbox(r, cur, ent, rs, rec)
	}
	emit, err := h.emitFor(r, cur, ent, dsl.TriggerCreated)
	if err != nil {
		return nil, false, err
	}
	out, created, err := h.store.CreateRecordUnique(r.Context(), rs, rec, uniqueClaim(ent, rec.Data, rec.Author), emit)
	if errors.Is(err, store.ErrDuplicate) {
		return nil, false, duplicateRecord(ent)
	}
	return out, created, err
}

// updateRecordKept is updateRecordOutbox keeping entity.unique (#273). A
// record whose unique values do not change keeps its claim (records written
// before unique are not compared again on every edit).
func (h *Handler) updateRecordKept(r *http.Request, cur *store.App, ent *dsl.Entity, rs store.RecordSet, prev *store.Record, data map[string]any) (*store.Record, error) {
	if len(ent.Unique) == 0 {
		return h.updateRecordOutbox(r, cur, ent, rs, prev.ID, data)
	}
	claim := uniqueClaim(ent, data, prev.Author)
	if old := uniqueClaim(ent, prev.Data, prev.Author); old.Key == claim.Key {
		claim = store.UniqueClaim{Key: prev.UniqueKey}
	}
	emit, err := h.emitFor(r, cur, ent, dsl.TriggerUpdated)
	if err != nil {
		return nil, err
	}
	out, err := h.store.UpdateRecordUnique(r.Context(), rs, prev.ID, data, claim, emit)
	if errors.Is(err, store.ErrDuplicate) {
		return nil, duplicateRecord(ent)
	}
	return out, err
}

func (h *Handler) emitFor(r *http.Request, cur *store.App, ent *dsl.Entity, kind string) (store.EmitFunc, error) {
	p, err := h.outboxPlanFor(r, cur, ent, kind)
	if err != nil || p == nil {
		return nil, err
	}
	return p.emit(), nil
}
