package entitlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"

	"github.com/jphacks/kz_2601/backend/internal/googleplay"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// PlayNotificationsPath receives Google Play Real-time developer notifications
// through a Pub/Sub push subscription. docs/android/play-billing.md
const PlayNotificationsPath = "/v1/googleplay/notifications"

// maxPushBytes bounds a Pub/Sub push body (a notification is < 1 KB).
const maxPushBytes = 64 << 10

// voidedSubscription is voidedPurchaseNotification.productType of a subscription.
const voidedSubscription = 1

// PushVerifier authenticates a Pub/Sub push request (its Authorization header).
type PushVerifier interface {
	Verify(ctx context.Context, authorization string) error
}

// OIDCPushVerifier checks the OIDC token Pub/Sub signs for a push subscription
// with authentication: audience and the push service account's email.
type OIDCPushVerifier struct {
	Audience string
	Email    string
	// Validate is idtoken.Validate (tests replace it).
	Validate func(ctx context.Context, token, audience string) (*idtoken.Payload, error)
}

// Verify implements PushVerifier.
func (v *OIDCPushVerifier) Verify(ctx context.Context, authorization string) error {
	tok, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || tok == "" {
		return errors.New("no bearer token")
	}
	validate := v.Validate
	if validate == nil {
		validate = idtoken.Validate
	}
	p, err := validate(ctx, tok, v.Audience)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	email, _ := p.Claims["email"].(string)
	verified, _ := p.Claims["email_verified"].(bool)
	if email != v.Email || !verified {
		return errors.New("token is not from the push service account")
	}
	return nil
}

// PlayNotifications applies Real-time developer notifications: renewals,
// cancellations, holds, refunds. The notification only names the purchase
// token; the state is read again from the Play Developer API and stored for
// the uid the token is bound to (POST /v1/me/entitlements/google bound it).
type PlayNotifications struct {
	Service  *Service
	Verifier PushVerifier
	Log      *slog.Logger
}

func (n *PlayNotifications) log() *slog.Logger {
	if n.Log != nil {
		return n.Log
	}
	return slog.Default()
}

// developerNotification is the Pub/Sub message data.
type developerNotification struct {
	PackageName              string `json:"packageName"`
	SubscriptionNotification *struct {
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
		SubscriptionID   string `json:"subscriptionId"`
	} `json:"subscriptionNotification"`
	VoidedPurchaseNotification *struct {
		PurchaseToken string `json:"purchaseToken"`
		ProductType   int    `json:"productType"`
	} `json:"voidedPurchaseNotification"`
	TestNotification *struct{} `json:"testNotification"`
}

// Handle applies one notification (the decoded message data). Outcomes:
// applied / unbound (no uid has sent the purchase yet) / test / ignored.
// An error is transient (Pub/Sub redelivers).
func (n *PlayNotifications) Handle(ctx context.Context, data []byte) (string, error) {
	gp := n.Service.GooglePlay
	var dn developerNotification
	if err := json.Unmarshal(data, &dn); err != nil {
		n.log().WarnContext(ctx, "play notification: not JSON", slog.Any("error", err))
		return "ignored", nil
	}
	if dn.PackageName != gp.Policy.PackageName {
		n.log().InfoContext(ctx, "play notification: other package", slog.String("packageName", dn.PackageName))
		return "ignored", nil
	}
	var token string
	attrs := []any{}
	switch {
	case dn.TestNotification != nil:
		n.log().InfoContext(ctx, "play notification: test")
		return "test", nil
	case dn.SubscriptionNotification != nil:
		token = dn.SubscriptionNotification.PurchaseToken
		attrs = append(attrs, slog.Int("notificationType", dn.SubscriptionNotification.NotificationType))
	case dn.VoidedPurchaseNotification != nil && dn.VoidedPurchaseNotification.ProductType == voidedSubscription:
		token = dn.VoidedPurchaseNotification.PurchaseToken
		attrs = append(attrs, slog.Bool("voided", true))
	default:
		return "ignored", nil
	}
	if token == "" {
		return "ignored", nil
	}
	outcome, owner, state, err := n.refresh(ctx, token)
	if err != nil {
		return "", err
	}
	if owner != "" {
		attrs = append(attrs, slog.String("account", hash(owner)), slog.String("state", state))
	}
	n.log().InfoContext(ctx, "play notification: "+outcome, attrs...)
	return outcome, nil
}

// refresh re-reads token's purchase and stores it for the bound uid.
func (n *PlayNotifications) refresh(ctx context.Context, token string) (outcome, owner, state string, err error) {
	s, gp := n.Service, n.Service.GooglePlay
	key := PlayKey(token)
	if owner, err = s.Store.Owner(ctx, key); err != nil {
		return "", "", "", fmt.Errorf("owner: %w", err)
	}
	if owner == "" {
		return "unbound", "", "", nil
	}
	sub, err := gp.API.Subscription(ctx, gp.Policy.PackageName, token)
	if errors.Is(err, googleplay.ErrNotFound) {
		return "ignored", owner, "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	item, ok := sub.Item(gp.Policy.ProductIDs)
	if !ok || item.ExpiryTime.IsZero() {
		return "ignored", owner, sub.State, nil
	}
	if _, err := s.Store.Apply(ctx, owner, FromPlay(sub, item, key, s.now()), ""); err != nil {
		return "", "", "", fmt.Errorf("apply: %w", err)
	}
	if s.Changed != nil {
		s.Changed(owner)
	}
	return "applied", owner, sub.State, nil
}

// RegisterPlayNotifications mounts POST PlayNotificationsPath on a public mux
// (no Firebase / App Check: the Pub/Sub OIDC token authenticates it).
//
//	200 {"outcome": "..."}   Pub/Sub stops redelivering
//	400 / 401                malformed / not from the push subscription
//	500                      transient; Pub/Sub redelivers
func RegisterPlayNotifications(mux *http.ServeMux, n *PlayNotifications) {
	mux.HandleFunc("POST "+PlayNotificationsPath, func(w http.ResponseWriter, r *http.Request) {
		if err := n.Verifier.Verify(r.Context(), r.Header.Get("Authorization")); err != nil {
			slog.WarnContext(r.Context(), "play notification: unauthenticated", slog.Any("error", err))
			httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "unauthenticated")
			return
		}
		var req struct {
			Message struct {
				Data []byte `json:"data"` // base64 in JSON
			} `json:"message"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPushBytes)).Decode(&req); err != nil || len(req.Message.Data) == 0 {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "message.data is required")
			return
		}
		outcome, err := n.Handle(r.Context(), req.Message.Data)
		if err != nil {
			slog.ErrorContext(r.Context(), "play notification: failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"outcome": outcome})
	})
}
