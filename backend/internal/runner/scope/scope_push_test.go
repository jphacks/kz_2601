package scope

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

var errPush = errors.New("devices down")

// pushBackend can fail the device and account reads.
type pushBackend struct {
	*store.Memory
	fail bool
}

func (b *pushBackend) ListDevices(ctx context.Context, uid string) ([]store.Device, error) {
	if b.fail {
		return nil, errPush
	}
	return b.Memory.ListDevices(ctx, uid)
}

func (b *pushBackend) DeleteDeviceToken(ctx context.Context, uid, token string) (int, error) {
	if b.fail {
		return 0, errPush
	}
	return b.Memory.DeleteDeviceToken(ctx, uid, token)
}

func (b *pushBackend) UserDeleted(ctx context.Context, uid string) (bool, error) {
	if b.fail {
		return false, errPush
	}
	return b.Memory.UserDeleted(ctx, uid)
}

func pushHandle(t *testing.T, b Backend) *Handle {
	t.Helper()
	h, err := New(b, "test", store.OutboxEvent{AppID: "S", Scope: "shared"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func installationIDs(ds []store.Device) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.InstallationID)
	}
	sort.Strings(out)
	return out
}

// PushDevices: iOS devices of the APNs environment asked for (all when it is
// ""), every Android device; nothing for a user without devices. Consent is
// not checked here (the notifier asks Allows first).
func TestHandlePushDevices(t *testing.T) {
	ctx := context.Background()
	b := &pushBackend{Memory: store.NewMemory()}
	for uid, ds := range map[string][]store.Device{
		"many": {
			{InstallationID: "ios-sb", Token: "t1", Platform: store.PlatformIOS, Environment: store.DeviceSandbox},
			{InstallationID: "ios-prod", Token: "t2", Platform: store.PlatformIOS, Environment: store.DeviceProduction},
			{InstallationID: "android", Token: "t3", Platform: store.PlatformAndroid},
		},
		"one":       {{InstallationID: "only", Token: "t4", Environment: store.DeviceSandbox}}, // platform defaults to iOS
		"noconsent": {{InstallationID: "nc", Token: "t5", Platform: store.PlatformAndroid}},
	} {
		for _, d := range ds {
			if _, err := b.PutDevice(ctx, uid, d); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := b.SetNotifyConsent(ctx, "noconsent", "S", false); err != nil {
		t.Fatal(err)
	}
	h := pushHandle(t, b)
	cases := []struct {
		name, uid, env string
		want           []string
	}{
		{"all environments", "many", "", []string{"android", "ios-prod", "ios-sb"}},
		{"sandbox", "many", store.DeviceSandbox, []string{"android", "ios-sb"}},
		{"production", "many", store.DeviceProduction, []string{"android", "ios-prod"}},
		{"one device", "one", store.DeviceSandbox, []string{"only"}},
		{"other environment", "one", store.DeviceProduction, []string{}},
		{"no devices", "nobody", "", []string{}},
		{"no consent", "noconsent", store.DeviceProduction, []string{"nc"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ds, err := h.PushDevices(ctx, c.uid, c.env)
			if err != nil {
				t.Fatal(err)
			}
			if got := installationIDs(ds); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("devices %v, want %v", got, c.want)
			}
			for _, d := range ds {
				if d.Token == "" {
					t.Fatalf("device without token: %+v", d)
				}
			}
		})
	}
	b.fail = true
	if _, err := h.PushDevices(ctx, "many", ""); !errors.Is(err, errPush) {
		t.Fatalf("devices error: %v", err)
	}
}

// DropDeviceToken: the devices of uid holding a token the push service
// rejected stop being recipients; other users' devices and an unknown
// token are left alone.
func TestHandleDropDeviceToken(t *testing.T) {
	ctx := context.Background()
	b := &pushBackend{Memory: store.NewMemory()}
	for _, x := range []struct {
		uid string
		d   store.Device
	}{
		{"u", store.Device{InstallationID: "a", Token: "dead", Platform: store.PlatformAndroid}},
		{"u", store.Device{InstallationID: "b", Token: "live", Platform: store.PlatformAndroid}},
		{"v", store.Device{InstallationID: "c", Token: "other", Platform: store.PlatformAndroid}},
	} {
		if _, err := b.PutDevice(ctx, x.uid, x.d); err != nil {
			t.Fatal(err)
		}
	}
	h := pushHandle(t, b)
	recipients := func(uid string) []string {
		t.Helper()
		ds, err := h.PushDevices(ctx, uid, "")
		if err != nil {
			t.Fatal(err)
		}
		return installationIDs(ds)
	}
	if err := h.DropDeviceToken(ctx, "u", "dead"); err != nil {
		t.Fatal(err)
	}
	if got := recipients("u"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("after drop: %v", got)
	}
	// someone else's token and an unknown token: no error, nothing removed
	for _, tok := range []string{"other", "unknown"} {
		if err := h.DropDeviceToken(ctx, "u", tok); err != nil {
			t.Fatalf("drop %s: %v", tok, err)
		}
	}
	if got := recipients("u"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("u after no-op drops: %v", got)
	}
	if got := recipients("v"); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("v: %v", got)
	}
	b.fail = true
	if err := h.DropDeviceToken(ctx, "u", "live"); !errors.Is(err, errPush) {
		t.Fatalf("drop error: %v", err)
	}
}

// Deleted: true once the account is deactivated and after it is purged;
// false for a live or unknown user.
func TestHandleDeleted(t *testing.T) {
	ctx := context.Background()
	b := &pushBackend{Memory: store.NewMemory()}
	for _, uid := range []string{"gone", "purged"} {
		if err := b.DeactivateUser(ctx, uid); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.MarkAccountPurged(ctx, "purged"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SeeUser(ctx, "live"); err != nil {
		t.Fatal(err)
	}
	h := pushHandle(t, b)
	for _, c := range []struct {
		uid  string
		want bool
	}{
		{"gone", true},
		{"purged", true},
		{"live", false},
		{"unknown", false},
	} {
		t.Run(c.uid, func(t *testing.T) {
			got, err := h.Deleted(ctx, c.uid)
			if err != nil || got != c.want {
				t.Fatalf("deleted %s: %v %v, want %v", c.uid, got, err, c.want)
			}
		})
	}
	b.fail = true
	if _, err := h.Deleted(ctx, "gone"); !errors.Is(err, errPush) {
		t.Fatalf("deleted error: %v", err)
	}
}
