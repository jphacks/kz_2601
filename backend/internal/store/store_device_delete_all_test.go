package store

import "testing"

// #2562: DeleteDevices removes all of one user's devices and frees their tokens;
// other users' devices stay.
func TestStoreDeleteDevicesAll(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := t.Context()
		uid, other := "deldev"+sfx, "deldev-other"+sfx
		putDev(t, s, uid, "inst-d1", 41)
		putDev(t, s, uid, "inst-d2", 42)
		putDev(t, s, uid, "inst-d3", 43)
		putDev(t, s, other, "inst-o1", 44)

		cases := []struct {
			name string
			uid  string
			want int
		}{
			{"all of uid", uid, 3},
			{"again", uid, 0},
			{"unknown user", "deldev-none" + sfx, 0},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				n, err := s.DeleteDevices(ctx, c.uid)
				if err != nil || n != c.want {
					t.Fatalf("DeleteDevices = %d %v, want %d", n, err, c.want)
				}
				if got := deviceIDs(t, s, c.uid); len(got) != 0 {
					t.Fatalf("left: %v", got)
				}
			})
		}
		if got := deviceIDs(t, s, other); len(got) != 1 || got[0] != "inst-o1" {
			t.Fatalf("other user's devices: %v", got)
		}
		// the deleted tokens are free: registering one again keeps other's device
		putDev(t, s, uid, "inst-d1", 41)
		if got := deviceIDs(t, s, uid); len(got) != 1 {
			t.Fatalf("re-register: %v", got)
		}
		if got := deviceIDs(t, s, other); len(got) != 1 {
			t.Fatalf("other after re-register: %v", got)
		}
	})
}

// #2562: DeleteNotifyConsents removes all of one user's consents; other users' stay.
func TestStoreDeleteNotifyConsentsAll(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := t.Context()
		uid, other := "delcons"+sfx, "delcons-other"+sfx
		appA, appB := "APP-DA"+sfx, "APP-DB"+sfx
		for _, c := range []struct {
			uid, app string
			allowed  bool
		}{
			{uid, appA, true},
			{uid, appB, false},
			{other, appA, true},
		} {
			if _, err := s.SetNotifyConsent(ctx, c.uid, c.app, c.allowed); err != nil {
				t.Fatal(err)
			}
		}

		cases := []struct {
			name string
			uid  string
			want int
		}{
			{"all of uid (allowed and withdrawn)", uid, 2},
			{"again", uid, 0},
			{"unknown user", "delcons-none" + sfx, 0},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				n, err := s.DeleteNotifyConsents(ctx, c.uid)
				if err != nil || n != c.want {
					t.Fatalf("DeleteNotifyConsents = %d %v, want %d", n, err, c.want)
				}
			})
		}
		for _, app := range []string{appA, appB} {
			if ok, err := s.NotifyAllowed(ctx, uid, app); err != nil || ok {
				t.Fatalf("uid %s after delete: %v %v", app, ok, err)
			}
		}
		if ok, err := s.NotifyAllowed(ctx, other, appA); err != nil || !ok {
			t.Fatalf("other user's consent: %v %v", ok, err)
		}
	})
}
