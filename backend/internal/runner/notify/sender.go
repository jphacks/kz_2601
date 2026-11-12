package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// Push platforms (the same strings as store.PlatformIOS / PlatformAndroid).
const (
	PlatformIOS     = "ios"     // APNs
	PlatformAndroid = "android" // FCM (#918)
)

// Device is one push target of a recipient.
type Device struct {
	Token string
	// Platform: PlatformIOS ("" counts as iOS) or PlatformAndroid.
	Platform string
}

// Push is one notification to one device.
type Push struct {
	Token string
	// Platform picks the provider (see PlatformSender); "" = iOS.
	Platform string
	// AppID opens the app on tap (inside Tailor It only, never a URL).
	AppID string
	// AppName is shown as the sender (#339: the app must be identifiable).
	AppName string
	Title   string
	Body    string
	// CollapseID makes a retried send replace, not repeat, the notification
	// (apns-collapse-id, at most 64 bytes). See CollapseID.
	CollapseID string
}

// Result classifies one send.
type Result struct {
	// Status is the provider's HTTP status (0 when no response).
	Status int
	// Reason is the provider's reason string ("" on success).
	Reason string
	// Unregistered: the token is no longer valid (APNs 410 / 400 BadDeviceToken,
	// FCM UNREGISTERED / SENDER_ID_MISMATCH).
	// Delete it; do not retry.
	Unregistered bool
	// Retryable: a later attempt may succeed (5xx, 429, network).
	Retryable bool
}

// Sender sends one push. err is nil only when the push was accepted.
type Sender interface {
	Send(ctx context.Context, p Push) (Result, error)
}

// TokenReader reads uid's devices. The runner reads users/{uid}/devices
// (registered by PUT /v1/me/devices, #269) through its appId-bound handle;
// tests may use MemoryTokens or MemoryDevices.
type TokenReader interface {
	Tokens(ctx context.Context, uid string) ([]Device, error)
}

// CollapseID derives an apns-collapse-id from the run's idempotency key and
// the action index: the same notification of a retried run collapses on the
// device. 64 hex characters (APNs allows up to 64 bytes).
func CollapseID(key string, action int) string {
	sum := sha256.Sum256([]byte(key + "\x00" + strconv.Itoa(action)))
	return hex.EncodeToString(sum[:])
}

// Content is the text of one notification and where it comes from.
type Content struct {
	// Key is the run's idempotency key (the outbox entry's).
	Key     string
	AppID   string
	AppName string
	Title   string
	Body    string
}

// DeviceToken is a token rejected as unregistered (for the caller to delete).
type DeviceToken struct {
	UID   string
	Token string
}

// Outcome is the result of one planned notification.
type Outcome struct {
	Action  int
	Refused Reason
	Sent    []string
	Skipped []Skip
	// Retryable: some sends failed in a way a retry may fix.
	Retryable bool
	// Unregistered tokens should be deleted (APNs 410, FCM UNREGISTERED).
	Unregistered []DeviceToken
}

// Summary is an Outcome without uids: what the creator and runner.usage see.
type Summary struct {
	Action  int            `json:"action"`
	Refused Reason         `json:"refused,omitempty"`
	Sent    int            `json:"sent"`
	Skipped map[Reason]int `json:"skipped,omitempty"`
}

// Summary counts the outcome by reason.
func (o Outcome) Summary() Summary {
	s := Summary{Action: o.Action, Refused: o.Refused, Sent: len(o.Sent)}
	for _, k := range o.Skipped {
		if s.Skipped == nil {
			s.Skipped = map[Reason]int{}
		}
		s.Skipped[k.Reason]++
	}
	return s
}

// Dispatcher sends a resolved notification to every device of each recipient.
type Dispatcher struct {
	Tokens TokenReader
	Sender Sender
	// CountShared writes a shared.usage notify line per recipient reached
	// (#2298; off unless SHARED_USAGE_LOG=on). The automation run sets it;
	// the notices to the creator (limits, allowance) do not.
	CountShared bool
}

// Dispatch sends c to res.To. Skips of res are carried over. A recipient
// counts as sent when at least one device accepted it. An error means a
// token read failed (retry the run; the collapse id keeps devices from
// showing it twice).
func (d *Dispatcher) Dispatch(ctx context.Context, res Resolution, c Content) (Outcome, error) {
	if d.Tokens == nil || d.Sender == nil {
		return Outcome{}, errors.New("notify: Dispatcher needs Tokens and Sender")
	}
	out := Outcome{Action: res.Action, Refused: res.Refused, Skipped: append([]Skip(nil), res.Skipped...)}
	if res.Refused != "" {
		return out, nil
	}
	collapse := CollapseID(c.Key, res.Action)
	for _, uid := range res.To {
		tokens, err := d.Tokens.Tokens(ctx, uid)
		if err != nil {
			return Outcome{}, fmt.Errorf("notify: device tokens: %w", err)
		}
		if len(tokens) == 0 {
			out.Skipped = append(out.Skipped, Skip{UID: uid, Reason: ReasonNoDevice})
			continue
		}
		sent, unregistered, retryable := false, 0, false
		for _, dev := range tokens {
			tok := dev.Token
			r, err := d.Sender.Send(ctx, Push{Token: tok, Platform: dev.Platform, AppID: c.AppID, AppName: c.AppName,
				Title: c.Title, Body: c.Body, CollapseID: collapse})
			switch {
			case err == nil:
				sent = true
			case r.Unregistered:
				unregistered++
				out.Unregistered = append(out.Unregistered, DeviceToken{UID: uid, Token: tok})
			case r.Retryable:
				retryable = true
			}
		}
		switch {
		case sent:
			out.Sent = append(out.Sent, uid)
			if d.CountShared {
				metrics.EmitSharedUsage(ctx, c.AppID, uid, metrics.SharedNotify, 1)
			}
		case unregistered == len(tokens):
			out.Skipped = append(out.Skipped, Skip{UID: uid, Reason: ReasonUnregistered})
		default:
			out.Skipped = append(out.Skipped, Skip{UID: uid, Reason: ReasonSendFailed})
			out.Retryable = out.Retryable || retryable
		}
	}
	return out, nil
}
