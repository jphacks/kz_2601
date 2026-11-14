package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const rakutenTestKey = "1012345678901234567"

// rrServer is a stand-in for the Rakuten Recipe API (no live calls).
type rrServer struct {
	*httptest.Server
	path   string
	query  map[string]string
	status int
	body   []byte
}

func newRRServer(t *testing.T, body []byte) *rrServer {
	t.Helper()
	s := &rrServer{status: http.StatusOK, body: body}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.path, s.query = r.URL.Path, map[string]string{}
		for k, v := range r.URL.Query() {
			s.query[k] = strings.Join(v, ",")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write(s.body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *rrServer) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newRakutenRecipe(rakutenTestKey, s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestRakutenRecipeRegistered_2141(t *testing.T) {
	p, ok := Lookup(RakutenRecipe)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://app.rakuten.co.jp"}) {
		t.Fatalf("rakutenRecipe %+v", p)
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL <= 0 {
			t.Fatalf("%s: read ops, cached", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_RAKUTEN_RECIPE_API_KEY": rakutenTestKey}[k]
	})
	if err != nil || cfg.Enabled[RakutenRecipe] || New(cfg, nil, nil, nil, nil, nil).Enabled(RakutenRecipe) {
		t.Fatalf("on without EXT_RAKUTEN_RECIPE_ENABLED: %v", err)
	}
	cfg, err = ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_RAKUTEN_RECIPE_ENABLED": "1", "EXT_RAKUTEN_RECIPE_API_KEY": rakutenTestKey}[k]
	})
	if err != nil || !New(cfg, nil, nil, nil, nil, nil).Enabled(RakutenRecipe) {
		t.Fatalf("EXT_RAKUTEN_RECIPE_ENABLED: %v", err)
	}
	cfg.Secrets[RakutenRecipe] = Secrets{}
	if New(cfg, nil, nil, nil, nil, nil).Enabled(RakutenRecipe) {
		t.Fatal("on without an application id")
	}
}

func TestRakutenRecipeCheckArgs_2141(t *testing.T) {
	p, _ := Lookup(RakutenRecipe)
	for _, args := range []map[string]string{{}, {"parent": "30"}, {"parent": "30-300"}} {
		if _, err := p.CheckArgs("categories", args); err != nil {
			t.Errorf("categories %v: %v", args, err)
		}
	}
	for _, id := range []string{"30", "30-300", "30-300-1234"} {
		if _, err := p.CheckArgs("ranking", map[string]string{"categoryId": id}); err != nil {
			t.Errorf("ranking %s: %v", id, err)
		}
	}
	bad := []struct {
		op   string
		args map[string]string
	}{
		{"categories", map[string]string{"parent": "30-300-1234"}},
		{"categories", map[string]string{"parent": "abc"}},
		{"ranking", map[string]string{}},
		{"ranking", map[string]string{"categoryId": "30-"}},
		{"ranking", map[string]string{"categoryId": "30-300-1234-1"}},
		{"ranking", map[string]string{"categoryId": "30&applicationId=x"}},
		{"ranking", map[string]string{"categoryId": "30", "keyword": "カレー"}},
	}
	for _, c := range bad {
		if _, err := p.CheckArgs(c.op, c.args); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s %v: %v, want bad_request", c.op, c.args, err)
		}
	}
}

func TestRakutenRecipeCategories_2141(t *testing.T) {
	s := newRRServer(t, readTestdata(t, "rakuten_recipe", "categories.json"))
	cases := []struct {
		parent string
		want   []Row
	}{
		{"", []Row{{Title: "人気メニュー", Subtitle: "大分類", Value: "30"}, {Title: "定番の肉料理", Subtitle: "大分類", Value: "31"}}},
		{"30", []Row{{Title: "ハンバーグ", Subtitle: "中分類", Value: "30-300"}, {Title: "餃子", Subtitle: "中分類", Value: "30-301"}}},
		{"30-300", []Row{{Title: "煮込みハンバーグ", Subtitle: "小分類", Value: "30-300-1234"}, {Title: "豆腐ハンバーグ", Subtitle: "小分類", Value: "30-300-1235"}}},
	}
	for _, c := range cases {
		args := map[string]string{}
		if c.parent != "" {
			args["parent"] = c.parent
		}
		out, err := s.ops(t)["categories"].Do(context.Background(), Input{Args: args})
		if err != nil {
			t.Fatalf("%q: %v", c.parent, err)
		}
		if !slices.Equal(out.Rows, c.want) {
			t.Errorf("%q rows %+v", c.parent, out.Rows)
		}
	}
	if s.path != rakutenCategoryPath || s.query["applicationId"] != rakutenTestKey || s.query["format"] != "json" {
		t.Fatalf("request %s %v", s.path, s.query)
	}
	// a medium id under another large one, or one the list has not
	for _, parent := range []string{"31-300", "99"} {
		if _, err := s.ops(t)["categories"].Do(context.Background(), Input{Args: map[string]string{"parent": parent}}); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s: %v, want bad_request", parent, err)
		}
	}
}

func TestRakutenRecipeRanking_2141(t *testing.T) {
	s := newRRServer(t, readTestdata(t, "rakuten_recipe", "ranking.json"))
	out, err := s.ops(t)["ranking"].Do(context.Background(), Input{Args: map[string]string{"categoryId": "30-300"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.path != rakutenRankingPath || s.query["categoryId"] != "30-300" || s.query["applicationId"] != rakutenTestKey {
		t.Fatalf("request %s %v", s.path, s.query)
	}
	want := []Row{
		{Title: "簡単ふわふわハンバーグ", Subtitle: "約30分・300円前後・材料: 合いびき肉、玉ねぎ、パン粉、牛乳、卵、塩 ほか", Value: "1位", URL: "https://recipe.rakuten.co.jp/recipe/1100001234/"},
		{Title: "豆腐ハンバーグ", Subtitle: "約15分・100円以下・材料: 豆腐、鶏ひき肉", Value: "2位"}, // a link off rakuten.co.jp is dropped
		{Title: "名前だけ"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
}

func TestRakutenRecipeEmptyAndErrors_2141(t *testing.T) {
	s := newRRServer(t, readTestdata(t, "rakuten_recipe", "ranking_empty.json"))
	out, err := s.ops(t)["ranking"].Do(context.Background(), Input{Args: map[string]string{"categoryId": "30"}})
	if err != nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out, err)
	}
	s.status, s.body = http.StatusTooManyRequests, readTestdata(t, "rakuten_recipe", "too_many_requests.json")
	var pe *ProviderError
	_, err = s.ops(t)["ranking"].Do(context.Background(), Input{Args: map[string]string{"categoryId": "30"}})
	if !errors.As(err, &pe) || !pe.Retryable || pe.Status != http.StatusTooManyRequests {
		t.Fatalf("429: %v", err)
	}
	if strings.Contains(err.Error(), rakutenTestKey) {
		t.Fatal("the error carries the application id")
	}
	s.status, s.body = http.StatusBadRequest, []byte(`{"error":"wrong_parameter","error_description":"categoryId is not valid"}`)
	if _, err := s.ops(t)["ranking"].Do(context.Background(), Input{Args: map[string]string{"categoryId": "9999"}}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("400: %v, want bad_request", err)
	}
	if _, err := newRakutenRecipe("", s.URL, s.Client()); err == nil {
		t.Fatal("built without an application id")
	}
}
