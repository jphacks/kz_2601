package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	fbauth "firebase.google.com/go/v4/auth"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify/apns"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// testKeyPEM is a key generated here (no real .p8 is read).
func testKeyPEM(t *testing.T, curve elliptic.Curve) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestPushFromEnv(t *testing.T) {
	key := testKeyPEM(t, elliptic.P256())
	full := func(extra map[string]string) map[string]string {
		m := map[string]string{"APNS_AUTH_KEY": key, "APNS_KEY_ID": "KEY123", "APNS_TEAM_ID": "TEAM456", "APNS_TOPIC": "jp.example.app"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name   string
		env    map[string]string
		ok     bool
		sender bool
		pushTo string
	}{
		{"no key: log only", map[string]string{}, true, false, store.DeviceProduction},
		{"no key, sandbox", map[string]string{"RUNNER_APNS_ENVIRONMENT": "sandbox"}, true, false, store.DeviceSandbox},
		{"key", full(nil), true, true, store.DeviceProduction},
		{"key, sandbox", full(map[string]string{"RUNNER_APNS_ENVIRONMENT": " Sandbox "}), true, true, store.DeviceSandbox},
		{"bad environment", map[string]string{"RUNNER_APNS_ENVIRONMENT": "development"}, false, false, ""},
		{"key without topic", full(map[string]string{"APNS_TOPIC": ""}), false, false, ""},
		{"key without key id", full(map[string]string{"APNS_KEY_ID": ""}), false, false, ""},
		{"not PEM", full(map[string]string{"APNS_AUTH_KEY": "not-a-key"}), false, false, ""},
		{"not P-256", full(map[string]string{"APNS_AUTH_KEY": testKeyPEM(t, elliptic.P384())}), false, false, ""},
	}
	for _, c := range cases {
		got, err := pushFromEnv(envMap(c.env))
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
			continue
		}
		if err != nil {
			if strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Errorf("%s: the error shows the key: %v", c.name, err)
			}
			continue
		}
		if (got.Sender != nil) != c.sender || got.Environment != c.pushTo {
			t.Errorf("%s: got sender=%v env=%q", c.name, got.Sender != nil, got.Environment)
		}
		if c.sender {
			if _, ok := got.Sender.(*apns.Client); !ok {
				t.Errorf("%s: sender %T", c.name, got.Sender)
			}
		}
	}
}

type fakeUsers struct {
	calls int
	users map[string]*fbauth.UserRecord
	err   error
}

func (f *fakeUsers) GetUser(_ context.Context, uid string) (*fbauth.UserRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.users[uid], nil
}

func TestFirebaseAge(t *testing.T) {
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	fu := &fakeUsers{users: map[string]*fbauth.UserRecord{
		"old":     {UserMetadata: &fbauth.UserMetadata{CreationTimestamp: created.UnixMilli()}},
		"no-meta": {},
	}}
	a := &firebaseAge{users: fu}
	for range 2 {
		at, known, err := a.CreatedAt(t.Context(), "old")
		if err != nil || !known || !at.Equal(created) {
			t.Fatalf("old: %v %v %v", at, known, err)
		}
	}
	if fu.calls != 1 {
		t.Errorf("creation time not cached: %d calls", fu.calls)
	}
	if _, known, err := a.CreatedAt(t.Context(), "no-meta"); known || err != nil {
		t.Errorf("no metadata: known=%v err=%v", known, err)
	}

	// an error other than user-not-found retries the run
	boom := errors.New("unavailable")
	a = &firebaseAge{users: &fakeUsers{err: boom}}
	if _, _, err := a.CreatedAt(t.Context(), "x"); !errors.Is(err, boom) {
		t.Errorf("error: %v", err)
	}
}

func TestAgeFromEnv(t *testing.T) {
	if a, err := ageFromEnv(t.Context(), envMap(nil), "demo-kz2601"); a != nil || err != nil {
		t.Errorf("unset: %v %v", a, err)
	}
	if _, err := ageFromEnv(t.Context(), envMap(map[string]string{"RUNNER_ACCOUNT_AGE": "assume-old"}), "demo-kz2601"); err == nil {
		t.Error("unknown mode accepted")
	}
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "127.0.0.1:9099") // no credentials needed
	a, err := ageFromEnv(t.Context(), envMap(map[string]string{"RUNNER_ACCOUNT_AGE": "firebase"}), "demo-kz2601")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(*firebaseAge); !ok {
		t.Errorf("age %T", a)
	}
}
