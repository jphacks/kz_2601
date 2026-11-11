package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

// testPlayKey is a service account key generated here (no real key is read).
func testPlayKey(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": "demo-play",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "play@demo-play.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"})
	return string(b)
}

func TestGooglePlayFromEnv(t *testing.T) {
	ctx := context.Background()
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if gp, err := googlePlayFromEnv(ctx, env(nil)); gp != nil || err != nil {
		t.Fatalf("no key: %v %v", gp, err)
	}
	_, err := googlePlayFromEnv(ctx, env(map[string]string{"GOOGLE_PLAY_SERVICE_ACCOUNT_JSON": "-----BEGIN PRIVATE KEY-----x"}))
	if err == nil || strings.Contains(err.Error(), "BEGIN") {
		t.Fatalf("bad key: %v", err)
	}
	gp, err := googlePlayFromEnv(ctx, env(map[string]string{"GOOGLE_PLAY_SERVICE_ACCOUNT_JSON": testPlayKey(t), "GOOGLE_PLAY_PACKAGE_NAME": "jp.kz2601.metamon.stg"}))
	if err != nil || gp == nil || gp.API == nil || gp.Policy.PackageName != "jp.kz2601.metamon.stg" {
		t.Fatalf("key: %+v %v", gp, err)
	}
}

// Real-time developer notifications are mounted only when fully configured.
func TestPlayNotificationsMount(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	rtdn := map[string]string{"GOOGLE_PLAY_RTDN_AUDIENCE": "aud", "GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT": "push@demo.iam.gserviceaccount.com"}
	post := func(register func(*http.ServeMux)) int {
		mux := http.NewServeMux()
		register(mux)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", entitlement.PlayNotificationsPath, strings.NewReader(`{}`)))
		return w.Code
	}
	ents := &entitlement.Service{Store: entitlement.NewMemory()}
	if code := post(playNotifications(ents, env(rtdn), logger)); code != http.StatusNotFound {
		t.Fatalf("play not configured: %d", code)
	}
	ents.GooglePlay = &entitlement.GooglePlay{}
	if code := post(playNotifications(ents, env(nil), logger)); code != http.StatusNotFound {
		t.Fatalf("no RTDN env: %d", code)
	}
	if code := post(playNotifications(ents, env(rtdn), logger)); code != http.StatusUnauthorized {
		t.Fatalf("mounted (no token = 401): %d", code)
	}
}
