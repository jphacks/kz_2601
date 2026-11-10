package fixed

import (
	"context"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// ConsentAppID is the users/{uid}/notifyConsent/{id} document that records a
// sender's agreement to this push. Nothing in the app sets it yet, so no
// push is sent until the app asks (a product decision: docs/backend/
// features/feedback-loop.md). The in-app notice does not need it.
const ConsentAppID = "_feedback_fixed"

// AppName is shown as the sender of the push.
const AppName = "Tailor It"

// StoreConsent reads ConsentAppID with store.NotifyConsents.
type StoreConsent struct {
	C interface {
		NotifyAllowed(ctx context.Context, uid, appID string) (bool, error)
	}
}

// PushAllowed implements Consent.
func (s StoreConsent) PushAllowed(ctx context.Context, uid string) (bool, error) {
	return s.C.NotifyAllowed(ctx, uid, ConsentAppID)
}

// APNsPusher sends through the runner's APNs client (#269) to uid's devices
// of one environment. Rejected tokens are left alone (the runner owns them).
type APNsPusher struct {
	Sender  notify.Sender
	Devices interface {
		ListDevices(ctx context.Context, uid string) ([]store.Device, error)
	}
	Environment string // store.DeviceProduction / store.DeviceSandbox
}

// Push implements Pusher.
func (p APNsPusher) Push(ctx context.Context, uid, title, body string) (bool, error) {
	devs, err := p.Devices.ListDevices(ctx, uid)
	if err != nil {
		return false, err
	}
	sent := false
	var firstErr error
	for _, d := range devs {
		if d.Token == "" || (p.Environment != "" && d.Environment != p.Environment) {
			continue
		}
		_, err := p.Sender.Send(ctx, notify.Push{Token: d.Token, AppName: AppName, Title: title, Body: body,
			CollapseID: notify.CollapseID("loop-fixed:"+uid+":"+body, 0)})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sent = true
	}
	if sent {
		return true, nil
	}
	return false, firstErr
}
