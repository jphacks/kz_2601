package store

import (
	"context"
	"sort"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	devicesCollection       = "devices"       // under users/{uid}
	notifyConsentCollection = "notifyConsent" // under users/{uid}
	// device_tokens/{tokenKey} {uid, installationId}: who holds a token, so
	// that registering it under another account removes the old one.
	deviceTokensCollection = "device_tokens"
)

type deviceDoc struct {
	Token       string    `firestore:"token"`
	Platform    string    `firestore:"platform,omitempty"` // "" = ios (stored before #918)
	Environment string    `firestore:"environment"`
	AppVersion  string    `firestore:"appVersion,omitempty"`
	OS          string    `firestore:"os,omitempty"`
	CreatedAt   time.Time `firestore:"createdAt"`
	UpdatedAt   time.Time `firestore:"updatedAt"`
}

func (d deviceDoc) device(id string) Device {
	p := d.Platform
	if p == "" {
		p = PlatformIOS
	}
	return Device{InstallationID: id, Token: d.Token, Platform: p, Environment: d.Environment, AppVersion: d.AppVersion,
		OS: d.OS, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

type deviceRefDoc struct {
	UID            string `firestore:"uid"`
	InstallationID string `firestore:"installationId"`
}

type consentDoc struct {
	Allowed   bool      `firestore:"allowed"`
	UpdatedAt time.Time `firestore:"updatedAt"`
}

func (f *Firestore) devices(uid string) *firestore.CollectionRef {
	return f.userRef(uid).Collection(devicesCollection)
}

func (f *Firestore) tokenRef(token string) *firestore.DocumentRef {
	return f.c.Collection(deviceTokensCollection).Doc(tokenKey(token))
}

// PutDevice implements Devices in one transaction: the device, the token
// index, the token's previous holder and the devices over the limit.
func (f *Firestore) PutDevice(ctx context.Context, uid string, d Device) (*Device, error) {
	ref := f.devices(uid).Doc(d.InstallationID)
	idx := f.tokenRef(d.Token)
	var out Device
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		now := Now()
		// reads
		var drop []*firestore.DocumentRef
		var holder deviceRefDoc
		if snap, err := tx.Get(idx); err == nil {
			if err := snap.DataTo(&holder); err != nil {
				return err
			}
		} else if !isNotFound(err) {
			return err
		}
		if holder.UID != "" && (holder.UID != uid || holder.InstallationID != d.InstallationID) {
			href := f.devices(holder.UID).Doc(holder.InstallationID)
			if snap, err := tx.Get(href); err == nil {
				if t, _ := snap.DataAt("token"); t == d.Token {
					drop = append(drop, href)
				}
			} else if !isNotFound(err) {
				return err
			}
		}
		snaps, err := tx.Documents(f.devices(uid).Query).GetAll()
		if err != nil {
			return err
		}
		var mine []*Device
		created := now
		var stale []*Device // devices of uid whose token index entry may go
		for _, s := range snaps {
			var doc deviceDoc
			if err := s.DataTo(&doc); err != nil {
				return err
			}
			dev := doc.device(s.Ref.ID)
			if holder.UID == uid && dev.InstallationID == holder.InstallationID && dev.InstallationID != d.InstallationID && dev.Token == d.Token {
				continue // the token's previous holder: already in drop
			}
			if dev.InstallationID == d.InstallationID {
				created = dev.CreatedAt
				if dev.Token != d.Token {
					stale = append(stale, &dev)
				}
				continue
			}
			mine = append(mine, &dev)
		}
		if n := len(mine) - (MaxDevicesPerUser - 1); n > 0 {
			sortOldestFirst(mine)
			for _, old := range mine[:n] {
				drop = append(drop, f.devices(uid).Doc(old.InstallationID))
				stale = append(stale, old)
			}
		}
		var dropIdx []*firestore.DocumentRef
		for _, old := range stale {
			r := f.tokenRef(old.Token)
			if r.ID == idx.ID {
				continue // rewritten below
			}
			snap, err := tx.Get(r)
			if isNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			var h deviceRefDoc
			if err := snap.DataTo(&h); err != nil {
				return err
			}
			if h.UID == uid && h.InstallationID == old.InstallationID {
				dropIdx = append(dropIdx, r)
			}
		}
		// writes
		for _, r := range append(drop, dropIdx...) {
			if err := tx.Delete(r); err != nil {
				return err
			}
		}
		doc := deviceDoc{Token: d.Token, Platform: d.Platform, Environment: d.Environment, AppVersion: d.AppVersion, OS: d.OS,
			CreatedAt: created, UpdatedAt: now}
		if err := tx.Set(ref, doc); err != nil {
			return err
		}
		if err := tx.Set(idx, deviceRefDoc{UID: uid, InstallationID: d.InstallationID}); err != nil {
			return err
		}
		out = doc.device(d.InstallationID)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDevice implements Devices.
func (f *Firestore) DeleteDevice(ctx context.Context, uid, installationID string) (bool, error) {
	return f.removeDevice(ctx, uid, installationID, "")
}

// removeDevice deletes uid's device (only when it holds token, if token is
// set) and its token index entry when that still points to it.
func (f *Firestore) removeDevice(ctx context.Context, uid, installationID, token string) (bool, error) {
	ref := f.devices(uid).Doc(installationID)
	removed := false
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		removed = false
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var doc deviceDoc
		if err := snap.DataTo(&doc); err != nil {
			return err
		}
		if token != "" && doc.Token != token {
			return nil
		}
		idx := f.tokenRef(doc.Token)
		var h deviceRefDoc
		ownIdx := false
		if is, err := tx.Get(idx); err == nil {
			if err := is.DataTo(&h); err != nil {
				return err
			}
			ownIdx = h.UID == uid && h.InstallationID == installationID
		} else if !isNotFound(err) {
			return err
		}
		if err := tx.Delete(ref); err != nil {
			return err
		}
		if ownIdx {
			if err := tx.Delete(idx); err != nil {
				return err
			}
		}
		removed = true
		return nil
	})
	return removed, err
}

// ListDevices implements Devices.
func (f *Firestore) ListDevices(ctx context.Context, uid string) ([]Device, error) {
	var out []Device
	err := each(ctx, f.devices(uid).Query, func(s *firestore.DocumentSnapshot) error {
		var doc deviceDoc
		if err := s.DataTo(&doc); err != nil {
			return err
		}
		out = append(out, doc.device(s.Ref.ID))
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}

// DeleteDeviceToken implements Devices.
func (f *Firestore) DeleteDeviceToken(ctx context.Context, uid, token string) (int, error) {
	return f.removeDevices(ctx, uid, f.devices(uid).Where("token", "==", token), token)
}

// DeleteDevices implements Devices.
func (f *Firestore) DeleteDevices(ctx context.Context, uid string) (int, error) {
	return f.removeDevices(ctx, uid, f.devices(uid).Query, "")
}

func (f *Firestore) removeDevices(ctx context.Context, uid string, q firestore.Query, token string) (int, error) {
	var ids []string
	if err := each(ctx, q, func(s *firestore.DocumentSnapshot) error {
		ids = append(ids, s.Ref.ID)
		return nil
	}); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		ok, err := f.removeDevice(ctx, uid, id, token)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

// SetNotifyConsent implements NotifyConsents.
func (f *Firestore) SetNotifyConsent(ctx context.Context, uid, appID string, allowed bool) (*NotifyConsent, error) {
	doc := consentDoc{Allowed: allowed, UpdatedAt: Now()}
	if _, err := f.userRef(uid).Collection(notifyConsentCollection).Doc(appID).Set(ctx, doc); err != nil {
		return nil, err
	}
	return &NotifyConsent{AppID: appID, Allowed: allowed, UpdatedAt: doc.UpdatedAt}, nil
}

// NotifyAllowed implements NotifyConsents.
func (f *Firestore) NotifyAllowed(ctx context.Context, uid, appID string) (bool, error) {
	snap, err := f.userRef(uid).Collection(notifyConsentCollection).Doc(appID).Get(ctx)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var doc consentDoc
	if err := snap.DataTo(&doc); err != nil {
		return false, err
	}
	return doc.Allowed, nil
}

// DeleteNotifyConsents implements NotifyConsents.
func (f *Firestore) DeleteNotifyConsents(ctx context.Context, uid string) (int, error) {
	return f.deleteQuery(ctx, f.userRef(uid).Collection(notifyConsentCollection).Query, nil)
}
