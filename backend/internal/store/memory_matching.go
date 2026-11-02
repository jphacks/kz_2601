package store

import (
	"context"
	"sort"
	"strings"
)

// ChangeMatchPair implements Matching.
func (m *Memory) ChangeMatchPair(_ context.Context, appID, a, b string, fn func(*MatchPair) (*MatchPair, error)) (*MatchPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := MatchPairID(a, b)
	var cur *MatchPair
	if p := m.matchPairs[appID][id]; p != nil {
		cur = p.clone()
	}
	next, err := fn(cur)
	if err != nil || next == nil {
		return cur, err
	}
	next = next.clone()
	next.ID, next.UIDs = id, SortedPair(a, b)
	now := m.now().UTC()
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}
	next.UpdatedAt = now
	if m.matchPairs == nil {
		m.matchPairs = map[string]map[string]*MatchPair{}
	}
	if m.matchPairs[appID] == nil {
		m.matchPairs[appID] = map[string]*MatchPair{}
	}
	m.matchPairs[appID][id] = next
	return next.clone(), nil
}

// ListMatchPairs implements Matching.
func (m *Memory) ListMatchPairs(_ context.Context, appID, uid string) ([]MatchPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []MatchPair
	for _, p := range m.matchPairs[appID] {
		if p.UIDs[0] == uid || p.UIDs[1] == uid {
			out = append(out, *p.clone())
		}
	}
	sortMatchPairs(out)
	return page(out, MaxMatchPairs, 0), nil
}

func sortMatchPairs(ps []MatchPair) {
	sort.Slice(ps, func(i, j int) bool {
		if !ps[i].UpdatedAt.Equal(ps[j].UpdatedAt) {
			return ps[i].UpdatedAt.After(ps[j].UpdatedAt)
		}
		return ps[i].ID < ps[j].ID
	})
}

// TakeMatchQuota implements Matching.
func (m *Memory) TakeMatchQuota(_ context.Context, uid, day string, limit int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.matchQuota == nil {
		m.matchQuota = map[string]int{}
	}
	key := uid + "/" + day
	if limit > 0 && m.matchQuota[key] >= limit {
		return ErrRateLimited
	}
	m.matchQuota[key]++
	return nil
}

// BlockedEither implements Matching.
func (m *Memory) BlockedEither(_ context.Context, uid string, others []string) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, o := range others {
		if m.blocks[uid][o] != nil || m.blocks[o][uid] != nil {
			out[o] = true
		}
	}
	return out, nil
}

// purgeMatching erases uid's pairs in appID and uid's records in its
// profile / matched entities. Callers hold m.mu.
func (m *Memory) purgeMatching(appID, uid string, res *AccountPurgeResult) {
	for id, p := range m.matchPairs[appID] {
		if p.UIDs[0] == uid || p.UIDs[1] == uid {
			delete(m.matchPairs[appID], id)
			res.MatchPairs++
		}
	}
	p := m.public[appID]
	if p == nil || p.app == nil {
		return
	}
	for _, e := range matchEntityIDs(p.app.DSL) {
		for id, r := range p.records[e] {
			if r.Author == uid {
				delete(p.records[e], id)
				m.dropMarks("pub/" + appID + "/" + e + "/" + id + "/")
				res.MatchRecords++
			}
		}
	}
}

// dropMatchQuota: callers hold m.mu.
func (m *Memory) dropMatchQuota(uid string) {
	for k := range m.matchQuota {
		if strings.HasPrefix(k, uid+"/") {
			delete(m.matchQuota, k)
		}
	}
}
