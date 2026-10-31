package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// DeactivateUser implements Accounts.
func (m *Memory) DeactivateUser(_ context.Context, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	if m.deleted == nil {
		m.deleted = map[string]time.Time{}
	}
	if _, ok := m.deleted[uid]; !ok && !m.purged[uid] {
		m.deleted[uid] = now // the first deletedAt is kept
	}
	for _, a := range m.users[uid] {
		if a.app.Status != "archived" {
			a.app.Status, a.app.UpdatedAt = "archived", now
		}
	}
	for _, s := range m.shares {
		if s.OwnerUID == uid {
			s.Revoked = true
		}
	}
	for id, c := range m.catalog {
		if c.OwnerUID == uid {
			delete(m.catalog, id)
		}
	}
	return nil
}

// UserDeleted implements Accounts.
func (m *Memory) UserDeleted(_ context.Context, uid string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.deleted[uid]
	return ok || m.purged[uid], nil
}

// DeletedAccounts implements AccountPurger.
func (m *Memory) DeletedAccounts(_ context.Context, cutoff time.Time, limit int) ([]DeletedAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []DeletedAccount
	for uid, at := range m.deleted {
		if at.Before(cutoff) {
			out = append(out, DeletedAccount{UID: uid, DeletedAt: at})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeletedAt.Before(out[j].DeletedAt) })
	return page(out, limit, 0), nil
}

// PurgeAccount implements AccountPurger (same rules as the Firestore one).
func (m *Memory) PurgeAccount(_ context.Context, uid string) (AccountPurgeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res AccountPurgeResult
	if _, ok := m.deleted[uid]; !ok && !m.purged[uid] {
		return res, ErrNotDeleted
	}
	for appID := range m.users[uid] {
		c, listed := m.catalog[appID]
		if listed && c.OwnerUID == uid {
			delete(m.catalog, appID)
			res.Listings++
		}
		if (listed && c.OwnerUID == uid) || (!listed && m.owners[appID] == uid) {
			m.dropInstalls(func(id, _ string) bool { return id == appID })
		}
		p := m.public[appID]
		owned := p != nil && p.app != nil && p.app.OwnerUID == uid
		if owned && !m.othersIn(appID, uid) {
			delete(m.public, appID)
			delete(m.members, appID)
			m.dropMarks("pub/" + appID + "/")
		} else if p != nil {
			m.scrubPublic(appID, uid, &res)
			if owned {
				res.KeptMirrors++
			}
		}
		delete(m.users[uid], appID)
		res.Apps++
	}
	for tok, s := range m.shares {
		if s.OwnerUID == uid {
			delete(m.shares, tok)
			res.Shares++
		}
	}
	for id, c := range m.catalog {
		if c.OwnerUID == uid {
			delete(m.catalog, id)
			m.dropInstalls(func(l, _ string) bool { return l == id })
			res.Listings++
		}
	}
	for id, r := range m.releases {
		if r.OwnerUID == uid {
			delete(m.releases, id)
			res.Releases++
		}
	}
	res.Installs += m.dropInstalls(func(_, u string) bool { return u == uid })
	for id, r := range m.reports {
		if r.ReporterUID == uid || r.TargetUID == uid {
			delete(m.reports, id)
			res.Reports++
		}
	}
	for k := range m.quota {
		if strings.HasPrefix(k, uid+"/") {
			delete(m.quota, k)
		}
	}
	res.Blocks += len(m.blocks[uid])
	delete(m.blocks, uid)
	for appID, o := range m.owners {
		if p := m.public[appID]; o == uid && (p == nil || p.app == nil || p.app.OwnerUID != uid) { // a kept mirror keeps its owner
			delete(m.owners, appID)
			res.Owners++
		}
	}
	return res, nil
}

// MarkAccountPurged implements AccountPurger.
func (m *Memory) MarkAccountPurged(_ context.Context, uid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.purged[uid] {
		return nil
	}
	if _, ok := m.deleted[uid]; !ok {
		return ErrNotDeleted
	}
	delete(m.deleted, uid)
	if m.purged == nil {
		m.purged = map[string]bool{}
	}
	m.purged[uid] = true
	return nil
}

// othersIn: callers hold m.mu.
func (m *Memory) othersIn(appID, uid string) bool {
	for id := range m.members[appID] {
		if id != uid {
			return true
		}
	}
	for _, recs := range m.public[appID].records {
		for _, r := range recs {
			if r.Author != uid {
				return true
			}
		}
	}
	return false
}

// scrubPublic: callers hold m.mu.
func (m *Memory) scrubPublic(appID, uid string, res *AccountPurgeResult) {
	if _, ok := m.members[appID][uid]; ok {
		delete(m.members[appID], uid)
		res.Memberships++
	}
	p := m.public[appID]
	for _, recs := range p.records {
		for _, r := range recs {
			if r.Author == uid && r.AuthorName != "" {
				r.AuthorName = ""
				res.PostsScrubbed++
			}
		}
	}
	// keys: pub/{appId}/{entity}/{recordId}/{markId}/{uid}
	prefix := "pub/" + appID + "/"
	for k := range m.marks {
		if !strings.HasPrefix(k, prefix) || !strings.HasSuffix(k, "/"+uid) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(k, prefix), "/")
		if len(parts) != 4 {
			continue
		}
		delete(m.marks, k)
		res.Marks++
		if r := p.records[parts[0]][parts[1]]; r != nil && r.MarkCounts != nil {
			r.MarkCounts[parts[2]] = max(0, r.MarkCounts[parts[2]]-1)
		}
	}
}

// dropInstalls deletes installs whose (listing id, uid) match. Callers hold m.mu.
func (m *Memory) dropInstalls(match func(id, uid string) bool) int {
	n := 0
	for k := range m.installs {
		id, u, _ := strings.Cut(k, "\x00")
		if match(id, u) {
			delete(m.installs, k)
			n++
		}
	}
	return n
}
