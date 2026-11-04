package ai

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// refundStack is NewHandler behind the real rate limiter, like cmd/ai (#548).
func refundStack(t *testing.T, f *fakeCaller, store appconfigclient.Store) (http.Handler, *countingStore) {
	t.Helper()
	svc := &Service{Claude: f, Store: store, Model: "test-model", Verifier: auth.DevVerifier{}, AuthMode: auth.ModeDev}
	counting := &countingStore{inner: ratelimit.NewMemory()}
	limiter := ratelimit.New(ratelimit.Config{Limits: ratelimit.DefaultLimits, Weights: ratelimit.DefaultWeights}, counting, nil)
	var logs bytes.Buffer
	return Pause(false, slog.New(slog.NewJSONHandler(&logs, nil)))(limiter.Middleware(auth.DevVerifier{})(NewHandler(svc))), counting
}

// Claude failing (no replies: Claude 500 -> 503 upstream_unavailable) refunds,
// in the JSON mode and when the SSE stream ends with `event: error`.
func TestRateLimitRefundsClaudeFailure(t *testing.T) {
	for _, path := range []string{"/v1/generate", "/v1/generate?stream=1"} {
		t.Run(path, func(t *testing.T) {
			h, store := refundStack(t, &fakeCaller{}, appconfigclient.NewMemory())
			rec := serve(h, http.MethodPost, path, `{"appId":"a1","transcript":"家計簿を作って","mode":"auto"}`)
			if !strings.Contains(rec.Body.String(), "upstream_unavailable") {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if store.n.Load() != 1 || store.refunds.Load() != 1 {
				t.Fatalf("charged %d refunded %d, want 1/1", store.n.Load(), store.refunds.Load())
			}
			if want := int32(ratelimit.DefaultWeights.Create); store.refundUnits.Load() != want {
				t.Fatalf("refunded %d units, want %d", store.refundUnits.Load(), want)
			}
		})
	}
}

// mode=auto resolved to an edit: charged as create, the difference comes back.
func TestRateLimitRefundsAutoResolvedToEdit(t *testing.T) {
	kb := kakeibo(t)
	app := appconfigclient.NewMemory()
	if _, err := app.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	goodPatch := `{"patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"}],"summary":"カテゴリにカフェを追加しました"}`
	for _, path := range []string{"/v1/generate", "/v1/generate?stream=1"} {
		t.Run(path, func(t *testing.T) {
			h, store := refundStack(t, &fakeCaller{replies: []string{goodPatch}}, app)
			rec := serve(h, http.MethodPost, path, `{"appId":"a1","transcript":"カテゴリにカフェを追加して","mode":"auto"}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"patch"`) {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if store.n.Load() != 1 || store.refunds.Load() != 1 {
				t.Fatalf("charged %d refunded %d, want 1/1", store.n.Load(), store.refunds.Load())
			}
			if want := int32(ratelimit.DefaultWeights.Create - ratelimit.DefaultWeights.Edit); store.refundUnits.Load() != want {
				t.Fatalf("refunded %d units, want %d", store.refundUnits.Load(), want)
			}
		})
	}
}

// A client error (400) and a create that really created keep the charge.
func TestRateLimitKeepsChargeOnClientErrorAndCreate(t *testing.T) {
	kb := kakeibo(t)
	cases := []struct {
		name, body string
		replies    []string
	}{
		{"missing transcript 400", `{"appId":"a1","mode":"auto"}`, nil},
		{"auto resolved to create", `{"appId":"a1","transcript":"家計簿アプリを作って","mode":"auto"}`, []string{string(kb)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, store := refundStack(t, &fakeCaller{replies: c.replies}, appconfigclient.NewMemory())
			rec := serve(h, http.MethodPost, "/v1/generate", c.body)
			if rec.Code >= 500 {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			if store.n.Load() != 1 || store.refunds.Load() != 0 {
				t.Fatalf("charged %d refunded %d, want 1/0", store.n.Load(), store.refunds.Load())
			}
		})
	}
}
