package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestAddNumberTable2589(t *testing.T) {
	cases := []struct {
		name string
		v    any
		by   float64
		want any
	}{
		{name: "missing counts as 0", v: nil, by: 1, want: int64(1)},
		{name: "text counts as 0", v: "12", by: 2, want: int64(2)},
		{name: "bool counts as 0", v: true, by: 3, want: int64(3)},
		{name: "int64", v: int64(5), by: 1, want: int64(6)},
		{name: "int", v: 5, by: -2, want: int64(3)},
		{name: "float64 whole stays integer", v: 1.5, by: 0.5, want: int64(2)},
		{name: "fraction stays float", v: int64(1), by: 0.25, want: 1.25},
		{name: "float plus fraction", v: 0.5, by: 0.25, want: 0.75},
		{name: "json number", v: json.Number("7"), by: 1, want: int64(8)},
		{name: "json number fraction", v: json.Number("1.5"), by: 1, want: 2.5},
		{name: "bad json number counts as 0", v: json.Number("x"), by: 4, want: int64(4)},
		{name: "negative result", v: int64(1), by: -3, want: int64(-2)},
		{name: "beyond 2^53 stays float", v: float64(1 << 53), by: 2, want: float64(1<<53) + 2},
		{name: "infinite stays float", v: math.Inf(1), by: 1, want: math.Inf(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := addNumber(tc.v, tc.by)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("addNumber(%#v, %v) = %#v (%T), want %#v (%T)", tc.v, tc.by, got, got, tc.want, tc.want)
			}
		})
	}
}

// dropStore2589 fails DeleteDeviceToken for one token.
type dropStore2589 struct {
	*store.Memory
	failToken string
}

func (s *dropStore2589) DeleteDeviceToken(ctx context.Context, uid, token string) (int, error) {
	if token == s.failToken {
		return 0, errors.New("boom")
	}
	return s.Memory.DeleteDeviceToken(ctx, uid, token)
}

