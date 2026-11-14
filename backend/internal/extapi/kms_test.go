package extapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/option"
)

const kms012Key = "projects/p1/locations/global/keyRings/r1/cryptoKeys/k1"

// kms012Fake is a Cloud KMS stand-in: the ciphertext is "ct:" + aad + ":" +
// plaintext, so Decrypt can check the aad it was given.
type kms012Fake struct {
	mu    sync.Mutex
	paths []string
	aads  []string
	fail  bool
}

func (f *kms012Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	fail := f.fail
	f.mu.Unlock()
	if fail {
		http.Error(w, `{"error":{"code":403,"message":"denied"}}`, http.StatusForbidden)
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	enc := base64.StdEncoding
	aad, _ := enc.DecodeString(body["additionalAuthenticatedData"])
	f.mu.Lock()
	f.aads = append(f.aads, string(aad))
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, ":encrypt"):
		pt, _ := enc.DecodeString(body["plaintext"])
		ct := "ct:" + string(aad) + ":" + string(pt)
		_ = json.NewEncoder(w).Encode(map[string]string{"ciphertext": enc.EncodeToString([]byte(ct))})
	case strings.HasSuffix(r.URL.Path, ":decrypt"):
		ct, _ := enc.DecodeString(body["ciphertext"])
		prefix := "ct:" + string(aad) + ":"
		if !strings.HasPrefix(string(ct), prefix) {
			http.Error(w, `{"error":{"code":400,"message":"aad mismatch"}}`, http.StatusBadRequest)
			return
		}
		pt := strings.TrimPrefix(string(ct), prefix)
		_ = json.NewEncoder(w).Encode(map[string]string{"plaintext": enc.EncodeToString([]byte(pt))})
	default:
		http.NotFound(w, r)
	}
}

func newKMS012Sealer(t *testing.T, f *kms012Fake) *KMSSealer {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := NewKMSSealer(context.Background(), kms012Key,
		option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("NewKMSSealer: %v", err)
	}
	return s
}

func TestKMSSealerRoundTrip(t *testing.T) {
	f := &kms012Fake{}
	s := newKMS012Sealer(t, f)
	ctx := context.Background()
	aad := []byte("uid-1/github")
	sealed, err := s.Seal(ctx, []byte("fake-token"), aad)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(sealed) == "fake-token" {
		t.Fatal("Seal returned the plaintext")
	}
	got, err := s.Open(ctx, sealed, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != "fake-token" {
		t.Fatalf("Open = %q, want fake-token", got)
	}
	if _, err := s.Open(ctx, sealed, []byte("other")); err == nil {
		t.Fatal("Open with another aad must fail")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.paths {
		if !strings.Contains(p, kms012Key+":") {
			t.Errorf("path %q lacks the key name", p)
		}
	}
	if len(f.aads) < 2 || f.aads[0] != "uid-1/github" || f.aads[1] != "uid-1/github" {
		t.Errorf("aads = %q, want the aad on encrypt and decrypt", f.aads)
	}
}

func TestKMSSealer012WrapsErrors(t *testing.T) {
	f := &kms012Fake{fail: true}
	s := newKMS012Sealer(t, f)
	ctx := context.Background()
	if _, err := s.Seal(ctx, []byte("x"), nil); err == nil || !strings.Contains(err.Error(), "kms encrypt") {
		t.Errorf("Seal err = %v, want kms encrypt", err)
	}
	if _, err := s.Open(ctx, []byte("x"), nil); err == nil || !strings.Contains(err.Error(), "kms decrypt") {
		t.Errorf("Open err = %v, want kms decrypt", err)
	}
	if _, err := s.Open(ctx, nil, nil); err == nil || !strings.Contains(err.Error(), "empty sealed value") {
		t.Errorf("Open(empty) err = %v", err)
	}
}

func TestNewKMSSealer012RejectsBadKey(t *testing.T) {
	for _, k := range []string{"", "projects/p/locations/l/keyRings/r", kms012Key + "/cryptoKeyVersions/1"} {
		if _, err := NewKMSSealer(context.Background(), k); err == nil {
			t.Errorf("NewKMSSealer(%q) must fail", k)
		}
	}
}
