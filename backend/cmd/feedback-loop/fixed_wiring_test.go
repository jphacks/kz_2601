package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop/fixed"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type fixed017Devices struct{}

func (fixed017Devices) ListDevices(context.Context, string) ([]store.Device, error) { return nil, nil }

type fixed017Consent struct{}

func (fixed017Consent) PushAllowed(context.Context, string) (bool, error) { return false, nil }

type fixed017Pusher struct{}

func (fixed017Pusher) Push(context.Context, string, string, string) (bool, error) { return false, nil }

func fixed017Env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// fixed017KeyPEM makes a throwaway P-256 key at test time (nothing real).
func fixed017KeyPEM(t *testing.T) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestFixedIssues(t *testing.T) {
	stub := fakeIssues{}
	a := &app{issues: stub, getenv: fixed017Env(nil)}
	got, err := a.fixedIssues()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(fakeIssues); !ok {
		t.Fatalf("set issues not preferred: %T", got)
	}

	a = &app{getenv: fixed017Env(nil)}
	if _, err := a.fixedIssues(); err == nil || !strings.Contains(err.Error(), envGitHubToken+" is not set") {
		t.Fatalf("no token: err = %v", err)
	}

	a = &app{getenv: fixed017Env(map[string]string{envGitHubToken: "fake-token-017"})}
	got, err = a.fixedIssues()
	if err != nil {
		t.Fatal(err)
	}
	c, ok := got.(*github.Client)
	if !ok || c.Token != "fake-token-017" {
		t.Fatalf("with token: %T %+v", got, got)
	}
}

func TestFixedPusher(t *testing.T) {
	key := fixed017KeyPEM(t)
	full := func(over map[string]string) map[string]string {
		m := map[string]string{envNotifyPush: "on", "APNS_AUTH_KEY": key, "APNS_KEY_ID": "KEYID017",
			"APNS_TEAM_ID": "TEAM017", "APNS_TOPIC": "jp.example.app017"}
		for k, v := range over {
			m[k] = v
		}
		return m
	}

	t.Run("off", func(t *testing.T) {
		for _, v := range []string{"", "off", "ON", "true"} {
			a := &app{pusher: fixed017Pusher{}, consent: fixed017Consent{}, getenv: fixed017Env(map[string]string{envNotifyPush: v})}
			p, c, err := a.fixedPusher()
			if p != nil || c != nil || err != nil {
				t.Errorf("%s=%q: %v %v %v, want all nil", envNotifyPush, v, p, c, err)
			}
		}
	})
	t.Run("set pusher preferred", func(t *testing.T) {
		a := &app{pusher: fixed017Pusher{}, consent: fixed017Consent{}, getenv: fixed017Env(map[string]string{envNotifyPush: "on"})}
		p, c, err := a.fixedPusher()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := p.(fixed017Pusher); !ok {
			t.Fatalf("pusher = %T", p)
		}
		if _, ok := c.(fixed017Consent); !ok {
			t.Fatalf("consent = %T", c)
		}
	})
	t.Run("no stores", func(t *testing.T) {
		a := &app{getenv: fixed017Env(full(nil))}
		if _, _, err := a.fixedPusher(); err == nil || !strings.Contains(err.Error(), "device and consent stores") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing settings", func(t *testing.T) {
		for _, k := range []string{"APNS_AUTH_KEY", "APNS_KEY_ID", "APNS_TEAM_ID", "APNS_TOPIC"} {
			a := &app{devices: fixed017Devices{}, consent: fixed017Consent{}, getenv: fixed017Env(full(map[string]string{k: ""}))}
			if _, _, err := a.fixedPusher(); err == nil || !strings.Contains(err.Error(), "needs APNS_AUTH_KEY") {
				t.Errorf("without %s: err = %v", k, err)
			}
		}
	})
	t.Run("bad key", func(t *testing.T) {
		a := &app{devices: fixed017Devices{}, consent: fixed017Consent{}, getenv: fixed017Env(full(map[string]string{"APNS_AUTH_KEY": "not-a-pem"}))}
		if _, _, err := a.fixedPusher(); err == nil || !strings.Contains(err.Error(), "APNS_AUTH_KEY:") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad environment", func(t *testing.T) {
		a := &app{devices: fixed017Devices{}, consent: fixed017Consent{}, getenv: fixed017Env(full(map[string]string{"LOOP_APNS_ENVIRONMENT": "staging"}))}
		if _, _, err := a.fixedPusher(); err == nil || !strings.Contains(err.Error(), "production or sandbox") {
			t.Fatalf("err = %v", err)
		}
	})
	for _, tc := range []struct{ env, want string }{
		{"", store.DeviceProduction},
		{" Production ", store.DeviceProduction},
		{"sandbox", store.DeviceSandbox},
	} {
		t.Run("built "+strings.TrimSpace(tc.env), func(t *testing.T) {
			a := &app{devices: fixed017Devices{}, consent: fixed017Consent{}, getenv: fixed017Env(full(map[string]string{"LOOP_APNS_ENVIRONMENT": tc.env}))}
			p, c, err := a.fixedPusher()
			if err != nil {
				t.Fatal(err)
			}
			ap, ok := p.(fixed.APNsPusher)
			if !ok || ap.Environment != tc.want || ap.Sender == nil || ap.Devices == nil {
				t.Fatalf("pusher = %T %+v, want APNsPusher env %s", p, p, tc.want)
			}
			if _, ok := c.(fixed017Consent); !ok {
				t.Fatalf("consent = %T", c)
			}
		})
	}
}
