package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestGuard(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		allowProd  bool
		fromExport bool
		ok         bool
	}{
		{"no project", map[string]string{}, true, true, false},
		{"prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, false, true, false},
		{"prod db with flag", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, true, false, true},
		{"emulator with export", map[string]string{"GOOGLE_CLOUD_PROJECT": "demo-kz2601", "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, false, true, true},
		{"emulator with the real auth", map[string]string{"GOOGLE_CLOUD_PROJECT": "demo-kz2601", "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, false, false, false},
		{"emulator with the auth emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": "demo-kz2601", "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089", "FIREBASE_AUTH_EMULATOR_HOST": "127.0.0.1:9099"}, false, false, true},
	}
	for _, c := range cases {
		getenv := envOf(c.env)
		err := guard(fsenv.FromEnv(getenv), c.allowProd, c.fromExport, getenv)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

const exportJSON = `{"users":[
 {"localId":"u-apple","email":"a@example.com","providerUserInfo":[{"providerId":"apple.com","rawId":"001.sub","email":"a@example.com"}]},
 {"localId":"u-both","providerUserInfo":[{"providerId":"google.com","rawId":"g-1"},{"providerId":"apple.com","rawId":"001.two"}]},
 {"localId":"u-mail","providerUserInfo":[{"providerId":"password","rawId":"m@example.com"}]},
 {"localId":"u-anon"}
]}`

func TestParseExport(t *testing.T) {
	us, err := parseExport(strings.NewReader(exportJSON))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range us {
		got = append(got, u.UID+"="+strings.Join(u.subjects(), "+"))
	}
	want := []string{"u-apple=apple.com:001.sub", "u-both=apple.com:001.two+google.com:g-1", "u-mail=", "u-anon="}
	if !slices.Equal(got, want) {
		t.Fatalf("users %v, want %v", got, want)
	}
	if _, err := parseExport(strings.NewReader(`{"users":[{"providerUserInfo":[]}]}`)); err == nil {
		t.Fatal("a user without localId")
	}
}

// Dry run by default, the same key as the grant, counts only in the output.
func TestRun(t *testing.T) {
	ctx, now := context.Background(), time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	st := coins.NewMemory()
	us, err := parseExport(strings.NewReader(exportJSON))
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{"u-apple", "u-mail"} { // granted before the claims existed
		if err := st.Update(ctx, coins.Keys{UID: uid}, func(tx *coins.Tx) error {
			tx.Wallet.Initial, tx.Wallet.InitialGranted, tx.WalletDirty = 200, true, true
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if code := run(ctx, st, us, false, now, &out); code != 0 {
		t.Fatalf("dry run: %d %s", code, out.String())
	}
	for _, line := range []string{"users read: 4 of 4", "no Apple / Google sign-in: 2", "initial grant not given: 1", "accounts to record: 1", "claims would write: 1"} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("dry run output lacks %q:\n%s", line, out.String())
		}
	}
	for _, secret := range []string{"001.sub", "g-1", "example.com", "u-apple"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("output shows %q:\n%s", secret, out.String())
		}
	}
	out.Reset()
	if code := run(ctx, st, us, true, now, &out); code != 0 || !strings.Contains(out.String(), "claims wrote: 1") {
		t.Fatalf("apply: %d %s", code, out.String())
	}
	out.Reset()
	if code := run(ctx, st, us, true, now, &out); code != 0 || !strings.Contains(out.String(), "already recorded: 1") || !strings.Contains(out.String(), "claims wrote: 0") {
		t.Fatalf("second run: %d %s", code, out.String())
	}

	// a new account of the same Apple ID now gets no initial grant
	s := &coins.Service{Config: coins.Config{Mode: coins.ModeEnforce, InitialGrant: 200}, Store: st, Now: func() time.Time { return now },
		Subjects: func(context.Context, string) []string { return []string{"apple.com:001.sub"} }}
	if v, err := s.Wallet(ctx, "u-new", false); err != nil || v.Initial != 0 {
		t.Fatalf("new account of the same person: %+v %v", v, err)
	}
}
