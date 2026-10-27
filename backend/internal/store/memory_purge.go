package store

import (
	"context"
	"strings"
)

// PurgeApp implements Store.
func (m *Memory) PurgeApp(_ context.Context, uid, appID string) (PurgeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res PurgeResult
	a := m.get(uid, appID)
	if a == nil {
		return res, ErrNotFound
	}
	for tok, s := range m.shares {
		if s.OwnerUID == uid && s.AppID == appID {
			delete(m.shares, tok)
			res.Shares++
		}
	}
	if c, ok := m.catalog[appID]; ok && c.OwnerUID == uid {
		delete(m.catalog, appID)
		for k := range m.installs {
			if strings.HasPrefix(k, appID+"\x00") {
				delete(m.installs, k)
			}
		}
		res.Listing = true
	}
	if p, ok := m.public[appID]; ok && p.app != nil && p.app.OwnerUID == uid {
		delete(m.public, appID)
		delete(m.members, appID) // Firestore deletes the mirror's members subcollection with it
		res.Mirror = true
	}
	res.Versions = len(a.versions)
	for _, recs := range a.records {
		res.Records += len(recs)
	}
	delete(m.users[uid], appID)
	return res, nil
}
