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
	"unicode"
	"unicode/utf8"
)

// Foursquare Places API: find places near a point and read one place.
// Paid per request: a call is one `runs` and coins at
// EXT_COST_USD_FOURSQUARE_SEARCH / _DETAILS (the API reports no cost).
// docs/decisions/external-apis.md candidate 10.
//
//	search(lat, lng, q?, radius?, limit?)   Rows: places in the circle, nearest first
//	details(placeId)                        Item: one place
//
// A row is the place's name / 「category・address・n m」/ the fsq place id
// / its foursquare.com page. Only those fields are asked for (fields=), so
// a call stays in the base tier ("上位の項目は高い"). radius is metres
// (1-100000, default 1000), limit 1-50 (default 10); lat / lng are rounded
// to 4 places (about 10 m).
//
// Terms: the display and storage rules are not on the pricing page (a
// person reads the terms), so nothing is cached and the answer goes back
// only to be shown, with FoursquareAttribution. The key goes in the
// Authorization header, never in a URL or a log.
const (
	Foursquare = "foursquare"
	// FoursquareAttribution is the credit shown with the rows.
	FoursquareAttribution = "Powered by Foursquare"

	foursquareHost       = "https://places-api.foursquare.com"
	foursquareAPIVersion = "2025-06-17"
	foursquareFields     = "fsq_place_id,name,location,categories,distance"
	foursquareMaxQuery   = 100 // runes
)

var (
	foursquareKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{20,128}$`)
	foursquareIDRe  = regexp.MustCompile(`^[A-Za-z0-9]{8,64}$`)
)

func init() {
	register(Provider{ID: Foursquare, Billing: BillingPaid,
		Ops: []OpSpec{
			{Name: "search", Args: []string{"lat", "lng", "q", "radius", "limit"}, Check: checkFoursquareSearch},
			{Name: "details", Args: []string{"placeId"}, Check: checkFoursquareDetails},
		},
		Hosts: []string{foursquareHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newFoursquare(sec.APIKey, foursquareHost, NewHTTPClient([]string{foursquareHost}, DefaultTimeout))
		}})
}

func checkFoursquareSearch(a map[string]string) error {
	_, _, ok, err := argLatLng(a, 4)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("lat and lng are required")
	}
	if q := strings.TrimSpace(a["q"]); utf8.RuneCountInString(q) > foursquareMaxQuery || strings.IndexFunc(q, unicode.IsControl) >= 0 {
		return fmt.Errorf("q must be one line of at most %d characters", foursquareMaxQuery)
	}
	if _, err := argInt(a, "radius", 1000, 1, 100000); err != nil {
		return err
	}
	_, err = argInt(a, "limit", 10, 1, 50)
	return err
}

func checkFoursquareDetails(a map[string]string) error {
	if !foursquareIDRe.MatchString(strings.TrimSpace(a["placeId"])) {
		return errors.New("placeId must be a Foursquare place id")
	}
	return nil
}

// newFoursquare builds the ops against base (the API, or a test server).
func newFoursquare(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !foursquareKeyRe.MatchString(key) {
		return nil, errors.New("EXT_FOURSQUARE_API_KEY is not set or not a key")
	}
	f := &foursquare{key: key, base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{"search": OperationFunc(f.search), "details": OperationFunc(f.details)}, nil
}

type foursquare struct {
	key  string
	base string
	hc   *http.Client
}

type foursquarePlace struct {
	ID       string `json:"fsq_place_id"`
	Name     string `json:"name"`
	Distance *int   `json:"distance"`
	Location struct {
		FormattedAddress string `json:"formatted_address"`
	} `json:"location"`
	Categories []struct {
		Name string `json:"name"`
	} `json:"categories"`
}

// row: name / category・address・distance / id / the foursquare.com page.
func (p foursquarePlace) row() (Row, bool) {
	name := clip(strings.TrimSpace(p.Name), 200)
	if name == "" || !foursquareIDRe.MatchString(p.ID) {
		return Row{}, false
	}
	var cat, dist string
	if len(p.Categories) > 0 {
		cat = clip(strings.TrimSpace(p.Categories[0].Name), 60)
	}
	if p.Distance != nil && *p.Distance >= 0 {
		dist = strconv.Itoa(*p.Distance) + " m"
	}
	return Row{Title: name, Subtitle: joinDots(cat, clip(strings.TrimSpace(p.Location.FormattedAddress), 200), dist),
		Value: p.ID, URL: "https://foursquare.com/v/" + p.ID}, true
}

func (f *foursquare) search(ctx context.Context, in Input) (Output, error) {
	if err := checkFoursquareSearch(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	lat, lng, _, _ := argLatLng(in.Args, 4)
	radius, _ := argInt(in.Args, "radius", 1000, 1, 100000)
	limit, _ := argInt(in.Args, "limit", 10, 1, 50)
	v := url.Values{
		"ll":     {strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lng, 'f', -1, 64)},
		"radius": {strconv.Itoa(radius)},
		"limit":  {strconv.Itoa(limit)},
		"sort":   {"DISTANCE"},
		"fields": {foursquareFields},
	}
	if q := strings.TrimSpace(in.Args["q"]); q != "" {
		v.Set("query", q)
	}
	var a struct {
		Results []foursquarePlace `json:"results"`
	}
	if err := f.get(ctx, "/places/search?"+v.Encode(), &a); err != nil {
		return Output{}, err
	}
	rows := []Row{}
	for _, p := range a.Results {
		if len(rows) == limit {
			break
		}
		if r, ok := p.row(); ok {
			rows = append(rows, r)
		}
	}
	// no places is an answer (0 rows): the request was billed
	return Output{Rows: rows}, nil
}

func (f *foursquare) details(ctx context.Context, in Input) (Output, error) {
	if err := checkFoursquareDetails(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	id := strings.TrimSpace(in.Args["placeId"])
	var p foursquarePlace
	err := f.get(ctx, "/places/"+url.PathEscape(id)+"?"+url.Values{"fields": {foursquareFields}}.Encode(), &p)
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == http.StatusNotFound || pe.Status == http.StatusBadRequest) {
		return Output{}, fmt.Errorf("%w: foursquare has no such place", ErrUnknown)
	}
	if err != nil {
		return Output{}, err
	}
	r, ok := p.row()
	if !ok {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("foursquare: the place has no name")}
	}
	return Output{Item: &r}, nil
}

func (f *foursquare) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.base+path, nil)
	if err != nil {
		return errors.New("foursquare: building the request failed")
	}
	req.Header.Set("Authorization", "Bearer "+f.key)
	req.Header.Set("X-Places-Api-Version", foursquareAPIVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "ja")
	return sendJSON(ctx, f.hc, req, out)
}
