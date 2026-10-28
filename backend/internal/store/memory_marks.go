package store

import (
	"context"
	"strings"
)

func markKey(rs RecordSet, recordID, markID, uid string) string {
	return recordsKey(rs) + "/" + recordID + "/" + markID + "/" + uid
}

// SetMark implements Marks.
func (m *Memory) SetMark(_ context.Context, rs RecordSet, recordID, markID, uid string, on bool) (MarkState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recs, err := m.recordsOf(rs)
	if err != nil {
		return MarkState{}, err
	}
	r, ok := recs[rs.Entity][recordID]
	if !ok {
		return MarkState{}, ErrNotFound
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}
	key := markKey(rs, recordID, markID, uid)
	if m.marks[key] != on {
		if r.MarkCounts == nil {
			r.MarkCounts = map[string]int{}
		}
		if on {
			m.marks[key] = true
			r.MarkCounts[markID]++
		} else {
			delete(m.marks, key)
			r.MarkCounts[markID] = max(0, r.MarkCounts[markID]-1)
		}
		m.hub.publish(recordsKey(rs), 0)
	}
	return MarkState{Count: r.MarkCounts[markID], Mine: on}, nil
}

// MarkedBy implements Marks.
func (m *Memory) MarkedBy(_ context.Context, rs RecordSet, recordIDs, markIDs []string, uid string) (map[string]map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]map[string]bool{}
	for _, rid := range recordIDs {
		for _, mid := range markIDs {
			if m.marks[markKey(rs, rid, mid, uid)] {
				if out[rid] == nil {
					out[rid] = map[string]bool{}
				}
				out[rid][mid] = true
			}
		}
	}
	return out, nil
}

// dropMarks forgets every mark under prefix (a record or a whole entity).
// Callers hold m.mu.
func (m *Memory) dropMarks(prefix string) {
	for k := range m.marks {
		if strings.HasPrefix(k, prefix) {
			delete(m.marks, k)
		}
	}
}
