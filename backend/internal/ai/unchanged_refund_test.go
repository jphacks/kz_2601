package ai

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// An edit that changed nothing (#211 unchanged) gets its charge back through
// the real rate limiter, in the JSON mode and over SSE.
func TestRateLimitRefundsUnchangedEdit(t *testing.T) {
	kb := kakeibo(t)
	noop := `{"summary":"項目は変更できませんでした。","patch":[{"op":"test","path":"/entities/0/id","value":"expense"}]}`
	cases := []struct{ path, mode string }{
		{"/v1/generate", "edit"},
		{"/v1/generate?stream=1", "edit"},
		{"/v1/generate", "auto"},
	}
	for _, c := range cases {
		t.Run(c.path+" "+c.mode, func(t *testing.T) {
			app := appconfigclient.NewMemory()
			if _, err := app.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
				t.Fatal(err)
			}
			h, store := refundStack(t, &fakeCaller{replies: []string{noop}}, app)
			rec := serve(h, http.MethodPost, c.path, `{"appId":"a1","transcript":"項目を足して","mode":"`+c.mode+`"}`)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"unchanged":true`) {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
			charged := ratelimit.DefaultWeights.Edit
			if c.mode == "auto" {
				charged = ratelimit.DefaultWeights.Create
			}
			if store.n.Load() != 1 || store.refunds.Load() != 1 || store.unchanged.Load() != 1 {
				t.Fatalf("charged %d refunded %d unchanged %d, want 1/1/1", store.n.Load(), store.refunds.Load(), store.unchanged.Load())
			}
			if got := store.refundUnits.Load(); got != int32(charged) {
				t.Fatalf("refunded %d units, want %d", got, charged)
			}
		})
	}
}
