package appstore_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

func TestServerAPISubscriptionStatuses(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	parsed, err := appstore.ParsePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := checkJWT(t, r.Header.Get("Authorization"), &key.PublicKey)
		if claims["iss"] != "issuer" || claims["bid"] != "b" || claims["aud"] != "appstoreconnect-v1" {
			t.Errorf("claims %v", claims)
		}
		switch r.URL.Path {
		case "/inApps/v1/subscriptions/tx-1":
			_, _ = w.Write([]byte(`{"environment":"Sandbox","data":[{"subscriptionGroupIdentifier":"g","lastTransactions":[{"originalTransactionId":"o","status":1,"signedTransactionInfo":"t","signedRenewalInfo":"r"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	api := &appstore.ServerAPI{KeyID: "kid", IssuerID: "issuer", BundleID: "b", Key: parsed,
		BaseURL: map[string]string{appstore.EnvSandbox: srv.URL}, Now: func() time.Time { return now }}

	got, err := api.SubscriptionStatuses(context.Background(), appstore.EnvSandbox, "tx-1")
	if err != nil || len(got) != 1 || got[0].OriginalTransactionID != "o" || got[0].SignedTransactionInfo != "t" || got[0].SignedRenewalInfo != "r" {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := api.SubscriptionStatuses(context.Background(), appstore.EnvSandbox, "missing"); !errors.Is(err, appstore.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := api.SubscriptionStatuses(context.Background(), "Xcode", "tx-1"); err == nil {
		t.Fatal("unknown environment accepted")
	}
	if _, err := appstore.ParsePrivateKey([]byte("not a key")); err == nil {
		t.Fatal("bad key accepted")
	}
}

// checkJWT verifies the ES256 bearer token and returns its claims.
func checkJWT(t *testing.T, auth string, pub *ecdsa.PublicKey) map[string]any {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", auth)
	}
	var h map[string]string
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &h)
	if h["alg"] != "ES256" || h["kid"] != "kid" || h["typ"] != "JWT" {
		t.Errorf("header %v", h)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("bad signature")
	}
	var c map[string]any
	pb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(pb, &c)
	return c
}
