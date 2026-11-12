package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

// fakeStore lists bindings and answers Get with getErr.
type fakeStore struct {
	entitlement.NotifyStore
	bindings    []entitlement.Binding
	bindingsErr error
	getErr      error
}

func (s fakeStore) Bindings(context.Context, string, int) ([]entitlement.Binding, error) {
	return s.bindings, s.bindingsErr
}

func (s fakeStore) Get(context.Context, string) (*entitlement.Pro, error) { return nil, s.getErr }

// run's bool is the job's exit status: any failure must make it non-zero so
// the next scheduled run retries and the failure is visible.
func TestRunReportsFailures(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	bound := []entitlement.Binding{{OriginalTransactionID: "1", UID: "u"}}
	for _, tc := range []struct {
		name  string
		store fakeStore
		want  bool
	}{
		{"nothing bound", fakeStore{}, false},
		{"no Pro stored", fakeStore{bindings: bound}, false},
		{"binding without uid", fakeStore{bindings: []entitlement.Binding{{OriginalTransactionID: "1"}}, getErr: errors.New("unused")}, false},
		{"bindings error", fakeStore{bindingsErr: errors.New("firestore down")}, true},
		{"store error", fakeStore{bindings: bound, getErr: errors.New("boom")}, true},
	} {
		r := &entitlement.Reconciler{Store: tc.store, Log: log}
		for _, dry := range []bool{false, true} {
			if got := run(ctx, r, 10, dry, log); got != tc.want {
				t.Errorf("%s (dryRun=%v): failed=%v, want %v", tc.name, dry, got, tc.want)
			}
		}
	}
}
