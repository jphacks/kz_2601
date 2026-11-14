package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Rakuten Recipe (#2141, a trial for #1700): the recipe categories and the
// top recipes of a category from Rakuten Web Service (Recipe API
// 20170426). The API has no word search. Free per call, the operator's
// application id (EXT_RAKUTEN_RECIPE_API_KEY, Secret Manager) in the query
// as the API wants; errors never carry the URL (keyed.go). Off by default
// (EXT_RAKUTEN_RECIPE_ENABLED=1).
//
//	categories(parent?)   Rows: no parent = the large categories; a large
//	                      id ("30") = its medium ones; a medium id
//	                      ("30-300") = its small ones. Value = the id ranking takes.
//	ranking(categoryId)   Rows: the top recipes (the API gives 4): title /
//	                      time and cost guide · materials / "1位" / link
//
// Whether to ship it, the credit line and how the application id is handled
// under the terms are #1700's decision; nothing about them is fixed here.
const RakutenRecipe = "rakutenRecipe"

var rakutenRecipeHosts = []string{"https://app.rakuten.co.jp"}

const (
	rakutenCategoryPath = "/services/api/Recipe/CategoryList/20170426"
	rakutenRankingPath  = "/services/api/Recipe/CategoryRanking/20170426"
	rakutenMaxRows      = 200
	rakutenMaxMaterials = 6
)

var (
	// rakutenCategoryRe is a large, medium or small id as ranking takes it:
	// "30", "30-300", "30-300-1234".
	rakutenCategoryRe = regexp.MustCompile(`^[0-9]{1,4}(-[0-9]{1,6}){0,2}$`)
	rakutenKeyRe      = regexp.MustCompile(`^[0-9A-Za-z_-]{8,64}$`)
)

func init() {
	register(Provider{ID: RakutenRecipe, Billing: BillingFree, Hosts: rakutenRecipeHosts,
		Ops: []OpSpec{
			{Name: "categories", Args: []string{"parent"}, Check: checkRakutenParent, CacheTTL: 24 * time.Hour},
			{Name: "ranking", Args: []string{"categoryId"}, Check: checkRakutenRanking, CacheTTL: time.Hour},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newRakutenRecipe(sec.APIKey, rakutenRecipeHosts[0], NewHTTPClient(rakutenRecipeHosts, DefaultTimeout))
		}})
}

// checkRakutenParent: parent is a large or medium id (a small one has no children).
func checkRakutenParent(a map[string]string) error {
	if v, ok := a["parent"]; ok && (!rakutenCategoryRe.MatchString(v) || strings.Count(v, "-") > 1) {
		return errors.New("parent must be a large or medium category id like 30 or 30-300")
	}
	return nil
}

func checkRakutenRanking(a map[string]string) error {
	if !rakutenCategoryRe.MatchString(a["categoryId"]) {
		return errors.New("categoryId must be a category id like 30, 30-300 or 30-300-1234")
	}
	return nil
}

// newRakutenRecipe builds the ops against base (tests pass an httptest server).
func newRakutenRecipe(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !rakutenKeyRe.MatchString(key) {
		return nil, errors.New("EXT_RAKUTEN_RECIPE_API_KEY is not set or not an application id")
	}
	r := &rakutenRecipe{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"categories": OperationFunc(r.categories),
		"ranking":    OperationFunc(r.ranking),
	}, nil
}

type rakutenRecipe struct {
	key  string
	base string
	hc   *http.Client
}

func (r *rakutenRecipe) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("applicationId", r.key)
	q.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("rakuten recipe: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	err = sendJSON(ctx, r.hc, req, out)
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == http.StatusBadRequest || pe.Status == http.StatusNotFound) {
		// wrong_parameter / not_found: a category the API does not have
		return fmt.Errorf("%w: rakuten recipe %d", ErrUnknown, pe.Status)
	}
	return err
}

// rakutenID is a category id; the API writes them as strings or numbers.
type rakutenID string

func (id *rakutenID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if _, err := strconv.ParseUint(s, 10, 32); err != nil {
		return fmt.Errorf("category id %s", b)
	}
	*id = rakutenID(s)
	return nil
}

type rakutenCategory struct {
	ID     rakutenID `json:"categoryId"`
	Name   string    `json:"categoryName"`
	Parent rakutenID `json:"parentCategoryId"`
}

