package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func devToken(n int) string { return fmt.Sprintf("%064x", n) }

func deviceIDs(t *testing.T, s Store, uid string) []string {
	t.Helper()
	ds, err := s.ListDevices(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range ds {
		out = append(out, d.InstallationID)
	}
	return out
}

func putDev(t *testing.T, s Store, uid, id string, tok int) *Device {
	t.Helper()
	d, err := s.PutDevice(context.Background(), uid, Device{InstallationID: id, Token: devToken(tok), Environment: DeviceProduction, OS: "iOS 18"})
	if err != nil {
		t.Fatalf("put %s/%s: %v", uid, id, err)
	}
	return d
}

// #269: register, replace, delete, and remove by token (410).
func TestStoreDevices(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid := "dev-a" + sfx
		first := putDev(t, s, uid, "inst-0001", 1)
		if first.CreatedAt.IsZero() || !first.UpdatedAt.Equal(first.CreatedAt) {
			t.Fatalf("timestamps: %+v", first)
		}
		again := putDev(t, s, uid, "inst-0001", 2) // new token on the same install
		if !again.CreatedAt.Equal(first.CreatedAt) || again.Token != devToken(2) {
			t.Fatalf("replace: %+v (first %+v)", again, first)
		}
		putDev(t, s, uid, "inst-0002", 3)
		ds, err := s.ListDevices(ctx, uid)
		if err != nil || len(ds) != 2 {
			t.Fatalf("list: %+v %v", ds, err)
		}
		// the old token of inst-0001 is free: another user may take it
		putDev(t, s, "dev-b"+sfx, "inst-b001", 1)
		if got := deviceIDs(t, s, uid); len(got) != 2 {
			t.Fatalf("old token reuse touched uid: %v", got)
		}

		if n, err := s.DeleteDeviceToken(ctx, uid, devToken(3)); err != nil || n != 1 {
			t.Fatalf("delete token: %d %v", n, err)
		}
		if n, err := s.DeleteDeviceToken(ctx, uid, devToken(3)); err != nil || n != 0 {
			t.Fatalf("delete token again: %d %v", n, err)
		}
		if ok, err := s.DeleteDevice(ctx, uid, "inst-0001"); err != nil || !ok {
			t.Fatalf("delete: %v %v", ok, err)
		}
		if ok, err := s.DeleteDevice(ctx, uid, "inst-0001"); err != nil || ok {
			t.Fatalf("delete again: %v %v", ok, err)
		}
		if got := deviceIDs(t, s, uid); len(got) != 0 {
			t.Fatalf("left: %v", got)
		}
	})
}

// One device signed into another account stops getting the first account's
// pushes; the same user re-installing keeps one entry per token.
func TestStoreDeviceTokenMoves(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		a, b := "move-a"+sfx, "move-b"+sfx
		putDev(t, s, a, "inst-shared", 7)
		putDev(t, s, b, "inst-shared", 7)
		if got := deviceIDs(t, s, a); len(got) != 0 {
			t.Fatalf("a still holds the token: %v", got)
		}
		if got := deviceIDs(t, s, b); len(got) != 1 {
			t.Fatalf("b: %v", got)
		}
		putDev(t, s, b, "inst-reinstalled", 7)
		if got := deviceIDs(t, s, b); len(got) != 1 || got[0] != "inst-reinstalled" {
			t.Fatalf("reinstall: %v", got)
		}
		// deleting b's device frees the index: a can register the token again
		if _, err := s.DeleteDevice(context.Background(), b, "inst-reinstalled"); err != nil {
			t.Fatal(err)
		}
		putDev(t, s, a, "inst-a", 7)
		if got := deviceIDs(t, s, a); len(got) != 1 {
			t.Fatalf("a again: %v", got)
		}
	})
}

// At most MaxDevicesPerUser: the least recently updated goes.
func TestStoreDeviceLimit(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		uid := "lim" + sfx
		if m, ok := s.(*Memory); ok { // distinct times, as Firestore's round trips give
			at := time.Now()
			m.now = func() time.Time { at = at.Add(time.Millisecond); return at }
		}
		for i := range MaxDevicesPerUser {
			putDev(t, s, uid, fmt.Sprintf("inst-%04d", i), 100+i)
		}
		putDev(t, s, uid, "inst-0000", 100) // refresh the oldest: inst-0001 is now the oldest
		putDev(t, s, uid, "inst-new1", 200)
		got := deviceIDs(t, s, uid)
		if len(got) != MaxDevicesPerUser {
			t.Fatalf("count: %d %v", len(got), got)
		}
		joined := strings.Join(got, ",")
		if strings.Contains(joined, "inst-0001") || !strings.Contains(joined, "inst-0000") || got[0] != "inst-new1" {
			t.Fatalf("evicted the wrong one: %v", got)
		}
		// the evicted token is free for someone else, and the new one stays
		putDev(t, s, "lim-other"+sfx, "inst-o", 101)
		if len(deviceIDs(t, s, uid)) != MaxDevicesPerUser {
			t.Fatal("re-using an evicted token removed a device")
		}
	})
}

// Decision #337: devices go at the logical deletion; the purge counts
// leftovers and erases consents.
func TestStoreDevicesAccountDeletion(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid := "leave" + sfx
		putDev(t, s, uid, "inst-l1", 31)
		putDev(t, s, uid, "inst-l2", 32)
		if _, err := s.SetNotifyConsent(ctx, uid, "APP-L"+sfx, true); err != nil {
			t.Fatal(err)
		}
		if err := s.DeactivateUser(ctx, uid); err != nil {
			t.Fatal(err)
		}
		if got := deviceIDs(t, s, uid); len(got) != 0 {
			t.Fatalf("devices after deletion: %v", got)
		}
		// the tokens are free again
		putDev(t, s, "leave-next"+sfx, "inst-n", 31)
		res, err := s.PurgeAccount(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		if res.Devices != 0 || res.NotifyConsents != 1 {
			t.Fatalf("purge: %+v", res)
		}
		if ok, err := s.NotifyAllowed(ctx, uid, "APP-L"+sfx); err != nil || ok {
			t.Fatalf("consent after purge: %v %v", ok, err)
		}
		if len(deviceIDs(t, s, "leave-next"+sfx)) != 1 {
			t.Fatal("another user's device was removed")
		}
	})
}

// #339: consent per (uid, app); none stored = not agreed.
func TestStoreNotifyConsent(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		uid, app := "cons"+sfx, "APP-C"+sfx
		if ok, err := s.NotifyAllowed(ctx, uid, app); err != nil || ok {
			t.Fatalf("default: %v %v", ok, err)
		}
		c, err := s.SetNotifyConsent(ctx, uid, app, true)
		if err != nil || !c.Allowed || c.AppID != app || c.UpdatedAt.IsZero() {
			t.Fatalf("set: %+v %v", c, err)
		}
		if ok, _ := s.NotifyAllowed(ctx, uid, app); !ok {
			t.Fatal("not stored")
		}
		if ok, _ := s.NotifyAllowed(ctx, uid, app+"-other"); ok {
			t.Fatal("leaked to another app")
		}
		if ok, _ := s.NotifyAllowed(ctx, uid+"-other", app); ok {
			t.Fatal("leaked to another user")
		}
		if _, err := s.SetNotifyConsent(ctx, uid, app, false); err != nil {
			t.Fatal(err)
		}
		if ok, _ := s.NotifyAllowed(ctx, uid, app); ok {
			t.Fatal("withdrawal not stored")
		}
	})
}
