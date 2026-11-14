package fcm

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/option"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
)

// testKeyJSON is a service account key generated here (no real key is read).
func testKeyJSON(t *testing.T, project string) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": project, "private_key_id": "test",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "fcm-test@" + project + ".iam.gserviceaccount.com", "client_id": "1",
		"token_uri": "https://oauth2.googleapis.com/token",
	})
	return b
}

// fakeFCM is an HTTP fake of the FCM v1 send endpoint.
type fakeFCM struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
	// answer by token: status and FCM errorCode ("" = success)
	answers map[string][2]string
}

func (f *fakeFCM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	msg, _ := body["message"].(map[string]any)
	tok, _ := msg["token"].(string)
	a, ok := f.answers[tok]
	w.Header().Set("content-type", "application/json")
	if !ok {
		_, _ = w.Write([]byte(`{"name":"projects/p/messages/1"}`))
		return
	}
	status := map[string]int{"400": 400, "403": 403, "404": 404, "429": 429, "500": 500}[a[0]]
	statusName := map[int]string{400: "INVALID_ARGUMENT", 403: "PERMISSION_DENIED", 404: "NOT_FOUND", 429: "RESOURCE_EXHAUSTED", 500: "INTERNAL"}[status]
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": status, "message": "fake", "status": statusName,
		"details": []any{map[string]any{"@type": "type.googleapis.com/google.firebase.fcm.v1.FcmError", "errorCode": a[1]}},
	}})
}

func newTestClient(t *testing.T, f *fakeFCM) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(context.Background(), Config{CredentialsJSON: testKeyJSON(t, "demo-fcm"),
		Options: []option.ClientOption{option.WithEndpoint(srv.URL), option.WithHTTPClient(srv.Client())}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSend(t *testing.T) {
	f := &fakeFCM{}
	c := newTestClient(t, f)
	p := notify.Push{Token: "tok-ok", Platform: notify.PlatformAndroid, AppID: "app1", AppName: "部活",
		Title: "題", Body: "本文", CollapseID: notify.CollapseID("k", 0)}
	r, err := c.Send(context.Background(), p)
	if err != nil || r.Status != 200 {
		t.Fatalf("got %+v %v", r, err)
	}
	if len(f.paths) != 1 || f.paths[0] != "/projects/demo-fcm/messages:send" {
		t.Fatalf("paths %v", f.paths)
	}
	msg := f.bodies[0]["message"].(map[string]any)
	n := msg["notification"].(map[string]any)
	data := msg["data"].(map[string]any)
	android := msg["android"].(map[string]any)
	an := android["notification"].(map[string]any)
	if msg["token"] != "tok-ok" || n["title"] != "題" || n["body"] != "本文" ||
		data[DataAppID] != "app1" || data[DataAppName] != "部活" ||
		android["priority"] != "high" || an["tag"] != p.CollapseID {
		t.Errorf("message %v", msg)
	}
}

func TestSendErrors(t *testing.T) {
	f := &fakeFCM{answers: map[string][2]string{
		"gone":     {"404", "UNREGISTERED"},
		"other":    {"403", "SENDER_ID_MISMATCH"},
		"busy":     {"429", "QUOTA_EXCEEDED"},
		"broken":   {"500", "INTERNAL"},
		"bad":      {"400", "INVALID_ARGUMENT"},
		"unauthed": {"403", "THIRD_PARTY_AUTH_ERROR"},
	}}
	c := newTestClient(t, f)
	for _, tc := range []struct {
		token        string
		status       int
		unregistered bool
		retryable    bool
	}{
		{"gone", 404, true, false},
		{"other", 403, true, false},
		{"busy", 429, false, true},
		{"broken", 500, false, true},
		{"bad", 400, false, false},
		{"unauthed", 403, false, false},
	} {
		t.Run(tc.token, func(t *testing.T) {
			r, err := c.Send(context.Background(), notify.Push{Token: tc.token, Platform: notify.PlatformAndroid, Title: "t"})
			if !errors.Is(err, ErrRejected) {
				t.Fatalf("err %v", err)
			}
			if r.Status != tc.status || r.Unregistered != tc.unregistered || r.Retryable != tc.retryable {
				t.Errorf("got %+v", r)
			}
			if strings.Contains(err.Error(), "BEGIN PRIVATE KEY") {
				t.Error("key in error")
			}
		})
	}
	if _, err := c.Send(context.Background(), notify.Push{}); err == nil {
		t.Error("want error for an empty token")
	}
}

func TestSendNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens
	c, err := New(context.Background(), Config{CredentialsJSON: testKeyJSON(t, "demo-fcm"),
		Options: []option.ClientOption{option.WithEndpoint(url), option.WithHTTPClient(&http.Client{})}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // no SDK retry loop
	r, err := c.Send(ctx, notify.Push{Token: "t"})
	if err == nil || !r.Retryable {
		t.Errorf("got %+v %v", r, err)
	}
}

func TestNewValidates(t *testing.T) {
	for name, key := range map[string][]byte{
		"empty":      nil,
		"not JSON":   []byte("-----BEGIN PRIVATE KEY-----"),
		"not SA":     []byte(`{"type":"authorized_user","project_id":"p"}`),
		"no project": []byte(`{"type":"service_account"}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(context.Background(), Config{CredentialsJSON: key})
			if err == nil {
				t.Fatal("want error")
			}
			if strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Error("key material in error")
			}
		})
	}
	if p, err := ProjectOf(testKeyJSON(t, "demo-x")); err != nil || p != "demo-x" {
		t.Errorf("ProjectOf = %q %v", p, err)
	}
}