type rakutenCategoryAnswer struct {
	Result struct {
		Large  []rakutenCategory `json:"large"`
		Medium []rakutenCategory `json:"medium"`
		Small  []rakutenCategory `json:"small"`
	} `json:"result"`
}

// categories: one request for the whole list (cached a day), cut to the
// level parent asks for. A medium id is "large-medium" and a small one
// "large-medium-small", as ranking takes them.
func (r *rakutenRecipe) categories(ctx context.Context, in Input) (Output, error) {
	parent := in.Args["parent"]
	if err := checkRakutenParent(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	var a rakutenCategoryAnswer
	if err := r.get(ctx, rakutenCategoryPath, url.Values{}, &a); err != nil {
		return Output{}, err
	}
	res := a.Result
	if len(res.Large) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("rakuten recipe: no categories")}
	}
	// the large id of each medium one, for the small ones' full ids
	mediumParent := map[rakutenID]rakutenID{}
	for _, m := range res.Medium {
		mediumParent[m.ID] = m.Parent
	}
	rows := []Row{}
	add := func(c rakutenCategory, id, sub string) {
		if len(rows) < rakutenMaxRows && strings.TrimSpace(c.Name) != "" {
			rows = append(rows, Row{Title: clip(strings.TrimSpace(c.Name), 60), Subtitle: sub, Value: id})
		}
	}
	switch large, medium, _ := strings.Cut(parent, "-"); {
	case parent == "":
		for _, c := range res.Large {
			add(c, string(c.ID), "大分類")
		}
	case medium == "":
		for _, c := range res.Medium {
			if string(c.Parent) == large {
				add(c, large+"-"+string(c.ID), "中分類")
			}
		}
	default:
		for _, c := range res.Small {
			if string(c.Parent) == medium && string(mediumParent[c.Parent]) == large {
				add(c, parent+"-"+string(c.ID), "小分類")
			}
		}
	}
	if len(rows) == 0 {
		return Output{}, fmt.Errorf("%w: rakuten recipe has no category %s", ErrUnknown, parent)
	}
	return Output{Rows: rows}, nil
}

type rakutenRecipeItem struct {
	Title      string   `json:"recipeTitle"`
	URL        string   `json:"recipeUrl"`
	Materials  []string `json:"recipeMaterial"`
	Indication string   `json:"recipeIndication"` // "約10分"
	Cost       string   `json:"recipeCost"`       // "300円前後"
	Rank       string   `json:"rank"`
}

type rakutenRankingAnswer struct {
	Result []rakutenRecipeItem `json:"result"`
}

// ranking: the top recipes of categoryId, in rank order. A link that is not
// https on a rakuten.co.jp host is dropped (the row stays).
func (r *rakutenRecipe) ranking(ctx context.Context, in Input) (Output, error) {
	id := in.Args["categoryId"]
	if err := checkRakutenRanking(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	var a rakutenRankingAnswer
	if err := r.get(ctx, rakutenRankingPath, url.Values{"categoryId": {id}}, &a); err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, it := range a.Result {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			continue
		}
		row := Row{Title: clip(title, 80), Subtitle: rakutenSubtitle(it), URL: rakutenLink(it.URL)}
		if n, err := strconv.Atoi(strings.TrimSpace(it.Rank)); err == nil && n > 0 {
			row.Value = strconv.Itoa(n) + "位"
		}
		rows = append(rows, row)
	}
	return Output{Rows: rows}, nil
}

// rakutenSubtitle writes "約10分・300円前後・材料: a、b、c ほか".
func rakutenSubtitle(it rakutenRecipeItem) string {
	var mats []string
	for _, m := range it.Materials {
		if m = strings.TrimSpace(m); m != "" {
			mats = append(mats, clip(m, 20))
		}
	}
	m := ""
	if len(mats) > 0 {
		more := ""
		if len(mats) > rakutenMaxMaterials {
			mats, more = mats[:rakutenMaxMaterials], " ほか"
		}
		m = "材料: " + strings.Join(mats, "、") + more
	}
	return joinDots(clip(strings.TrimSpace(it.Indication), 20), clip(strings.TrimSpace(it.Cost), 20), m)
}

func rakutenLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || (u.Host != "rakuten.co.jp" && !strings.HasSuffix(u.Host, ".rakuten.co.jp")) {
		return ""
	}
	return u.String()
}
