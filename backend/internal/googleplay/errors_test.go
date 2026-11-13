package googleplay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	androidpublisher "google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestAPIErr(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusGone} {
		err := apiErr(fmt.Errorf("wrapped: %w", &googleapi.Error{Code: code}))
		if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), fmt.Sprint(code)) {
			t.Errorf("%d: %v", code, err)
		}
	}
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		err := apiErr(&googleapi.Error{Code: code, Message: "secret detail", Body: "body"})
		if errors.Is(err, ErrNotFound) || err.Error() != fmt.Sprintf("googleplay: HTTP %d", code) {
			t.Errorf("%d: %v", code, err)
		}
	}
	plain := errors.New("boom")
	if err := apiErr(plain); !errors.Is(err, plain) || errors.Is(err, ErrNotFound) {
		t.Errorf("plain: %v", err)
	}
}

// A transport error's URL carries the purchase token; the mapped error must not.
func TestAPIErrDropsTokenFromURL(t *testing.T) {
	token := strings.Repeat("t", 200)
	cause := errors.New("connection refused")
	err := apiErr(&url.Error{Op: "Get", URL: "https://example.invalid/tokens/" + token, Err: cause})
	if strings.Contains(err.Error(), token) || !errors.Is(err, cause) {
		t.Fatalf("%v", err)
	}
}

func TestClientErrorsCarryNoToken(t *testing.T) {
	token := strings.Repeat("p", 150)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":acknowledge"):
			http.Error(w, `{"error":{"code":404,"message":"not found `+token+`"}}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":{"code":401,"message":"bad `+token+`"}}`, http.StatusUnauthorized)
		}
	}))
	ctx := context.Background()
	c, err := NewWithOptions(ctx, option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Acknowledge(ctx, "jp.kz2601.metamon", "pro", token); !errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), token) {
		t.Fatalf("ack: %v", err)
	}
	if _, err := c.Subscription(ctx, "jp.kz2601.metamon", token); err == nil || errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), token) {
		t.Fatalf("401: %v", err)
	}
	srv.Close() // now a transport error
	if _, err := c.Subscription(ctx, "jp.kz2601.metamon", token); err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("closed: %v", err)
	}
}

func TestFromV2SkipsBadParts(t *testing.T) {
	s := fromV2(&androidpublisher.SubscriptionPurchaseV2{
		SubscriptionState: StatePending,
		LineItems: []*androidpublisher.SubscriptionPurchaseLineItem{
			nil,
			{ProductId: "p", ExpiryTime: "not a time"},
		},
	})
	if s.Test || s.ObfuscatedAccountID != "" || len(s.LineItems) != 1 {
		t.Fatalf("%+v", s)
	}
	if li := s.LineItems[0]; !li.ExpiryTime.IsZero() || li.AutoRenew {
		t.Fatalf("%+v", li)
	}
	if s.Entitles() {
		t.Fatal("pending must not entitle")
	}
}

func TestReject(t *testing.T) {
	err := Reject("product_id", "product %s is not allowed", "x")
	var e *Error
	if !errors.As(err, &e) || e.Reason != "product_id" {
		t.Fatalf("%#v", err)
	}
	if err.Error() != "googleplay: product_id: product x is not allowed" || errors.Unwrap(err) == nil {
		t.Fatalf("%v", err)
	}
	wrapped := Reject("not_found", "lookup: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatalf("unwrap: %v", wrapped)
	}
}

// New rejects a bad key before building anything, and the error never echoes it.
func TestNewRejectsBadKeys(t *testing.T) {
	secret := strings.Repeat("k", 64)
	for _, key := range []string{"", secret, `{"type":"authorized_user","private_key":"` + secret + `"}`, `{"type":1}`} {
		_, err := New(context.Background(), []byte(key))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("%.20q: %v", key, err)
		}
	}
}
