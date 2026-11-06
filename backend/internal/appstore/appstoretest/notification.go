package appstoretest

import (
	"testing"
	"time"
)

// RenewalInfo is a JWSRenewalInfo payload.
type RenewalInfo struct {
	OriginalTransactionID  string `json:"originalTransactionId"`
	ProductID              string `json:"productId"`
	AutoRenewProductID     string `json:"autoRenewProductId"`
	AutoRenewStatus        int    `json:"autoRenewStatus"`
	Environment            string `json:"environment"`
	GracePeriodExpiresDate int64  `json:"gracePeriodExpiresDate,omitempty"`
	SignedDate             int64  `json:"signedDate"`
}

// Renewal is the renewal info of tx (auto-renew on, no grace period).
func Renewal(tx Transaction) RenewalInfo {
	return RenewalInfo{
		OriginalTransactionID: tx.OriginalTransactionID,
		ProductID:             tx.ProductID,
		AutoRenewProductID:    tx.ProductID,
		AutoRenewStatus:       1,
		Environment:           tx.Environment,
		SignedDate:            tx.SignedDate,
	}
}

// NotificationData is responseBodyV2DecodedPayload.data.
type NotificationData struct {
	BundleID              string `json:"bundleId"`
	Environment           string `json:"environment"`
	SignedTransactionInfo string `json:"signedTransactionInfo,omitempty"`
	SignedRenewalInfo     string `json:"signedRenewalInfo,omitempty"`
	Status                int    `json:"status,omitempty"`
}

// NotificationPayload is responseBodyV2DecodedPayload.
type NotificationPayload struct {
	NotificationType string           `json:"notificationType"`
	Subtype          string           `json:"subtype,omitempty"`
	NotificationUUID string           `json:"notificationUUID"`
	Version          string           `json:"version"`
	SignedDate       int64            `json:"signedDate"`
	Data             NotificationData `json:"data"`
}

// Notification signs tx (and renewal, when non-nil) and wraps them in a
// signed notification of type / subtype; the result is the signedPayload.
// signedDate of all three is at.
func (s *Signer) Notification(t testing.TB, uuid, typ, subtype string, tx Transaction, renewal *RenewalInfo, at time.Time) string {
	t.Helper()
	tx.SignedDate = at.UnixMilli()
	data := NotificationData{BundleID: tx.BundleID, Environment: tx.Environment, SignedTransactionInfo: s.Sign(t, tx)}
	if renewal != nil {
		r := *renewal
		r.SignedDate = at.UnixMilli()
		data.SignedRenewalInfo = s.Sign(t, r)
	}
	return s.Sign(t, NotificationPayload{
		NotificationType: typ, Subtype: subtype, NotificationUUID: uuid, Version: "2.0",
		SignedDate: at.UnixMilli(), Data: data,
	})
}
