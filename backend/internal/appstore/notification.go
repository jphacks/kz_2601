package appstore

import (
	"slices"
	"time"
)

// App Store Server Notifications v2 types Tailor It acts on (#315). Any other
// type that carries a subscription transaction is applied the same way.
const (
	NotifySubscribed             = "SUBSCRIBED"
	NotifyDidRenew               = "DID_RENEW"
	NotifyDidFailToRenew         = "DID_FAIL_TO_RENEW"
	NotifyDidChangeRenewalStatus = "DID_CHANGE_RENEWAL_STATUS"
	NotifyDidChangeRenewalPref   = "DID_CHANGE_RENEWAL_PREF"
	NotifyExpired                = "EXPIRED"
	NotifyGracePeriodExpired     = "GRACE_PERIOD_EXPIRED"
	NotifyRefund                 = "REFUND"
	NotifyRefundReversed         = "REFUND_REVERSED"
	NotifyRevoke                 = "REVOKE"
	NotifyRenewalExtended        = "RENEWAL_EXTENDED"
	NotifyTest                   = "TEST"
	SubtypeGracePeriod           = "GRACE_PERIOD"
	notificationVersion          = "2.0"
)

// Notification is a verified responseBodyV2DecodedPayload.
type Notification struct {
	Type        string
	Subtype     string
	UUID        string
	SignedDate  time.Time
	BundleID    string
	Environment string
	// SignedTransactionInfo / SignedRenewalInfo are still JWS; verify them
	// with Verify / VerifyRenewalInfo. Empty for TEST and summary notifications.
	SignedTransactionInfo string
	SignedRenewalInfo     string
}

type notificationPayload struct {
	NotificationType string `json:"notificationType"`
	Subtype          string `json:"subtype"`
	NotificationUUID string `json:"notificationUUID"`
	Version          string `json:"version"`
	Data             struct {
		BundleID              string `json:"bundleId"`
		Environment           string `json:"environment"`
		SignedTransactionInfo string `json:"signedTransactionInfo"`
		SignedRenewalInfo     string `json:"signedRenewalInfo"`
	} `json:"data"`
}

// VerifyNotification checks a notification's signedPayload (same chain and
// signature rules as Verify) and that its bundle id / environment are allowed.
// Errors are *Error.
func (v *Verifier) VerifyNotification(signedPayload string) (*Notification, error) {
	var p notificationPayload
	signedAt, err := v.verifyJWS(signedPayload, &p)
	if err != nil {
		return nil, err
	}
	if p.NotificationUUID == "" {
		return nil, reject(ReasonMalformed, "no notificationUUID")
	}
	if p.Version != notificationVersion {
		return nil, reject(ReasonMalformed, "version %q", p.Version)
	}
	n := &Notification{
		Type:                  p.NotificationType,
		Subtype:               p.Subtype,
		UUID:                  p.NotificationUUID,
		SignedDate:            signedAt.UTC(),
		BundleID:              p.Data.BundleID,
		Environment:           p.Data.Environment,
		SignedTransactionInfo: p.Data.SignedTransactionInfo,
		SignedRenewalInfo:     p.Data.SignedRenewalInfo,
	}
	if !slices.Contains(v.BundleIDs, n.BundleID) {
		return nil, reject(ReasonBundle, "bundleId %q not allowed", n.BundleID)
	}
	if !slices.Contains(v.Environments, n.Environment) {
		return nil, reject(ReasonEnvironment, "environment %q not allowed", n.Environment)
	}
	return n, nil
}

// RenewalInfo is a verified JWSRenewalInfo (the fields Tailor It uses).
type RenewalInfo struct {
	OriginalTransactionID  string
	Environment            string
	AutoRenew              bool
	GracePeriodExpiresDate time.Time // zero unless in the billing grace period
	SignedDate             time.Time
}

type renewalPayload struct {
	OriginalTransactionID  string `json:"originalTransactionId"`
	Environment            string `json:"environment"`
	AutoRenewStatus        int    `json:"autoRenewStatus"`
	GracePeriodExpiresDate int64  `json:"gracePeriodExpiresDate"`
}

// VerifyRenewalInfo checks a JWSRenewalInfo. Errors are *Error.
func (v *Verifier) VerifyRenewalInfo(signedJWS string) (*RenewalInfo, error) {
	var p renewalPayload
	signedAt, err := v.verifyJWS(signedJWS, &p)
	if err != nil {
		return nil, err
	}
	if p.OriginalTransactionID == "" {
		return nil, reject(ReasonMalformed, "renewal info without originalTransactionId")
	}
	if !slices.Contains(v.Environments, p.Environment) {
		return nil, reject(ReasonEnvironment, "environment %q not allowed", p.Environment)
	}
	return &RenewalInfo{
		OriginalTransactionID:  p.OriginalTransactionID,
		Environment:            p.Environment,
		AutoRenew:              p.AutoRenewStatus == 1,
		GracePeriodExpiresDate: millis(p.GracePeriodExpiresDate),
		SignedDate:             signedAt.UTC(),
	}, nil
}
