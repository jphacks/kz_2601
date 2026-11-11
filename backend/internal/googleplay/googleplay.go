// Package googleplay reads Google Play subscription purchases (Tailor It Pro
// on Android) through the Google Play Developer API with a service account.
// It never reads a key file itself and never logs the key or a purchase token.
//
//	purchases.subscriptionsv2.get        Subscription
//	purchases.subscriptions.acknowledge  Acknowledge
package googleplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	androidpublisher "google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// SubscriptionPurchaseV2.subscriptionState values.
const (
	StateActive                  = "SUBSCRIPTION_STATE_ACTIVE"
	StateCanceled                = "SUBSCRIPTION_STATE_CANCELED" // auto-renew off; entitled until expiry
	StateInGracePeriod           = "SUBSCRIPTION_STATE_IN_GRACE_PERIOD"
	StateOnHold                  = "SUBSCRIPTION_STATE_ON_HOLD"
	StatePaused                  = "SUBSCRIPTION_STATE_PAUSED"
	StateExpired                 = "SUBSCRIPTION_STATE_EXPIRED"
	StatePending                 = "SUBSCRIPTION_STATE_PENDING"
	StatePendingPurchaseCanceled = "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED"
)

// SubscriptionPurchaseV2.acknowledgementState values.
const (
	AckPending      = "ACKNOWLEDGEMENT_STATE_PENDING"
	AckAcknowledged = "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"
)

// ErrNotFound: Play does not know the token for the package (or it is malformed).
var ErrNotFound = errors.New("googleplay: purchase not found")

// LineItem is one product of a subscription purchase.
type LineItem struct {
	ProductID  string
	ExpiryTime time.Time // zero while pending
	AutoRenew  bool
	OrderID    string // latestSuccessfulOrderId (changes on every renewal)
}

// Subscription is the part of SubscriptionPurchaseV2 Tailor It uses.
type Subscription struct {
	State                string
	AcknowledgementState string
	LinkedPurchaseToken  string // the purchase this one replaced (upgrade, resubscribe)
	ObfuscatedAccountID  string // externalAccountIdentifiers.obfuscatedExternalAccountId
	Test                 bool   // testPurchase is set (license tester)
	LineItems            []LineItem
}

// Entitles reports whether the state grants access until the expiry.
func (s *Subscription) Entitles() bool {
	return s.State == StateActive || s.State == StateInGracePeriod || s.State == StateCanceled
}

// Item returns the line item of an allowed product with the latest expiry.
func (s *Subscription) Item(productIDs []string) (LineItem, bool) {
	var out LineItem
	found := false
	for _, li := range s.LineItems {
		if !slices.Contains(productIDs, li.ProductID) {
			continue
		}
		if !found || li.ExpiryTime.After(out.ExpiryTime) {
			out, found = li, true
		}
	}
	return out, found
}

// API is the Play Developer API Tailor It calls (*Client; a fake in tests).
type API interface {
	Subscription(ctx context.Context, packageName, token string) (*Subscription, error)
	Acknowledge(ctx context.Context, packageName, productID, token string) error
}

// Client implements API with androidpublisher v3.
type Client struct {
	svc *androidpublisher.Service
}

var _ API = (*Client)(nil)

// CheckKey reports whether credentialsJSON is a service account key. Its
// errors carry no key material.
func CheckKey(credentialsJSON []byte) error {
	var k struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(credentialsJSON, &k); err != nil {
		return errors.New("googleplay: the key is not JSON")
	}
	if k.Type != "service_account" {
		return errors.New("googleplay: the key is not a service account key")
	}
	return nil
}

// New builds a Client from a service account key (JSON). It does not call
// Google. opts are added after the credentials (tests: option.WithEndpoint).
func New(ctx context.Context, credentialsJSON []byte, opts ...option.ClientOption) (*Client, error) {
	if err := CheckKey(credentialsJSON); err != nil {
		return nil, err
	}
	all := append([]option.ClientOption{
		option.WithAuthCredentialsJSON(option.ServiceAccount, credentialsJSON),
		option.WithScopes(androidpublisher.AndroidpublisherScope),
	}, opts...)
	svc, err := androidpublisher.NewService(ctx, all...)
	if err != nil {
		return nil, fmt.Errorf("googleplay: init: %w", err)
	}
	return &Client{svc: svc}, nil
}

// NewWithOptions builds a Client with opts only (tests against a local server).
func NewWithOptions(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	svc, err := androidpublisher.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("googleplay: init: %w", err)
	}
	return &Client{svc: svc}, nil
}

// Subscription implements API (purchases.subscriptionsv2.get).
func (c *Client) Subscription(ctx context.Context, packageName, token string) (*Subscription, error) {
	r, err := c.svc.Purchases.Subscriptionsv2.Get(packageName, token).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	return fromV2(r), nil
}

// Acknowledge implements API (purchases.subscriptions.acknowledge).
func (c *Client) Acknowledge(ctx context.Context, packageName, productID, token string) error {
	err := c.svc.Purchases.Subscriptions.Acknowledge(packageName, productID, token,
		&androidpublisher.SubscriptionPurchasesAcknowledgeRequest{}).Context(ctx).Do()
	if err != nil {
		return apiErr(err)
	}
	return nil
}

// apiErr maps "no such token" answers to ErrNotFound. The error text never
// includes the token (it is in the URL path).
func apiErr(err error) error {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch gerr.Code {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusGone:
			return fmt.Errorf("%w (HTTP %d)", ErrNotFound, gerr.Code)
		}
		return fmt.Errorf("googleplay: HTTP %d", gerr.Code)
	}
	var uerr *url.Error
	if errors.As(err, &uerr) { // its URL holds the token
		return fmt.Errorf("googleplay: %s: %w", uerr.Op, uerr.Err)
	}
	return fmt.Errorf("googleplay: %w", err)
}

func fromV2(r *androidpublisher.SubscriptionPurchaseV2) *Subscription {
	s := &Subscription{
		State:                r.SubscriptionState,
		AcknowledgementState: r.AcknowledgementState,
		LinkedPurchaseToken:  r.LinkedPurchaseToken,
		Test:                 r.TestPurchase != nil,
	}
	if r.ExternalAccountIdentifiers != nil {
		s.ObfuscatedAccountID = r.ExternalAccountIdentifiers.ObfuscatedExternalAccountId
	}
	for _, li := range r.LineItems {
		if li == nil {
			continue
		}
		item := LineItem{ProductID: li.ProductId, OrderID: li.LatestSuccessfulOrderId}
		if t, err := time.Parse(time.RFC3339Nano, li.ExpiryTime); err == nil {
			item.ExpiryTime = t
		}
		if li.AutoRenewingPlan != nil {
			item.AutoRenew = li.AutoRenewingPlan.AutoRenewEnabled
		}
		s.LineItems = append(s.LineItems, item)
	}
	return s
}

// Error is a purchase that does not grant Pro (422 invalid_transaction).
type Error struct {
	Reason string // not_found / product_id / pending
	Err    error
}

func (e *Error) Error() string { return "googleplay: " + e.Reason + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Reject builds an *Error.
func Reject(reason, format string, a ...any) error {
	return &Error{Reason: reason, Err: fmt.Errorf(format, a...)}
}

// Policy is what the server accepts.
type Policy struct {
	PackageName string
	ProductIDs  []string
}
