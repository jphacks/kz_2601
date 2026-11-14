package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

func TestModeAndSource(t *testing.T) {
	if mode(false) != "dry-run" || mode(true) != "APPLY" {
		t.Fatalf("mode: %q %q", mode(false), mode(true))
	}
	if source("") != "firebase auth" || source("users.json") != "export file" {
		t.Fatalf("source: %q %q", source(""), source("users.json"))
	}
}

func TestErrorCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"grpc unavailable", status.Error(codes.Unavailable, "users/u-secret: unavailable"), "Unavailable"},
		{"grpc aborted", status.Error(codes.Aborted, "x"), "Aborted"},
		{"plain error", errors.New("users/u-secret broke"), "unknown"},
		{"grpc unknown", status.Error(codes.Unknown, "users/u-secret"), "unknown"},
	}
	for _, c := range cases {
		if got := errorCode(c.err); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// loadUsers with -export reads the file and never reaches Firebase Auth.
func TestLoadUsersFromExport(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "users.json")
	if err := os.WriteFile(good, []byte(exportJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"users":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	us, err := loadUsers(t.Context(), "", good)
	if err != nil || len(us) != 4 {
		t.Fatalf("export: %d users, %v", len(us), err)
	}
	if _, err := loadUsers(t.Context(), "", filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing export file was accepted")
	}
	if _, err := loadUsers(t.Context(), "", bad); err == nil || !strings.Contains(err.Error(), "export file") {
		t.Fatalf("broken export file: %v", err)
	}
}

func TestParseExportEdges(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		ok       bool
		subjects int
	}{
		{"empty users", `{"users":[]}`, true, 0},
		{"no users key", `{}`, true, 0},
		{"null provider entry skipped", `{"users":[{"localId":"u","providerUserInfo":[null,{"providerId":"apple.com","rawId":"s1"}]}]}`, true, 1},
		{"not json", `users`, false, 0},
		{"users not a list", `{"users":{}}`, false, 0},
	}
	for _, c := range cases {
		us, err := parseExport(strings.NewReader(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
			continue
		}
		var got []string
		for _, u := range us {
			got = append(got, u.subjects()...)
		}
		if len(got) != c.subjects {
			t.Errorf("%s: subjects %v, want %d", c.name, got, c.subjects)
		}
	}
}

// failingStore fails every Update after the first ok calls.
type failingStore struct {
	coins.Store
	ok    int
	calls int
	err   error
}

func (f *failingStore) Update(ctx context.Context, k coins.Keys, fn func(*coins.Tx) error) error {
	f.calls++
	if f.calls > f.ok {
		return f.err
	}
	return f.Store.Update(ctx, k, fn)
}

// A failure stops the run, keeps the counts so far and prints the code only.
func TestRunStopsOnStoreError(t *testing.T) {
	us := []user{
		{UID: "u-1", Providers: map[string][]string{"apple.com": {"s-1"}}},
		{UID: "u-secret", Providers: map[string][]string{"apple.com": {"s-secret"}}},
		{UID: "u-3", Providers: map[string][]string{"apple.com": {"s-3"}}},
	}
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"grpc", status.Error(codes.DeadlineExceeded, "coin_wallets/u-secret: deadline"), "ERROR: DeadlineExceeded"},
		{"plain", errors.New("coin_wallets/u-secret: boom"), "ERROR: unknown"},
	}
	for _, c := range cases {
		st := &failingStore{Store: coins.NewMemory(), ok: 1, err: c.err}
		var out bytes.Buffer
		if code := run(context.Background(), st, us, true, time.Unix(0, 0), &out); code != 1 {
			t.Fatalf("%s: exit %d", c.name, code)
		}
		s := out.String()
		if !strings.Contains(s, "users read: 1 of 3") || !strings.Contains(s, c.code) {
			t.Errorf("%s: output:\n%s", c.name, s)
		}
		if st.calls != 2 {
			t.Errorf("%s: kept going after the failure: %d calls", c.name, st.calls)
		}
		for _, secret := range []string{"u-secret", "s-secret", "coin_wallets"} {
			if strings.Contains(s, secret) {
				t.Errorf("%s: output shows %q:\n%s", c.name, secret, s)
			}
		}
	}
}

func TestRunNoUsers(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), coins.NewMemory(), nil, false, time.Unix(0, 0), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "users read: 0 of 0") || !strings.Contains(out.String(), "claims would write: 0") {
		t.Fatalf("output:\n%s", out.String())
	}
}
