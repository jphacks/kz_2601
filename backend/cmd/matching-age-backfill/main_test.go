package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestGuard(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		allowProd bool
		ok        bool
	}{
		{"no project", map[string]string{}, true, false},
		{"prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, false, false},
		{"prod db with flag", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject}, true, true},
		{"emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": fsenv.ProdProject, "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, false, true},
	}
	for _, c := range cases {
		getenv := envOf(c.env)
		err := guard(fsenv.FromEnv(getenv), c.allowProd, getenv)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

type fakeStore struct {
	dryRun []bool
	err    error
}

func (f *fakeStore) BackfillProfileAgeFlags(_ context.Context, dryRun bool) (store.ProfileAgeBackfill, error) {
	f.dryRun = append(f.dryRun, dryRun)
	return store.ProfileAgeBackfill{Apps: 2, Profiles: 5, Marked: 1, ToMark: 3, Undeclared: 1}, f.err
}

func TestRunDryRunByDefault(t *testing.T) {
	f := &fakeStore{}
	var out bytes.Buffer
	if code := run(context.Background(), f, false, &out); code != 0 {
		t.Fatalf("code %d", code)
	}
	if len(f.dryRun) != 1 || !f.dryRun[0] {
		t.Fatalf("without -apply it must be a dry run: %v", f.dryRun)
	}
	if !strings.Contains(out.String(), "would set flag: 3") || !strings.Contains(out.String(), "left hidden): 1") {
		t.Fatalf("output: %s", out.String())
	}
	out.Reset()
	if code := run(context.Background(), f, true, &out); code != 0 || f.dryRun[1] || !strings.Contains(out.String(), "set flag: 3") {
		t.Fatalf("apply: %d %v %s", code, f.dryRun, out.String())
	}
	f.err = errors.New("boom")
	if code := run(context.Background(), f, true, &out); code != 1 {
		t.Fatalf("error exit: %d", code)
	}
}

func TestRunErrorPrintsCodeNotText(t *testing.T) {
	const path = "projects/p/databases/(default)/documents/apps/app-secret-1/profiles/uid-secret-2"
	f := &fakeStore{err: status.Error(codes.PermissionDenied, "denied: "+path)}
	var out bytes.Buffer
	if code := run(context.Background(), f, true, &out); code != 1 {
		t.Fatalf("code %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "ERROR: PermissionDenied") || !strings.Contains(got, "set flag: 3") {
		t.Fatalf("want code and counts: %s", got)
	}
	for _, leak := range []string{path, "app-secret-1", "uid-secret-2", "denied:"} {
		if strings.Contains(got, leak) {
			t.Fatalf("raw error text %q leaked: %s", leak, got)
		}
	}

	out.Reset()
	f.err = errors.New("plain " + path)
	run(context.Background(), f, true, &out)
	if got := out.String(); !strings.Contains(got, "ERROR: unknown") || strings.Contains(got, "app-secret-1") {
		t.Fatalf("non-grpc error: %s", got)
	}
}
