package runner

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

type deleteCall struct {
	env, id    string
	maxVersion int
}

// dropStore records DeleteSchedule; any other Store method panics (nil embed).
type dropStore struct {
	Store
	calls []deleteCall
	// fail: ids whose delete returns an error; gone: ids already deleted.
	fail, gone map[string]bool
}

func (s *dropStore) DeleteSchedule(_ context.Context, env, id string, maxVersion int) (bool, error) {
	s.calls = append(s.calls, deleteCall{env, id, maxVersion})
	if s.fail[id] {
		return false, errors.New("boom")
	}
	return !s.gone[id], nil
}

func dropRunner(st *dropStore) (*Runner, *bytes.Buffer) {
	var buf bytes.Buffer
	return &Runner{cfg: Config{Env: "e", Store: st, Logger: slog.New(slog.NewJSONHandler(&buf, nil))}}, &buf
}

func TestDropSchedule(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		uid   string // the uid ScheduleKey gets
	}{
		{"private keeps uid", string(dsl.ScopePrivate), "u1"},
		{"no scope keeps uid", "", "u1"},
		{"shared drops uid", "shared", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &dropStore{}
			r, logs := dropRunner(st)
			r.dropSchedule(t.Context(), store.OutboxEvent{AppID: "A", UID: "u1", Scope: c.scope, Version: 3, RuleIDs: []string{"r1", "r2"}})
			want := []deleteCall{
				{"e", store.ScheduleKey("A", c.uid, "r1"), 3},
				{"e", store.ScheduleKey("A", c.uid, "r2"), 3},
			}
			if len(st.calls) != len(want) || st.calls[0] != want[0] || st.calls[1] != want[1] {
				t.Fatalf("calls = %+v, want %+v", st.calls, want)
			}
			if logs.Len() != 0 {
				t.Fatalf("unexpected log: %s", logs)
			}
		})
	}
}

// A schedule already gone (deleted=false) is not an error and logs nothing.
func TestDropScheduleAlreadyGone(t *testing.T) {
	id := store.ScheduleKey("A", "u1", "r1")
	st := &dropStore{gone: map[string]bool{id: true}}
	r, logs := dropRunner(st)
	r.dropSchedule(t.Context(), store.OutboxEvent{AppID: "A", UID: "u1", Version: 1, RuleIDs: []string{"r1"}})
	if len(st.calls) != 1 || logs.Len() != 0 {
		t.Fatalf("calls = %+v, logs = %s", st.calls, logs)
	}
}

// A store error is logged and the other rules are still deleted.
func TestDropScheduleStoreError(t *testing.T) {
	bad := store.ScheduleKey("A", "u1", "r1")
	st := &dropStore{fail: map[string]bool{bad: true}}
	r, logs := dropRunner(st)
	r.dropSchedule(t.Context(), store.OutboxEvent{AppID: "A", UID: "u1", Version: 2, RuleIDs: []string{"r1", "r2"}})
	if len(st.calls) != 2 || st.calls[1].id != store.ScheduleKey("A", "u1", "r2") {
		t.Fatalf("calls = %+v", st.calls)
	}
	out := logs.String()
	if strings.Count(out, "deleting the schedule of a deleted app failed") != 1 || !strings.Contains(out, `"app_id":"A"`) || !strings.Contains(out, "boom") {
		t.Fatalf("logs: %s", out)
	}
}

// No rule ids: nothing to delete.
func TestDropScheduleNoRules(t *testing.T) {
	st := &dropStore{}
	r, _ := dropRunner(st)
	r.dropSchedule(t.Context(), store.OutboxEvent{AppID: "A", UID: "u1"})
	if len(st.calls) != 0 {
		t.Fatalf("calls = %+v", st.calls)
	}
}

func TestAllowanceReason(t *testing.T) {
	cases := map[string]string{
		quota.OutcomeSkippedOverflowCap: ReasonOverflowCap,
		quota.OutcomeSkippedCoins:       ReasonCoins,
		quota.OutcomeSkippedAllowance:   ReasonAllowance,
		"":                              ReasonAllowance,
		"something_new":                 ReasonAllowance,
	}
	for stop, want := range cases {
		if got := allowanceReason(stop); got != want {
			t.Errorf("allowanceReason(%q) = %q, want %q", stop, got, want)
		}
	}
}

func TestAllowanceKindName(t *testing.T) {
	cases := map[string]string{
		coins.KindRecipients: "通知",
		coins.KindWrites:     "書き込み",
		coins.KindReads:      "読み取り",
		coins.KindRuns:       "自動の動き",
		"":                   "自動の動き",
		"unknown":            "自動の動き",
	}
	for kind, want := range cases {
		if got := allowanceKindName(kind); got != want {
			t.Errorf("allowanceKindName(%q) = %q, want %q", kind, got, want)
		}
	}
}
