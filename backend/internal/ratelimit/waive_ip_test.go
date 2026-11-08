package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// Waives are counted per client address too: new uids from one address do
// not get past the address's hourly cap.
func TestWaiveIPHourlyCap(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 25, 22, 10, 0, 0, JST)}
	m := NewMemory()
	l := New(Config{Limits: Limits{Anonymous: 30, PerIP: 300, Global: 2000, WaivedHourly: 5, WaivedIPHourly: 2},
		Weights: DefaultWeights, Env: "test", Now: c.now}, m, nil)
	h := func(got *bool) http.Handler { return l.Middleware(auth.DevVerifier{})(waiving(http.StatusPaymentRequired, got)) }
	for i, want := range []bool{true, true, false} {
		var waived bool
		serve(t, h(&waived), "POST", "/v1/text", "anon:new"+string(rune('a'+i)), `{}`) // same address, new uid each time
		if waived != want {
			t.Fatalf("#%d waived %v, want %v", i, waived, want)
		}
	}
	c.t = c.t.Add(time.Hour)
	var waived bool
	if serve(t, h(&waived), "POST", "/v1/text", "anon:newz", `{}`); !waived {
		t.Fatal("next hour should waive again")
	}
}

// refundFailing fails the refund of the daily counters only.
type refundFailing struct{ *Memory }

func (s refundFailing) Refund(ctx context.Context, cs []Counter, w int, exp time.Time) error {
	for _, c := range cs {
		if strings.Contains(c.ID, "_uid_") {
			return errors.New("refund failed")
		}
	}
	return s.Memory.Refund(ctx, cs, w, exp)
}

// When the daily units cannot be given back, the waive is not counted
// either: the request keeps its charge and the waive counters stay as they were.
func TestWaiveRefundFailureGivesTheWaiveCountBack(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 25, 22, 10, 0, 0, JST)}
	m := NewMemory()
	m.now = c.now
	l := New(Config{Limits: Limits{SignedIn: 60, Global: 2000, WaivedHourly: 5, WaivedIPHourly: 5},
		Weights: DefaultWeights, Env: "test", Now: c.now}, refundFailing{m}, nil)
	var waived bool
	serve(t, l.Middleware(auth.DevVerifier{})(waiving(http.StatusPaymentRequired, &waived)), "POST", "/v1/text", "u1", `{}`)
	if waived {
		t.Fatal("waived although the refund failed")
	}
	base := "test_2026-10-25T22_waived_"
	got, err := m.Peek(context.Background(), []Counter{{ID: base + "u1"}, {ID: base + "ip_" + ipKey("203.0.113.7")}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0 || got[1] != 0 {
		t.Errorf("waive counts = %v, want [0 0]", got)
	}
	if u, _, _ := counts(t, m, "u1", ""); u != DefaultWeights.Text {
		t.Errorf("user = %d, want %d (charged)", u, DefaultWeights.Text)
	}
}
