package googleplay

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
)

// fakeServer answers like the Play Developer API (no real calls).
func fakeServer(t *testing.T) (*Client, *[]string) {
	t.Helper()
	var acks []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/androidpublisher/v3/applications/jp.kz2601.metamon/purchases/subscriptionsv2/tokens/tok":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"subscriptionState": "SUBSCRIPTION_STATE_ACTIVE",
				"acknowledgementState": "ACKNOWLEDGEMENT_STATE_PENDING",
				"linkedPurchaseToken": "old",
				"externalAccountIdentifiers": {"obfuscatedExternalAccountId": "acct"},
				"testPurchase": {},
				"lineItems": [{"productId": "jp.kz2601.metamon.pro.monthly", "expiryTime": "2026-12-11T06:00:00.123Z",
				  "autoRenewingPlan": {"autoRenewEnabled": true}, "latestSuccessfulOrderId": "GPA.1"}]
			}`))
		case r.Method == "POST" && r.URL.Path == "/androidpublisher/v3/applications/jp.kz2601.metamon/purchases/subscriptions/jp.kz2601.metamon.pro.monthly/tokens/tok:acknowledge":
			acks = append(acks, "tok")
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/tokens/gone"):
			http.Error(w, `{"error":{"code":410,"message":"gone"}}`, http.StatusGone)
		default:
			http.Error(w, `{"error":{"code":500,"message":"boom"}}`, http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := NewWithOptions(context.Background(), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c, &acks
}

func TestClient(t *testing.T) {
	c, acks := fakeServer(t)
	ctx := context.Background()
	s, err := c.Subscription(ctx, "jp.kz2601.metamon", "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 12, 11, 6, 0, 0, 123e6, time.UTC)
	if s.State != StateActive || s.AcknowledgementState != AckPending || s.LinkedPurchaseToken != "old" ||
		s.ObfuscatedAccountID != "acct" || !s.Test || len(s.LineItems) != 1 {
		t.Fatalf("%+v", s)
	}
	if li := s.LineItems[0]; li.ProductID != "jp.kz2601.metamon.pro.monthly" || !li.ExpiryTime.Equal(want) || !li.AutoRenew || li.OrderID != "GPA.1" {
		t.Fatalf("%+v", li)
	}
	if err := c.Acknowledge(ctx, "jp.kz2601.metamon", "jp.kz2601.metamon.pro.monthly", "tok"); err != nil || len(*acks) != 1 {
		t.Fatalf("ack %v %v", err, *acks)
	}
	if _, err := c.Subscription(ctx, "jp.kz2601.metamon", "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("gone: %v", err)
	}
	_, err = c.Subscription(ctx, "jp.kz2601.metamon", "secret-token")
	if err == nil || errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("server error must be transient and never show the token: %v", err)
	}
}

func TestItemAndEntitles(t *testing.T) {
	now := time.Now()
	s := &Subscription{State: StateCanceled, LineItems: []LineItem{
		{ProductID: "other", ExpiryTime: now.Add(99 * time.Hour)},
		{ProductID: "pro", ExpiryTime: now.Add(time.Hour), OrderID: "a"},
		{ProductID: "pro", ExpiryTime: now.Add(2 * time.Hour), OrderID: "b"},
	}}
	if li, ok := s.Item([]string{"pro"}); !ok || li.OrderID != "b" {
		t.Fatalf("%+v %v", li, ok)
	}
	if _, ok := s.Item([]string{"none"}); ok {
		t.Fatal("no allowed item")
	}
	for state, want := range map[string]bool{StateActive: true, StateInGracePeriod: true, StateCanceled: true,
		StateOnHold: false, StatePaused: false, StateExpired: false, StatePending: false} {
		if (&Subscription{State: state}).Entitles() != want {
			t.Errorf("%s: want %v", state, want)
		}
	}
}

func TestCheckKey(t *testing.T) {
	if CheckKey([]byte(`{"type":"service_account"}`)) != nil {
		t.Fatal("service account key")
	}
	for _, k := range []string{"-----BEGIN PRIVATE KEY-----", `{"type":"authorized_user"}`} {
		err := CheckKey([]byte(k))
		if err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("%q: %v", k, err)
		}
	}
	if _, err := New(context.Background(), []byte(`{"type":"authorized_user"}`)); err == nil {
		t.Fatal("New must refuse a non service account key")
	}
}
