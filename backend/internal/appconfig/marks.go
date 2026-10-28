package appconfig

import (
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Marks (#255): like / save / read toggles on shared records, once per person.
//
//	PUT    /v1/apps/{appId}/entities/{entityId}/records/{recordId}/marks/{markId}  -> {count, mine:true}
//	DELETE /v1/apps/{appId}/entities/{entityId}/records/{recordId}/marks/{markId}  -> {count, mine:false}
//
// Both are idempotent. Anyone who can read the record may mark it (it is the
// caller's own mark, not an edit of the record), so entity.edit does not apply.
// Records of an entity with marks carry marks: {"<markId>": {count, mine}}.

func (h *Handler) putMark(w http.ResponseWriter, r *http.Request)    { h.setMark(w, r, true) }
func (h *Handler) deleteMark(w http.ResponseWriter, r *http.Request) { h.setMark(w, r, false) }

func (h *Handler) setMark(w http.ResponseWriter, r *http.Request, on bool) {
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
	mid, err := pathID(r, "markId")
	if err != nil {
		fail(w, err)
		return
	}
	if !ent.IsShared() || ent.MarkByID(mid) == nil {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "mark not found in current dsl").With("entity", eid).With("mark", mid))
		return
	}
	st, err := h.store.SetMark(r.Context(), recordSet(r, cur, ent, eid), rid, mid, uidOf(r), on)
	if err != nil {
		fail(w, mapStoreErr(err, "record"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

// withMarks fills out[i].Marks for an entity that declares marks: every mark
// gets an entry (count 0 when nobody set it), mine is the caller's own state.
func (h *Handler) withMarks(r *http.Request, rs store.RecordSet, ent *dsl.Entity, recs []store.Record, out []recordOut) error {
	if !ent.IsShared() || len(ent.Marks) == 0 || len(out) == 0 {
		return nil
	}
	ids := make([]string, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	mine, err := h.store.MarkedBy(r.Context(), rs, ids, ent.MarkIDs(), uidOf(r))
	if err != nil {
		return err
	}
	for i := range out {
		out[i].Marks = make(map[string]store.MarkState, len(ent.Marks))
		for _, m := range ent.Marks {
			out[i].Marks[m.ID] = store.MarkState{Count: recs[i].MarkCounts[m.ID], Mine: mine[out[i].ID][m.ID]}
		}
	}
	return nil
}
