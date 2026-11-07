package ai

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// textLimitStack is /v1/text behind the real rate limiter with the Pro gate on.
func textLimitStack(t *testing.T, pro proSet) (http.Handler, *fakeCaller, *countingStore) {
	t.Helper()
	m := store.NewMemory()
	_, freeApp := sharedAppFixture(t, m, "")
	if freeApp != "F" {
		t.Fatal(freeApp)
	}
	fc := &fakeCaller{replies: []string{`{"text":"がんばったね"}`}}
	svc := &Service{Claude: fc, Verifier: auth.DevVerifier{}, AuthMode: auth.ModeDev, Pro: pro, ProMode: paid.ModeEnforce, Members: m}
	counting := &countingStore{inner: ratelimit.NewMemory()}
	limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, counting, nil)
	return limiter.Middleware(auth.DevVerifier{})(NewHandler(svc)), fc, counting
}

func remaining(t *testing.T, h http.Header) int {
	t.Helper()
	n, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining %q", h.Get("X-RateLimit-Remaining"))
	}
	return n
}

// A /v1/text 402 (no Pro covers it, no Claude call) costs no units:
// X-RateLimit-Remaining stays the full limit, and the next accepted call is
// the first one charged.
func TestText402DoesNotChargeTheRateLimit(t *testing.T) {
	limit := ratelimit.DefaultLimits.SignedIn
	cases := []struct{ name, uid, body string }{
		{"no Pro", "u-free", `{"instruction":"励ましの一言"}`},
		{"task", "u-free", `{"task":"summarize","input":"今日は走った"}`},
		{"creator_needs_pro", "member", `{"instruction":"励ましの一言","appId":"F"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pro := proSet{}
			h, fc, counting := textLimitStack(t, pro)
			for i := range 2 {
				rec := postText(t, h, c.uid, c.body)
				if rec.Code != http.StatusPaymentRequired {
					t.Fatalf("#%d status %d body %s", i, rec.Code, rec.Body.String())
				}
				if got := remaining(t, rec.Header()); got != limit {
					t.Fatalf("#%d remaining %d, want %d", i, got, limit)
				}
				if rec.Header().Get("X-RateLimit-Limit") != strconv.Itoa(limit) || rec.Header().Get("X-RateLimit-Reset") == "" {
					t.Fatalf("#%d headers %v", i, rec.Header())
				}
			}
			if len(fc.calls) != 0 {
				t.Fatal("Claude must not be called")
			}
			if counting.n.Load() != 2 || counting.refunds.Load() != 2 {
				t.Fatalf("charged %d refunded %d", counting.n.Load(), counting.refunds.Load())
			}
			// the count really is untouched: an accepted call is charged from the full limit
			pro[c.uid] = true
			rec := postText(t, h, c.uid, `{"instruction":"励ましの一言"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if got, want := remaining(t, rec.Header()), limit-ratelimit.DefaultWeights.Text; got != want {
				t.Fatalf("after OK remaining %d, want %d", got, want)
			}
		})
	}
}

// 400 / 413 keep the charge (like /v1/generate), Pro or not.
func TestTextClientErrorsKeepTheCharge(t *testing.T) {
	limit := ratelimit.DefaultLimits.SignedIn
	big := strings.Repeat("a", TextMaxInputBytes+1)
	cases := []struct {
		name, body string
		weight     int
		status     int
	}{
		{"400 validation", `{"shape":"line"}`, ratelimit.DefaultWeights.Text, http.StatusBadRequest},
		{"400 invalid JSON", `{`, ratelimit.DefaultWeights.Text, http.StatusBadRequest},
		{"400 invalid appId", `{"instruction":"励ましの一言","appId":"a/b"}`, ratelimit.DefaultWeights.Text, http.StatusBadRequest},
		{"400 task without input", `{"task":"summarize"}`, ratelimit.DefaultWeights.TextTask, http.StatusBadRequest},
		{"413 input", `{"instruction":"要約","input":"` + big + `"}`, ratelimit.DefaultWeights.Text, http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, fc, counting := textLimitStack(t, proSet{})
			for i := 1; i <= 2; i++ {
				rec := postText(t, h, "u-free", c.body)
				if rec.Code != c.status {
					t.Fatalf("#%d status %d body %s", i, rec.Code, rec.Body.String())
				}
				if got, want := remaining(t, rec.Header()), limit-i*c.weight; got != want {
					t.Fatalf("#%d remaining %d, want %d", i, got, want)
				}
			}
			if len(fc.calls) != 0 || counting.n.Load() != 2 || counting.refunds.Load() != 0 {
				t.Fatalf("claude %d charged %d refunded %d", len(fc.calls), counting.n.Load(), counting.refunds.Load())
			}
		})
	}
}

// An accepted call that reached Claude keeps its charge.
func TestTextAcceptedCallKeepsTheCharge(t *testing.T) {
	h, _, counting := textLimitStack(t, proSet{"u-pro": true})
	rec := postText(t, h, "u-pro", `{"instruction":"励ましの一言"}`)
	if rec.Code != http.StatusOK || counting.refunds.Load() != 0 {
		t.Fatalf("status %d refunds %d", rec.Code, counting.refunds.Load())
	}
	if got, want := remaining(t, rec.Header()), ratelimit.DefaultLimits.SignedIn-ratelimit.DefaultWeights.Text; got != want {
		t.Fatalf("remaining %d, want %d", got, want)
	}
}
