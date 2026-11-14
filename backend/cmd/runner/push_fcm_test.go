package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify/fcm"
)

// testSAJSON is a service account key generated here (no real key is read).
func testSAJSON(t *testing.T, project string) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{"type": "service_account", "project_id": project,
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "fcm@" + project + ".iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"})
	return string(b)
}

// #918: no key = no FCM sender (the runner still starts); a key builds one.
func TestFCMFromEnv(t *testing.T) {
	ctx := context.Background()
	key := testSAJSON(t, "demo-key-project")
	cases := []struct {
		name    string
		env     map[string]string
		ok      bool
		sender  bool
		project string
	}{
		{"no key: skipped", map[string]string{}, true, false, ""},
		{"key", map[string]string{"FCM_SERVICE_ACCOUNT_JSON": key}, true, true, "demo-key-project"},
		{"key, project", map[string]string{"FCM_SERVICE_ACCOUNT_JSON": key, "FCM_PROJECT_ID": "demo-other"}, true, true, "demo-other"},
		{"not JSON", map[string]string{"FCM_SERVICE_ACCOUNT_JSON": "-----BEGIN PRIVATE KEY-----"}, false, false, ""},
		{"not a service account", map[string]string{"FCM_SERVICE_ACCOUNT_JSON": `{"type":"authorized_user"}`}, false, false, ""},
	}
	for _, c := range cases {
		s, project, err := fcmFromEnv(ctx, envMap(c.env))
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
		if (s != nil) != c.sender || project != c.project {
			t.Errorf("%s: sender=%v project=%q", c.name, s != nil, project)
		}
		if c.sender {
			if _, ok := s.(*fcm.Client); !ok {
				t.Errorf("%s: sender %T", c.name, s)
			}
		}
	}
}

func TestPushSender(t *testing.T) {
	if pushSender(nil, nil, nil) != nil {
		t.Error("no keys: want nil (log only)")
	}
	ios, android := &notify.FakeSender{}, &notify.FakeSender{}
	s, ok := pushSender(ios, nil, nil).(*notify.PlatformSender)
	if !ok || s.IOS != notify.Sender(ios) || s.Android != nil {
		t.Errorf("apns only: %+v", s)
	}
	s, ok = pushSender(nil, android, nil).(*notify.PlatformSender)
	if !ok || s.IOS != nil || s.Android != notify.Sender(android) {
		t.Errorf("fcm only: %+v", s)
	}
}
