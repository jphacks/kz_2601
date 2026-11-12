package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testProject = "demo-auth-test"

// fakeAuthEmulator answers the Admin SDK's accounts:lookup (the revocation
// check it makes in emulator mode). Unknown uids are "not found"; disabled
// and validSince are per uid. No network beyond the loopback test server.
func fakeAuthEmulator(t *testing.T, disabled map[string]bool, validSince map[string]int64) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/accounts:lookup") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			LocalID []string `json:"localId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		users := []map[string]any{}
		for _, uid := range req.LocalID {
			if uid == "missing" {
				continue
			}
			users = append(users, map[string]any{
				"localId":    uid,
				"disabled":   disabled[uid],
				"validSince": strconv.FormatInt(validSince[uid], 10),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", strings.TrimPrefix(srv.URL, "http://"))
}

// emulatorToken is an unsigned ID token as the Auth emulator issues them.
func emulatorToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	now := time.Now().Unix()
	base := map[string]any{
		"aud": testProject, "iss": "https://securetoken.google.com/" + testProject,
		"sub": "u1", "iat": now - 10, "exp": now + 3600, "auth_time": now - 10,
		"firebase": map[string]any{"sign_in_provider": "password"},
	}
	for k, v := range claims {
		if v == nil {
			delete(base, k)
			continue
		}
		base[k] = v
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(map[string]string{"alg": "none", "typ": "JWT"}) + "." + enc(base) + "."
}

func TestFirebaseVerifierIdentity(t *testing.T) {
	now := time.Now().Unix()
	fakeAuthEmulator(t, map[string]bool{"off": true}, map[string]int64{"revoked": now + 60})

	v := VerifierFromEnv(context.Background(), ModeFirebase, testProject)
	fv, ok := v.(*FirebaseVerifier)
	if !ok {
		t.Fatalf("VerifierFromEnv = %T, want *FirebaseVerifier", v)
	}

	tests := []struct {
		name    string
		claims  map[string]any
		want    Identity
		wantErr bool
	}{
		{
			name:   "password account",
			claims: map[string]any{},
			want:   Identity{UID: "u1"},
		},
		{
			name:   "anonymous account",
			claims: map[string]any{"sub": "anon1", "firebase": map[string]any{"sign_in_provider": "anonymous"}},
			want:   Identity{UID: "anon1", Anonymous: true},
		},
		{
			// only Apple / Google stand for a person; email is dropped, dups compacted
			name: "apple and google subjects",
			claims: map[string]any{"sub": "u2", "firebase": map[string]any{
				"sign_in_provider": "apple.com",
				"identities": map[string]any{
					"apple.com":  []any{"a-sub", "a-sub"},
					"google.com": []any{"g-sub"},
					"email":      []any{"x@example.com"},
				},
			}},
			want: Identity{UID: "u2", Subjects: []string{"apple.com:a-sub", "google.com:g-sub"}},
		},
		{name: "other project", claims: map[string]any{"aud": "someone-else"}, wantErr: true},
		{name: "other issuer", claims: map[string]any{"iss": "https://evil.example/" + testProject}, wantErr: true},
		{name: "expired", claims: map[string]any{"iat": now - 7200, "exp": now - 3600}, wantErr: true},
		{name: "issued in the future", claims: map[string]any{"iat": now + 3600}, wantErr: true},
		{name: "no subject", claims: map[string]any{"sub": ""}, wantErr: true},
		{name: "subject over 128", claims: map[string]any{"sub": strings.Repeat("a", 129)}, wantErr: true},
		{name: "disabled user", claims: map[string]any{"sub": "off"}, wantErr: true},
		{name: "revoked token", claims: map[string]any{"sub": "revoked"}, wantErr: true},
		{name: "deleted user", claims: map[string]any{"sub": "missing"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := emulatorToken(t, tt.claims)
			id, err := fv.VerifyIdentity(context.Background(), tok)
			uid, uidErr := fv.VerifyIDToken(context.Background(), tok)
			if tt.wantErr {
				if err == nil || uidErr == nil {
					t.Fatalf("accepted: id=%+v uid=%q", id, uid)
				}
				return
			}
			if err != nil || uidErr != nil {
				t.Fatalf("err = %v / %v", err, uidErr)
			}
			if id.UID != tt.want.UID || id.Anonymous != tt.want.Anonymous || !slices.Equal(id.Subjects, tt.want.Subjects) {
				t.Fatalf("identity = %+v, want %+v", id, tt.want)
			}
			if uid != tt.want.UID {
				t.Fatalf("uid = %q, want %q", uid, tt.want.UID)
			}
		})
	}

	for _, bad := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := fv.VerifyIdentity(context.Background(), bad); err == nil {
			t.Errorf("malformed token %q accepted", bad)
		}
	}
}

func TestVerifyIdentityDevAndNotConfigured(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		token   string
		want    Identity
		wantErr bool
	}{
		{token: "dev-smoke", want: Identity{UID: "dev-smoke", Subjects: []string{"dev:dev-smoke"}}},
		{token: "anon-1", want: Identity{UID: "anon-1", Anonymous: true}},
		{token: "has space", wantErr: true},
		{token: "", wantErr: true},
		{token: strings.Repeat("a", 129), wantErr: true},
	}
	for _, tt := range tests {
		id, err := DevVerifier{}.VerifyIdentity(ctx, tt.token)
		if (err != nil) != tt.wantErr {
			t.Errorf("dev %q: err = %v", tt.token, err)
			continue
		}
		if !tt.wantErr && (id.UID != tt.want.UID || id.Anonymous != tt.want.Anonymous || !slices.Equal(id.Subjects, tt.want.Subjects)) {
			t.Errorf("dev %q: identity = %+v, want %+v", tt.token, id, tt.want)
		}
	}
	if id, err := (NotConfigured{}).VerifyIdentity(ctx, "anything"); !errors.Is(err, ErrNotConfigured) || id.UID != "" {
		t.Errorf("NotConfigured = %+v, %v", id, err)
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER  abc  ", "abc", true},
		{"Bearer ", "", false},
		{"Bearer    ", "", false},
		{"Bearer", "", false},
		{"Basic abc", "", false},
		{"", "", false},
		{"Bearerabc", "", false},
	}
	for _, tt := range tests {
		got, ok := BearerToken(tt.header)
		if got != tt.want || ok != tt.ok {
			t.Errorf("BearerToken(%q) = %q, %v; want %q, %v", tt.header, got, ok, tt.want, tt.ok)
		}
	}
}