func TestDropUnregistered2589(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemory()
	for _, d := range []struct{ uid, inst, tok string }{
		{"u1", "i1", "tok-a"}, {"u1", "i2", "tok-keep"}, {"u2", "i3", "tok-b"}, {"u2", "i4", "tok-fail"},
	} {
		if _, err := mem.PutDevice(ctx, d.uid, store.Device{InstallationID: d.inst, Token: d.tok}); err != nil {
			t.Fatal(err)
		}
	}
	st := &dropStore2589{Memory: mem, failToken: "tok-fail"}
	h, err := scope.New(st, "test", store.OutboxEvent{AppID: "app-2589", UID: "u1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	r := &Runner{cfg: Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))}}

	r.dropUnregistered(ctx, h, []notify.DeviceToken{
		{UID: "u1", Token: "tok-a"},
		{UID: "u2", Token: "tok-fail"}, // logged, the rest goes on
		{UID: "u2", Token: "tok-b"},
		{UID: "u1", Token: "tok-unknown"}, // nothing to delete: no warning
	})

	tokens := func(uid string) []string {
		ds, err := mem.ListDevices(ctx, uid)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range ds {
			out = append(out, d.Token)
		}
		return out
	}
	if got := tokens("u1"); !reflect.DeepEqual(got, []string{"tok-keep"}) {
		t.Errorf("u1 tokens = %v, want [tok-keep]", got)
	}
	if got := tokens("u2"); !reflect.DeepEqual(got, []string{"tok-fail"}) {
		t.Errorf("u2 tokens = %v, want [tok-fail]", got)
	}
	if n := strings.Count(logs.String(), "unregistered device token not deleted"); n != 1 {
		t.Errorf("warnings = %d, want 1: %s", n, logs.String())
	}
	if strings.Contains(logs.String(), "tok-fail") {
		t.Errorf("the token must not be logged: %s", logs.String())
	}

	logs.Reset()
	r.dropUnregistered(ctx, h, nil)
	if logs.Len() != 0 {
		t.Errorf("no tokens: logged %s", logs.String())
	}
}

type allPro2589 struct{}

func (allPro2589) IsPro(context.Context, string) bool { return true }

func TestRecipientLimiterLimit2589(t *testing.T) {
	ctx := context.Background()
	newQ := func() *quota.Quota {
		return quota.New(quota.Config{Env: "test", Limits: quota.Limits{AppNotifyPro: 2}}, nil, allPro2589{})
	}
	begin := func(t *testing.T, q *quota.Quota) quota.Decision {
		t.Helper()
		d, err := q.Begin(ctx, quota.Run{AppID: "app-2589", CreatorUID: "c1", ActorUID: "a1", Attempt: 1})
		if err != nil || !d.Allowed {
			t.Fatalf("begin: %+v %v", d, err)
		}
		return d
	}

	t.Run("at the limit", func(t *testing.T) {
		q := newQ()
		var charged quota.Charge
		var usage quota.Usage
		stopped := ""
		l := recipientLimiter{q: q, dec: begin(t, q), charged: &charged, usage: &usage, stopped: &stopped}
		allowed, drops, err := l.Limit(ctx, []string{"u1", "u2"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(allowed, []string{"u1", "u2"}) || len(drops) != 0 {
			t.Fatalf("allowed=%v drops=%v", allowed, drops)
		}
		if charged.Empty() {
			t.Error("the grant's charge must be kept for a refund")
		}
		if usage.Limit != "" || stopped != "" {
			t.Errorf("usage.Limit=%q stopped=%q", usage.Limit, stopped)
		}
	})

	t.Run("one over", func(t *testing.T) {
		q := newQ()
		var usage quota.Usage
		l := recipientLimiter{q: q, dec: begin(t, q), usage: &usage}
		allowed, drops, err := l.Limit(ctx, []string{"u1", "u2", "u3", "u1", ""})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(allowed, []string{"u1", "u2"}) {
			t.Fatalf("allowed = %v", allowed)
		}
		if want := []notify.LimitDrop{{UID: "u3", Scope: quota.ScopeAppNotify}}; !reflect.DeepEqual(drops, want) {
			t.Fatalf("drops = %+v, want %+v", drops, want)
		}
		if usage.Limit != quota.ScopeAppNotify {
			t.Errorf("usage.Limit = %q", usage.Limit)
		}
	})

	t.Run("run not allowed: everyone dropped", func(t *testing.T) {
		var usage quota.Usage
		stopped := ""
		dec := quota.Decision{Block: &quota.Block{Scope: quota.ScopeAppNotify, Stop: quota.OutcomeSkippedAllowance}}
		l := recipientLimiter{q: newQ(), dec: dec, usage: &usage, stopped: &stopped}
		allowed, drops, err := l.Limit(ctx, []string{"u1", "u2"})
		if err != nil {
			t.Fatal(err)
		}
		if len(allowed) != 0 || len(drops) != 2 {
			t.Fatalf("allowed=%v drops=%v", allowed, drops)
		}
		if usage.Limit != quota.ScopeAppNotify || stopped != quota.OutcomeSkippedAllowance {
			t.Errorf("usage.Limit=%q stopped=%q", usage.Limit, stopped)
		}
	})

	t.Run("stop on another scope without sinks", func(t *testing.T) {
		dec := quota.Decision{Block: &quota.Block{Scope: "creator_month", Stop: quota.OutcomeSkippedAllowance}}
		l := recipientLimiter{q: newQ(), dec: dec}
		_, drops, err := l.Limit(ctx, []string{"u1"})
		if err != nil || len(drops) != 1 || drops[0].Scope != "creator_month" {
			t.Fatalf("drops=%v err=%v", drops, err)
		}
	})

	t.Run("stop on another scope with sinks", func(t *testing.T) {
		var usage quota.Usage
		stopped := ""
		dec := quota.Decision{Block: &quota.Block{Scope: "creator_month", Stop: quota.OutcomeSkippedAllowance}}
		l := recipientLimiter{q: newQ(), dec: dec, usage: &usage, stopped: &stopped}
		if _, _, err := l.Limit(ctx, []string{"u1"}); err != nil {
			t.Fatal(err)
		}
		if usage.Limit != "creator_month" || stopped != quota.OutcomeSkippedAllowance {
			t.Errorf("usage.Limit=%q stopped=%q", usage.Limit, stopped)
		}
	})

	t.Run("no recipients", func(t *testing.T) {
		q := newQ()
		l := recipientLimiter{q: q, dec: begin(t, q)}
		allowed, drops, err := l.Limit(ctx, nil)
		if err != nil || len(allowed) != 0 || len(drops) != 0 {
			t.Fatalf("allowed=%v drops=%v err=%v", allowed, drops, err)
		}
	})
}
