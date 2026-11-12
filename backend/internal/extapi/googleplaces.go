package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Google Places API (New) (#1501): find places by words or near a point.
// Paid per request (the SKU is set by the field mask): a call is one `runs`
// and coins at EXT_COST_USD_GOOGLE_PLACES_SEARCH_TEXT /
// _SEARCH_NEARBY (the API reports no cost).
//
//	searchText(q, lat?, lng?, radius?, limit?)      Rows; lat / lng bias the search
//	searchNearby(type?, lat, lng, radius?, limit?)  Rows within the circle
//
// A row is the place's name / short address / place id / Google Maps link.
// The field mask (placesFieldMask) asks for those 4 fields only, so a call
// stays in the cheapest SKU that has a name; no rating, hours, phone,
// reviews or photos. radius is metres (1-50000, default 5000 for a text
// search with a point and 1000 nearby), limit 1-20 (default 10); lat / lng
// are rounded to 4 places (about 10 m).
//
// Terms (Places API policies): only the place id may be kept; names and
// addresses must not be stored or cached, and the results are shown with
// the Google Maps credit (GooglePlacesAttribution). So nothing here is
// cached, Row.Value carries the place id (the one value a field may keep)
// and the answer goes back only to be shown. The key goes in the
// X-Goog-Api-Key header, never in a URL or a log.
const (
	GooglePlaces = "googlePlaces"
	// GooglePlacesAttribution is the credit shown with the rows.
	GooglePlacesAttribution = "Google Maps"

	googlePlacesHost = "https://places.googleapis.com"
	placesFieldMask  = "places.id,places.displayName,places.shortFormattedAddress,places.googleMapsUri"
	placesMaxQuery   = 200 // runes
)

var (
	placesKeyRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{20,64}$`)
	placesTypeRe = regexp.MustCompile(`^[a-z][a-z_]{1,49}$`)
)

func init() {
	register(Provider{ID: GooglePlaces, Billing: BillingPaid,
		Ops: []OpSpec{
			{Name: "searchText", Args: []string{"q", "lat", "lng", "radius", "limit"}},
			{Name: "searchNearby", Args: []string{"type", "lat", "lng", "radius", "limit"}},
		},
		Hosts: []string{googlePlacesHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newGooglePlaces(sec.APIKey, googlePlacesHost, NewHTTPClient([]string{googlePlacesHost}, DefaultTimeout))
		}})
}

// newGooglePlaces builds the ops against base (the API, or a test server).
func newGooglePlaces(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !placesKeyRe.MatchString(key) {
		return nil, errors.New("EXT_GOOGLE_PLACES_API_KEY is not set or not a key")
	}
	g := &googlePlaces{key: key, base: base, hc: hc}
	return map[string]Operation{
		"searchText":   OperationFunc(g.searchText),
		"searchNearby": OperationFunc(g.searchNearby),
	}, nil
}

type googlePlaces struct {
	key  string
	base string
	hc   *http.Client
}

type placesLatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type placesCircle struct {
	Circle struct {
		Center placesLatLng `json:"center"`
		Radius float64      `json:"radius"`
	} `json:"circle"`
}

type placesTextRequest struct {
	TextQuery    string        `json:"textQuery"`
	PageSize     int           `json:"pageSize"`
	LanguageCode string        `json:"languageCode"`
	LocationBias *placesCircle `json:"locationBias,omitempty"`
}

type placesNearbyRequest struct {
	IncludedTypes       []string     `json:"includedTypes,omitempty"`
	MaxResultCount      int          `json:"maxResultCount"`
	LanguageCode        string       `json:"languageCode"`
	LocationRestriction placesCircle `json:"locationRestriction"`
}

type placesAnswer struct {
	Places []struct {
		ID          string `json:"id"`
		DisplayName struct {
			Text string `json:"text"`
		} `json:"displayName"`
		ShortFormattedAddress string `json:"shortFormattedAddress"`
		GoogleMapsURI         string `json:"googleMapsUri"`
	} `json:"places"`
}

// placesCircleArg reads lat / lng / radius (nil when no point was given).
func placesCircleArg(args map[string]string, defRadius int) (*placesCircle, error) {
	lat, lng, ok, err := argLatLng(args, 4)
	if err != nil || !ok {
		if err == nil && strings.TrimSpace(args["radius"]) != "" {
			err = fmt.Errorf("%w: radius needs lat and lng", ErrUnknown)
		}
		return nil, err
	}
	r, err := argInt(args, "radius", defRadius, 1, 50000)
	if err != nil {
		return nil, err
	}
	c := &placesCircle{}
	c.Circle.Center = placesLatLng{Latitude: lat, Longitude: lng}
	c.Circle.Radius = float64(r)
	return c, nil
}

func (g *googlePlaces) searchText(ctx context.Context, in Input) (Output, error) {
	q := strings.TrimSpace(in.Args["q"])
	if q == "" || utf8.RuneCountInString(q) > placesMaxQuery {
		return Output{}, fmt.Errorf("%w: q must be 1-%d characters", ErrUnknown, placesMaxQuery)
	}
	limit, err := argInt(in.Args, "limit", 10, 1, 20)
	if err != nil {
		return Output{}, err
	}
	bias, err := placesCircleArg(in.Args, 5000)
	if err != nil {
		return Output{}, err
	}
	return g.post(ctx, "/v1/places:searchText", placesTextRequest{TextQuery: q, PageSize: limit, LanguageCode: "ja", LocationBias: bias}, limit)
}

func (g *googlePlaces) searchNearby(ctx context.Context, in Input) (Output, error) {
	typ := strings.TrimSpace(in.Args["type"])
	if typ != "" && !placesTypeRe.MatchString(typ) {
		return Output{}, fmt.Errorf("%w: type", ErrUnknown)
	}
	limit, err := argInt(in.Args, "limit", 10, 1, 20)
	if err != nil {
		return Output{}, err
	}
	c, err := placesCircleArg(in.Args, 1000)
	if err != nil {
		return Output{}, err
	}
	if c == nil {
		return Output{}, fmt.Errorf("%w: lat and lng are required", ErrUnknown)
	}
	body := placesNearbyRequest{MaxResultCount: limit, LanguageCode: "ja", LocationRestriction: *c}
	if typ != "" {
		body.IncludedTypes = []string{typ}
	}
	return g.post(ctx, "/v1/places:searchNearby", body, limit)
}

func (g *googlePlaces) post(ctx context.Context, path string, body any, limit int) (Output, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return Output{}, errors.New("googleplaces: building the request failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+path, bytes.NewReader(b))
	if err != nil {
		return Output{}, errors.New("googleplaces: building the request failed")
	}
	req.Header.Set("X-Goog-Api-Key", g.key)
	req.Header.Set("X-Goog-FieldMask", placesFieldMask)
	req.Header.Set("Content-Type", "application/json")
	var a placesAnswer
	if err := sendJSON(ctx, g.hc, req, &a); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(limit, len(a.Places)))
	for _, p := range a.Places {
		if len(rows) == limit {
			break
		}
		if p.ID == "" || p.DisplayName.Text == "" {
			continue
		}
		row := Row{Title: p.DisplayName.Text, Subtitle: p.ShortFormattedAddress, Value: p.ID}
		if strings.HasPrefix(p.GoogleMapsURI, "https://") {
			row.URL = p.GoogleMapsURI
		}
		rows = append(rows, row)
	}
	// no places is an answer (0 rows), not a failure: the request was billed
	return Output{Rows: rows}, nil
}
