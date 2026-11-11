package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Push environments of a device token (the APNs host it is valid on).
const (
	DeviceSandbox    = "sandbox"
	DeviceProduction = "production"
)

// Push platforms of a device (#918): the provider its token is sent through.
// A device stored before platforms existed is PlatformIOS.
const (
	PlatformIOS     = "ios"     // APNs
	PlatformAndroid = "android" // FCM
)

// MaxDevicesPerUser bounds users/{uid}/devices: registering one more removes
// the least recently updated.
const MaxDevicesPerUser = 10

// Device is users/{uid}/devices/{installationId}: one install of the app
// that can receive pushes (#269). Token is personal data: it is never
// logged and never returned by the API.
type Device struct {
	InstallationID string `json:"installationId"`
	Token          string `json:"-"`
	Platform       string `json:"platform"`
	// Environment is the APNs host an iOS token is valid on. Android
	// (FCM) tokens are valid in the Firebase project they were issued for,
	// so the runner does not filter them by it.
	Environment string    `json:"environment"`
	AppVersion  string    `json:"appVersion,omitempty"`
	OS          string    `json:"os,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Devices stores the push tokens of a user's devices (#269). The runner
// reads them only for uids that passed the recipient checks (#337).
type Devices interface {
	// PutDevice creates or replaces uid's device d.InstallationID and returns
	// it with its timestamps. The same token held by another (uid,
	// installation) is removed there: one device signed into another account
	// must not keep getting the old account's pushes. Beyond
	// MaxDevicesPerUser the least recently updated devices of uid are removed.
	PutDevice(ctx context.Context, uid string, d Device) (*Device, error)
	// DeleteDevice removes one device; false when it was not there.
	DeleteDevice(ctx context.Context, uid, installationID string) (bool, error)
	// ListDevices returns uid's devices, most recently updated first.
	ListDevices(ctx context.Context, uid string) ([]Device, error)
	// DeleteDeviceToken removes uid's devices holding token (APNs 410).
	DeleteDeviceToken(ctx context.Context, uid, token string) (int, error)
	// DeleteDevices removes all of uid's devices (account deletion: decision
	// #337 stops pushes at the logical deletion).
	DeleteDevices(ctx context.Context, uid string) (int, error)
}

// NotifyConsent is users/{uid}/notifyConsent/{appId}: whether uid agreed to
// receive the notifications others' actions send in appId (#339). No
// document = not agreed.
type NotifyConsent struct {
	AppID     string    `json:"appId"`
	Allowed   bool      `json:"allowed"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NotifyConsents stores the recipients' consent (#339).
type NotifyConsents interface {
	SetNotifyConsent(ctx context.Context, uid, appID string, allowed bool) (*NotifyConsent, error)
	// NotifyAllowed reports uid's consent for appID (false when none is stored).
	NotifyAllowed(ctx context.Context, uid, appID string) (bool, error)
	// DeleteNotifyConsents removes all of uid's consents (account purge).
	DeleteNotifyConsents(ctx context.Context, uid string) (int, error)
}

// tokenKey is the id of a token's device_tokens index document (the token
// itself is not used as an id).
func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
