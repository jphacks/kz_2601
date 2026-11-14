package appconfig

// Addressed records (#271, docs/dsl/README.md §3.14): a record of an entity
// with addressedTo is read only by its author and its recipients (a member /
// members field). The readers are stored on the record when it is created
// (store.Record.Readers) and the recipients cannot change afterwards, so a
// later DSL change does not open old messages to others. Every read path
// hides the record from anyone else as if it did not exist (404): list, get,
// PATCH / DELETE, marks, reports and the events stream.

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// normalizeMembers: a type members value is a list of participant uids
// (duplicates and blanks dropped). Participation is checked in checkAddressed.
func normalizeMembers(v any) (any, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("must be an array of participant uids")
	}
	if len(arr) > dsl.MaxMemberValues {
		return nil, fmt.Errorf("has more than %d participants", dsl.MaxMemberValues)
	}
	out := make([]any, 0, len(arr))
	for _, x := range arr {
		s, ok := asString(x)
		if ok && s == "" {
			continue
		}
		if !ok || !idRe.MatchString(s) {
			return nil, fmt.Errorf("must be an array of participant uids")
		}
		if !slices.Contains(out, any(s)) {
			out = append(out, s)
		}
	}
	return out, nil
}

// recordReaders is what a new record of ent stores as its readers: the
// author and the recipients (nil when ent is not addressed).
func recordReaders(ent *dsl.Entity, author string, data map[string]any) []string {
	if !ent.IsAddressed() {
		return nil
	}
	out := []string{author}
	for _, u := range ent.Recipients(data) {
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// canRead reports whether uid may see rec. Stored readers decide whatever
// the DSL says now; a record without them (written before the entity was
// addressed) is read by its author and the people in its recipient field.
func canRead(ent *dsl.Entity, rec *store.Record, uid string) bool {
	if len(rec.Readers) > 0 {
		return slices.Contains(rec.Readers, uid)
	}
	if ent == nil || !ent.IsAddressed() {
		return true
	}
	return rec.Author == uid || slices.Contains(ent.Recipients(rec.Data), uid)
}

// recordNotFound is what a caller who may not read a record gets.
func recordNotFound() *apiError {
	return apiErr(http.StatusNotFound, httpx.CodeNotFound, "record not found")
}

// participants tells whether a uid is the creator or a member of the shared
// data. The owner is read once; each other uid is one members/{uid} read,
// remembered for the request (#2297: not the whole member list).
type participants struct {
	ctx   context.Context
	store store.MemberGetter
	appID string
	owner string
	known map[string]bool
}

func (h *Handler) participantsOf(ctx context.Context, appID string) (*participants, error) {
	owner, err := h.appOwner(ctx, appID)
	if err != nil {
		return nil, err
	}
	return &participants{ctx: ctx, store: h.store, appID: appID, owner: owner, known: map[string]bool{}}, nil
}

func (p *participants) has(uid string) (bool, error) {
	if uid == "" {
		return false, nil
	}
	if uid == p.owner {
		return true, nil
	}
	if ok, seen := p.known[uid]; seen {
		return ok, nil
	}
	ok, err := store.IsMember(p.ctx, p.store, p.appID, uid)
	if err != nil {
		return false, err
	}
	p.known[uid] = ok
	return ok, nil
}

// checkAddressed checks a create (prev nil) or a PATCH delta: members values
// are participants, an addressed record has recipients and keeps them.
func (h *Handler) checkAddressed(r *http.Request, cur *store.App, ent *dsl.Entity, prev *store.Record, data map[string]any) error {
	if !ent.IsShared() {
		return nil
	}
	var ps *participants
	for i := range ent.Fields {
		f := &ent.Fields[i]
		v, ok := data[f.ID]
		if f.Type != dsl.FieldMembers || !ok || v == nil || (prev != nil && sameValue(prev.Data[f.ID], v)) {
			continue
		}
		if ps == nil {
			var err error
			if ps, err = h.participantsOf(r.Context(), cur.ID); err != nil {
				return err
			}
		}
		arr, _ := v.([]any)
		for _, x := range arr {
			s, _ := x.(string)
			ok, err := ps.has(s)
			if err != nil {
				return err
			}
			if !ok {
				return invalidRecord(f.ID, "lists someone who is not a participant of this app")
			}
		}
	}
	if !ent.IsAddressed() {
		return nil
	}
	to := ent.AddressedTo
	if prev == nil {
		if len(ent.Recipients(data)) == 0 {
			return invalidRecord(to, "needs at least one recipient")
		}
		return nil
	}
	if _, ok := data[to]; ok && !sameMembers(ent.Recipients(prev.Data), ent.Recipients(data)) {
		return invalidRecord(to, "cannot change after the record was sent")
	}
	return nil
}

func sameMembers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

// checkReadable loads a shared record and hides it (404) from a caller who
// may not read it (marks, which otherwise do not read the record).
func (h *Handler) checkReadable(r *http.Request, rs store.RecordSet, ent *dsl.Entity, id string) error {
	if !ent.IsShared() {
		return nil
	}
	rec, err := h.store.GetRecord(r.Context(), rs, id)
	if err != nil {
		return mapStoreErr(err, "record")
	}
	if !canRead(ent, rec, uidOf(r)) {
		return recordNotFound()
	}
	ok, err := h.sharedFilter(r, rs.AppID, ent.ID, []store.Record{*rec})
	if err != nil {
		return err
	}
	if !ok(rec) { // #321 / #507
		return recordNotFound()
	}
	return nil
}

// watchEntity is the events stream's change signal for one entity: on an
// addressed entity only changes the caller may read.
func (h *Handler) watchEntity(ctx context.Context, w store.Watcher, r *http.Request, cur *store.App, ent *dsl.Entity, uid string) (<-chan struct{}, error) {
	rs := recordSet(r, cur, ent, ent.ID)
	if vw, ok := w.(store.VisibleWatcher); ok && ent.IsAddressed() {
		return vw.WatchVisibleRecords(ctx, rs, func(rec *store.Record) bool { return canRead(ent, rec, uid) })
	}
	if ent.IsAddressed() {
		return nil, apiErr(http.StatusNotImplemented, httpx.CodeInternal, "realtime events are not supported by this store")
	}
	return w.WatchRecords(ctx, rs)
}
