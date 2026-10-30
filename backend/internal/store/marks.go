package store

import "context"

// MarkState is one mark of one record as seen by the caller (#255).
type MarkState struct {
	Count int  `json:"count"`
	Mine  bool `json:"mine"`
}

// Marks stores per-user toggles on shared records (like / save / read): one
// entry per (record, mark, uid), so a person counts once. The count is kept
// on the record (Record.MarkCounts) so listing stays one query; whether the
// caller marked it is looked up per page.
//
//	apps_public/{appId}/entities/{e}/records/{id}                    {..., markCounts: {like: 2}}
//	apps_public/{appId}/entities/{e}/records/{id}/marks/{markId}/by/{uid} {at}
type Marks interface {
	// SetMark adds (on) or removes uid's mark and returns the new state. It
	// is idempotent; ErrNotFound when the record does not exist.
	SetMark(ctx context.Context, rs RecordSet, recordID, markID, uid string, on bool) (MarkState, error)
	// MarkedBy reports, per record id, which of markIDs uid has set.
	MarkedBy(ctx context.Context, rs RecordSet, recordIDs, markIDs []string, uid string) (map[string]map[string]bool, error)
}
