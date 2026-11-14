package store

import (
	"context"
	"sort"
)

// deviceRef is who holds a token (device_tokens/{tokenKey}).
type deviceRef struct {
	UID            string
	InstallationID string
}

// PutDevice implements Devices.
func (m *Memory) PutDevice(_ context.Context, uid string, d Device) (*Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.devices == nil {
		m.devices = map[string]map[string]*Device{}
		m.deviceTokens = map[string]deviceRef{}
	}
	if d.Platform == "" {
		d.Platform = PlatformIOS
	}
	now := m.now().UTC()
	key := tokenKey(d.Token)
	if ref, ok := m.deviceTokens[key]; ok && ref != (deviceRef{uid, d.InstallationID}) {
		if old := m.devices[ref.UID][ref.InstallationID]; old != nil && old.Token == d.Token {
			delete(m.devices[ref.UID], ref.InstallationID)
		}
	}
	mine := m.devices[uid]
	if mine == nil {
		mine = map[string]*Device{}
		m.devices[uid] = mine
	}
	d.CreatedAt, d.UpdatedAt = now, now
	if old := mine[d.InstallationID]; old != nil {
		d.CreatedAt = old.CreatedAt
		if old.Token != d.Token {
			m.dropTokenRef(uid, old)
		}
	}
	for _, old := range oldestDevices(mine, d.InstallationID) {
		m.dropTokenRef(uid, old)
		delete(mine, old.InstallationID)
	}
	out := d
	mine[d.InstallationID] = &out
	m.deviceTokens[key] = deviceRef{uid, d.InstallationID}
	res := out
	return &res, nil
}

// oldestDevices are the devices to remove so that one more (keep) fits.
func oldestDevices(mine map[string]*Device, keep string) []*Device {
	others := make([]*Device, 0, len(mine))
	for id, d := range mine {
		if id != keep {
			others = append(others, d)
		}
	}
	n := len(others) - (MaxDevicesPerUser - 1)
	if n <= 0 {
		return nil
	}
	sortOldestFirst(others)
	return others[:n]
}

// sortOldestFirst orders by UpdatedAt, then InstallationID (ties).
func sortOldestFirst(ds []*Device) {
	sort.Slice(ds, func(i, j int) bool {
		if !ds[i].UpdatedAt.Equal(ds[j].UpdatedAt) {
			return ds[i].UpdatedAt.Before(ds[j].UpdatedAt)
		}
		return ds[i].InstallationID < ds[j].InstallationID
	})
}

// dropTokenRef: callers hold m.mu.
func (m *Memory) dropTokenRef(uid string, d *Device) {
	key := tokenKey(d.Token)
	if m.deviceTokens[key] == (deviceRef{uid, d.InstallationID}) {
		delete(m.deviceTokens, key)
	}
}

// DeleteDevice implements Devices.
func (m *Memory) DeleteDevice(_ context.Context, uid, installationID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.devices[uid][installationID]
	if d == nil {
		return false, nil
	}
	m.dropTokenRef(uid, d)
	delete(m.devices[uid], installationID)
	return true, nil
}

// ListDevices implements Devices.
func (m *Memory) ListDevices(_ context.Context, uid string) ([]Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices[uid]))
	for _, d := range m.devices[uid] {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// DeleteDeviceToken implements Devices.
func (m *Memory) DeleteDeviceToken(_ context.Context, uid, token string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, d := range m.devices[uid] {
		if d.Token == token {
			m.dropTokenRef(uid, d)
			delete(m.devices[uid], id)
			n++
		}
	}
	return n, nil
}

// DeleteDevices implements Devices.
func (m *Memory) DeleteDevices(_ context.Context, uid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropDevices(uid), nil
}

// dropDevices: callers hold m.mu.
func (m *Memory) dropDevices(uid string) int {
	n := len(m.devices[uid])
	for _, d := range m.devices[uid] {
		m.dropTokenRef(uid, d)
	}
	delete(m.devices, uid)
	return n
}

// SetNotifyConsent implements NotifyConsents.
func (m *Memory) SetNotifyConsent(_ context.Context, uid, appID string, allowed bool) (*NotifyConsent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.consents == nil {
		m.consents = map[string]map[string]*NotifyConsent{}
	}
	if m.consents[uid] == nil {
		m.consents[uid] = map[string]*NotifyConsent{}
	}
	c := NotifyConsent{AppID: appID, Allowed: allowed, UpdatedAt: m.now().UTC()}
	m.consents[uid][appID] = &c
	out := c
	return &out, nil
}

// NotifyAllowed implements NotifyConsents.
func (m *Memory) NotifyAllowed(_ context.Context, uid, appID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.consents[uid][appID]
	return c != nil && c.Allowed, nil
}

// DeleteNotifyConsents implements NotifyConsents.
func (m *Memory) DeleteNotifyConsents(_ context.Context, uid string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropConsents(uid), nil
}

// dropConsents: callers hold m.mu.
func (m *Memory) dropConsents(uid string) int {
	n := len(m.consents[uid])
	delete(m.consents, uid)
	return n
}
