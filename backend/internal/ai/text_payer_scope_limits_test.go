package ai

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// publishShared publishes raw as appID of pro-owner with "member" in it.
func publishShared(t *testing.T, m *store.Memory, appID, raw string) {
	t.Helper()
	ctx := context.Background()
	if _, err := m.ClaimApp(ctx, "pro-owner", appID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PublishApp(ctx, "pro-owner", appID, json.RawMessage(raw), "shared", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddMember(ctx, appID, store.Member{UID: "member", Via: store.MemberViaShare, InvitedBy: "pro-owner"}); err != nil {
		t.Fatal(err)
	}
}

func buttonDSL(prompts ...string) string {
	var comps []string
	for _, p := range prompts {
		b, _ := json.Marshal(p)
		comps = append(comps, `{"type":"aiButton","task":"generate","prompt":`+string(b)+`,"output":"s"}`)
	}
	return `{"version":1,"screens":[{"id":"home","title":"t","components":[` + strings.Join(comps, ",") + `]}]}`
}

// prefixRe stays within RE2's nesting limit for any prompt length and
// matches every prefix (the cut instruction) and nothing longer.
func TestPrefixRe(t *testing.T) {
	for _, n := range []int{0, 1, 63, 64, 65, 500, 1000} {
		s := strings.Repeat("あ", n)
		re, err := regexp.Compile("^(?:" + prefixRe(s) + ")$")
		if err != nil {
			t.Fatal(n, err)
		}
		for l := 0; l <= n; l++ {
			if !re.MatchString(s[:3*l]) {
				t.Fatalf("%d: prefix %d does not match", n, l)
			}
		}
		if re.MatchString(s + "あ") {
			t.Fatalf("%d: longer text matches", n)
		}
	}
}

// A prompt that is (almost) only placeholders, or has too many of them, is
// not something the creator pays for; one with enough text of its own is.
func TestTemplateEligibility(t *testing.T) {
	many := strings.Repeat("{{a}}、", maxScopePlaceholders+1) + strings.Repeat("十分な長さの説明文です", 3)
	cases := []struct {
		tmpl string
		want bool
	}{
		{"{{title}}", false},
		{"{{input}} {{title}}", false},
		{"{{title}}について一言", false}, // 6 runes of own text
		{"{{title}}について、短く優しい励ましの言葉を一言で書いてください", true}, // 30
		{"励ましの一言", true},       // no field: matched as it is
		{"{{input}}を要約", true}, // the marker is not free text
		{many, false},
		{strings.Repeat("{{a}}", maxScopePlaceholders) + strings.Repeat("十分な長さの説明文です", 3), true},
	}
	for _, c := range cases {
		tm, err := compileTemplate(c.tmpl)
		if err != nil {
			t.Fatalf("%q: %v", c.tmpl, err)
		}
		if got := tm.eligible(); got != c.want {
			t.Errorf("%q eligible = %v, want %v", c.tmpl, got, c.want)
		}
	}
}

// The regexp runs in linear time: a prompt of many placeholders against a
// long instruction that almost matches ends quickly. Wall-clock limits flake
// on slow runners (-race), so compare input sizes n and 8n instead: linear
// time grows about 8x, quadratic 64x.
func TestTemplateMatchLinear(t *testing.T) {
	tmpl := strings.Repeat("{{a}}あ", maxScopePlaceholders) + "終わり" + strings.Repeat("。", 20)
	tm, err := compileTemplate(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	cost := func(runes int) time.Duration {
		s := strings.Repeat("あ", runes-1) + "x"
		start := time.Now()
		if tm.match(s, false) || tm.match(s, true) {
			t.Fatalf("%d runes: matched", runes)
		}
		return time.Since(start)
	}
	if r, ok := linearGrowth(cost, dsl.MaxAIInstructionRunes); !ok {
		t.Errorf("%d -> %d runes took %.1fx longer: not linear", dsl.MaxAIInstructionRunes, growthFactor*dsl.MaxAIInstructionRunes, r)
	}
}

// The check itself still catches a quadratic cost.
func TestLinearGrowthCatchesQuadratic(t *testing.T) {
	var sink int
	quadratic := func(n int) time.Duration {
		start := time.Now()
		for i := range n * n / 2 {
			sink += i & 1
		}
		return time.Since(start)
	}
	if r, ok := linearGrowth(quadratic, dsl.MaxAIInstructionRunes); ok {
		t.Errorf("quadratic grew %.1fx and passed as linear", r)
	}
	_ = sink
}

const (
	growthFactor   = 8  // the large input is growthFactor times the small one
	maxGrowthRatio = 24 // linear ~8x, quadratic ~64x: 3x headroom for noise
)

// linearGrowth compares cost(n) and cost(growthFactor*n). Each size takes
// the fastest of several interleaved runs (noise only adds time), and a
// ratio over maxGrowthRatio is measured again before it counts, so a burst
// of load on a shared runner does not fail it. It returns the best ratio.
func linearGrowth(cost func(n int) time.Duration, n int) (float64, bool) {
	best := math.Inf(1)
	for range 3 {
		small, large := time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)
		for range 7 {
			small = min(small, cost(n))
			large = min(large, cost(growthFactor*n))
		}
		r := float64(large) / float64(max(small, 1))
		best = min(best, r)
		if r <= maxGrowthRatio {
			return best, true
		}
	}
	return best, false
}

// The compiled published DSL is cached per app for a short while, so a burst
// of calls reads apps_public once.
func TestAppScopeCached(t *testing.T) {
	m := store.NewMemory()
	publishShared(t, m, "C", sharedAppDSL)
	cm := &countingMembers{AppMembers: m}
	svc := &Service{Members: cm}
	for range 5 {
		if sc := svc.appScopeOf(context.Background(), "C"); sc == nil || !sc.sends(TextRequest{Instruction: "励ましの一言"}) {
			t.Fatal("no scope")
		}
	}
	if cm.public != 1 {
		t.Errorf("GetPublicApp called %d times, want 1", cm.public)
	}
	var c scopeCache
	now := time.Now()
	for i := range scopeCacheSize + 5 {
		c.put(string(rune('a'+i%26))+strings.Repeat("x", i), &appScope{}, now)
	}
	if c.order.Len() != scopeCacheSize || len(c.items) != scopeCacheSize {
		t.Errorf("cache size %d/%d", c.order.Len(), len(c.items))
	}
	c.put("k", &appScope{}, now)
	if c.get("k", now.Add(scopeCacheTTL)) != nil {
		t.Error("entry past its TTL")
	}
}

type countingMembers struct {
	AppMembers
	public int
}

func (c *countingMembers) GetPublicApp(ctx context.Context, appID string) (*store.PublicApp, error) {
	c.public++
	return c.AppMembers.GetPublicApp(ctx, appID)
}

// One member's creator-paid calls in an app are capped per day apart from
// the app's cap; another member still gets the app's remaining calls.
func TestTextCreatorPaidMemberCap(t *testing.T) {
	m := store.NewMemory()
	publishShared(t, m, "M", sharedAppDSL)
	if _, err := m.AddMember(context.Background(), "M", store.Member{UID: "member2", Via: store.MemberViaShare, InvitedBy: "pro-owner"}); err != nil {
		t.Fatal(err)
	}
	ok := `{"text":"ok"}`
	svc, fc := textSvc(ok, ok, ok, ok)
	svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
	svc.CreatorPaid = &CreatorPaidCap{Daily: 10, MemberDaily: 2, Env: "test"}
	h := NewHandler(svc)
	body := `{"instruction":"励ましの一言","appId":"M"}`
	for i := range 2 {
		if rec := postText(t, h, "member", body); rec.Code != http.StatusOK {
			t.Fatalf("#%d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := postText(t, h, "member", body)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"scope":"app_member"`) {
		t.Fatalf("over the member cap: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "member2", body); rec.Code != http.StatusOK {
		t.Fatalf("another member: %d %s", rec.Code, rec.Body.String())
	}
	// aiChat counts against the same caps
	if rec := postText(t, h, "member2", chatWithApp(t, "p", "M")); rec.Code != http.StatusOK {
		t.Fatalf("chat: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "member2", chatWithApp(t, "p", "M")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("chat over the member cap: %d %s", rec.Code, rec.Body.String())
	}
	if len(fc.calls) != 4 {
		t.Errorf("calls %d, want 4", len(fc.calls))
	}
}

// A Claude 5xx gives the creator-paid call back (like the rate limiter).
func TestTextCreatorPaidRefundOn5xx(t *testing.T) {
	m := store.NewMemory()
	publishShared(t, m, "R", sharedAppDSL)
	svc, _ := textSvc() // no replies: every Claude call fails
	svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
	svc.CreatorPaid = &CreatorPaidCap{Daily: 1, MemberDaily: 1, Env: "test"}
	h := NewHandler(svc)
	body := `{"instruction":"励ましの一言","appId":"R"}`
	for i := range 3 {
		rec := postText(t, h, "member", body)
		if rec.Code < 500 || rec.Code == http.StatusInternalServerError {
			t.Fatalf("#%d: %d %s, want 502/503/504", i, rec.Code, rec.Body.String())
		}
	}
	ok := `{"text":"ok"}`
	svc.Claude = &fakeCaller{replies: []string{ok}}
	if rec := postText(t, h, "member", body); rec.Code != http.StatusOK {
		t.Fatalf("after the failures: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postText(t, h, "member", body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the success counts: %d %s", rec.Code, rec.Body.String())
	}
}

// The field values a creator pays for are capped, and a prompt that is
// only placeholders does not pass any text through at the creator's cost.
func TestTextCreatorPaidPromptRules(t *testing.T) {
	m := store.NewMemory()
	publishShared(t, m, "T", buttonDSL("{{title}}", "{{title}}について、短く優しい励ましの言葉を一言で書いてください"))
	ok := `{"text":"ok"}`
	for _, c := range []struct {
		name, instruction string
		status            int
	}{
		{"only a placeholder", "好きな指示をここに書く", http.StatusPaymentRequired},
		{"filled in", "試験について、短く優しい励ましの言葉を一言で書いてください", http.StatusOK},
		{"value too long", strings.Repeat("長", 401) + "について、短く優しい励ましの言葉を一言で書いてください", http.StatusPaymentRequired},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := textSvc(ok)
			svc.Pro, svc.ProMode, svc.Members = proSet{"pro-owner": true}, paid.ModeEnforce, m
			b, _ := json.Marshal(map[string]string{"instruction": c.instruction, "appId": "T"})
			if rec := postText(t, NewHandler(svc), "member", string(b)); rec.Code != c.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreatorPaidMemberDailyFromEnv(t *testing.T) {
	for raw, want := range map[string]int{"": DefaultCreatorPaidMemberDaily, "0": 0, "5": 5} {
		got, err := CreatorPaidMemberDailyFromEnv(func(string) string { return raw })
		if err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", raw, got, err, want)
		}
	}
	if _, err := CreatorPaidMemberDailyFromEnv(func(string) string { return "-1" }); err == nil {
		t.Error("-1: no error")
	}
}
