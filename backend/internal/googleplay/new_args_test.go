package googleplay

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"google.golang.org/api/option"
)

// throwawayServiceAccountKey is a service account key JSON whose RSA key is
// generated for this run only (nothing is stored in the source).
func throwawayServiceAccountKey(t *testing.T) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "demo-kz2601",
		"client_email": "play@demo-kz2601.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// #2566: New refuses a missing / malformed / non service account key with
// CheckKey's message, refuses a service account key without its private key
// at init, and builds a Client (without calling Google) from a full key.
func TestNewKeyArguments(t *testing.T) {
	okKey := throwawayServiceAccountKey(t)
	noPrivateKey := `{"type":"service_account","project_id":"demo-kz2601","client_email":"play@demo-kz2601.iam.gserviceaccount.com"}`
	cases := []struct {
		name    string
		key     []byte
		opts    []option.ClientOption
		wantErr string
	}{
		{"nil key", nil, nil, "the key is not JSON"},
		{"empty key", []byte{}, nil, "the key is not JSON"},
		{"not JSON", []byte("not json"), nil, "the key is not JSON"},
		{"truncated JSON", []byte(`{"type":"service_account"`), nil, "the key is not JSON"},
		{"JSON array", []byte(`[]`), nil, "the key is not JSON"},
		{"no type", []byte(`{}`), nil, "not a service account key"},
		{"type not a string", []byte(`{"type":1}`), nil, "the key is not JSON"},
		{"user credentials", []byte(`{"type":"authorized_user"}`), nil, "not a service account key"},
		{"service account without private key", []byte(noPrivateKey), nil, "init"},
		{"service account", okKey, nil, ""},
		{"service account with endpoint", okKey, []option.ClientOption{option.WithEndpoint("http://127.0.0.1:1/")}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := New(t.Context(), c.key, c.opts...)
			if c.wantErr == "" {
				if err != nil || got == nil || got.svc == nil {
					t.Fatalf("New = %+v %v, want a client", got, err)
				}
				return
			}
			if err == nil || got != nil {
				t.Fatalf("New = %+v %v, want error %q", got, err, c.wantErr)
			}
			if !strings.HasPrefix(err.Error(), "googleplay: ") || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %q, want %q", err, c.wantErr)
			}
			if strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Fatalf("err echoes key material: %q", err)
			}
		})
	}
}
