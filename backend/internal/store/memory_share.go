package store

import (
	"context"
	"sort"
)

func cloneShare(s *Share) *Share {
	cp := *s
	cp.DSL = cloneRaw(s.DSL)
	return &cp
}

func cloneCatalog(c *CatalogApp, withDSL bool) *CatalogApp {
	cp := *c
	cp.Tags = append([]string(nil), c.Tags...)
	cp.Components = append([]string(nil), c.Components...)
	cp.DSL = nil
	if withDSL {
		cp.DSL = cloneRaw(c.DSL)
	}
	return &cp
}

// CreateShare implements Store.
func (m *Memory) CreateShare(_ context.Context, s Share) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shares == nil {
		m.shares = map[string]*Share{}
	}
	if _, ok := m.shares[s.Token]; ok {
		return ErrAlreadyExists
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = m.now().UTC()
	}
	m.shares[s.Token] = cloneShare(&s)
	return nil
}

// GetShare implements Store.
func (m *Memory) GetShare(_ context.Context, token string) (*Share, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.shares[token]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneShare(s), nil
}

// RevokeShare implements Store.
func (m *Memory) RevokeShare(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.shares[token]
	if !ok {
		return ErrNotFound
	}
	s.Revoked = true
	return nil
}

// PutCatalogApp implements Store.
func (m *Memory) PutCatalogApp(_ context.Context, uid string, c CatalogApp) (*CatalogApp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.catalog == nil {
		m.catalog = map[string]*CatalogApp{}
	}
	now := m.now().UTC()
	c.OwnerUID, c.UpdatedAt, c.CreatedAt, c.Installs = uid, now, now, 0
	if old, ok := m.catalog[c.ID]; ok {
		if old.OwnerUID != uid {
			return nil, ErrForbidden
		}
		c.CreatedAt, c.Installs = old.CreatedAt, old.Installs
	}
	m.catalog[c.ID] = cloneCatalog(&c, true)
	return cloneCatalog(&c, false), nil
}

// GetCatalogApp implements Store.
func (m *Memory) GetCatalogApp(_ context.Context, id string) (*CatalogApp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.catalog[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneCatalog(c, true), nil
}

// DeleteCatalogApp implements Store.
func (m *Memory) DeleteCatalogApp(_ context.Context, uid, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.catalog[id]
	if !ok {
		return ErrNotFound
	}
	if c.OwnerUID != uid {
		return ErrForbidden
	}
	delete(m.catalog, id)
	return nil
}

// ListCatalogApps implements Store.
func (m *Memory) ListCatalogApps(_ context.Context, limit int) ([]CatalogApp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CatalogApp, 0, len(m.catalog))
	for _, c := range m.catalog {
		out = append(out, *cloneCatalog(c, false))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Installs != out[j].Installs {
			return out[i].Installs > out[j].Installs
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return page(out, limit, 0), nil
}

// IncrementInstalls implements Store.
func (m *Memory) IncrementInstalls(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.catalog[id]
	if !ok {
		return ErrNotFound
	}
	c.Installs++
	return nil
}
