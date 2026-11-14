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
	"unicode"
	"unicode/utf8"
)

// The Movie Database (TMDB) API v3: movies and TV shows by title and the
// trending ones, read with the operator's key (api.themoviedb.org/3,
// language ja-JP). Free: a call is one `runs`.
//
//	search(q, type?, limit?)     /3/search/{movie|tv}: Rows (title / release · overview / vote / themoviedb.org page)
//	trending(type?, window?)     /3/trending/{movie|tv|all}/{day|week}: the same rows (at most 20)
//
// Output.Cells adds the poster's image URL (image.tmdb.org, w185) as a
// column for server-side readers; the runner never fetches it.
//
// Terms (TMDB API Terms of Use): the app shows TMDBAttribution and the TMDB
// logo (less prominent than its own); answers may be cached (6 months at
// most: the shared cache keeps one 1 hour); no AI / ML training. Commercial
// use needs a written agreement with TMDB (docs/backend/features/
// external-api-terms.md, 人の作業).
//
// The key (EXT_TMDB_API_KEY, Secret Manager) is an API read access token
// (v4, sent as Authorization: Bearer) or a v3 api_key (sent in the query, as
// the API wants); errors never carry the URL (keyed.go).
const (
	TMDB = "tmdb"
	// TMDBAttribution is the notice shown with the rows (API Terms of Use 3).
	TMDBAttribution = "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB."

	tmdbHost      = "https://api.themoviedb.org"
	tmdbPageURL   = "https://www.themoviedb.org/"
	tmdbPosterURL = "https://image.tmdb.org/t/p/w185"
	tmdbLanguage  = "ja-JP"
	tmdbCacheTTL  = time.Hour

	tmdbQueryMax      = 100
	tmdbSearchDefault = 10
	tmdbSearchMax     = 20
	tmdbTrendingMax   = 20
	tmdbOverviewRunes = 100
	tmdbTitleRunes    = 120
)

var tmdbKeyRe = regexp.MustCompile(`^[A-Za-z0-9._-]{20,1000}$`)

func init() {
	register(Provider{ID: TMDB, Billing: BillingFree, Hosts: []string{tmdbHost},
		Ops: []OpSpec{
			{Name: "search", Args: []string{"q", "type", "limit"}, Check: checkTMDBSearch, CacheTTL: tmdbCacheTTL},
			{Name: "trending", Args: []string{"type", "window"}, Check: checkTMDBTrending, CacheTTL: tmdbCacheTTL},
		},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newTMDB(sec.APIKey, tmdbHost, NewHTTPClient([]string{tmdbHost}, DefaultTimeout))
		}})
}

// argTMDBQuery reads q: 1-100 runes, no control characters.
func argTMDBQuery(args map[string]string) (string, error) {
	q := strings.TrimSpace(args["q"])
	if q == "" || !utf8.ValidString(q) || utf8.RuneCountInString(q) > tmdbQueryMax ||
		strings.ContainsFunc(q, unicode.IsControl) {
		return "", fmt.Errorf("%w: q must be 1-%d characters", ErrBadArgs, tmdbQueryMax)
	}
	return q, nil
}

// argTMDBType reads type (movie when empty; all only where allowed).
func argTMDBType(args map[string]string, all bool) (string, error) {
	t := strings.ToLower(strings.TrimSpace(args["type"]))
	switch t {
	case "":
		return "movie", nil
	case "movie", "tv":
		return t, nil
	case "all":
		if all {
			return t, nil
		}
	}
	return "", fmt.Errorf("%w: type must be movie or tv", ErrBadArgs)
}

// argTMDBWindow reads window (day when empty).
func argTMDBWindow(args map[string]string) (string, error) {
	switch w := strings.ToLower(strings.TrimSpace(args["window"])); w {
	case "":
		return "day", nil
	case "day", "week":
		return w, nil
	}
	return "", fmt.Errorf("%w: window must be day or week", ErrBadArgs)
}

func checkTMDBSearch(args map[string]string) error {
	if _, err := argTMDBQuery(args); err != nil {
		return err
	}
	if _, err := argTMDBType(args, false); err != nil {
		return err
	}
	_, err := argInt(args, "limit", tmdbSearchDefault, 1, tmdbSearchMax)
	return err
}

func checkTMDBTrending(args map[string]string) error {
	if _, err := argTMDBType(args, true); err != nil {
		return err
	}
	_, err := argTMDBWindow(args)
	return err
}

// newTMDB builds the ops against base (the API, or a test server).
func newTMDB(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !tmdbKeyRe.MatchString(key) {
		return nil, errors.New("EXT_TMDB_API_KEY is not set or not a key")
	}
	t := &tmdb{key: key, base: base, hc: hc}
	return map[string]Operation{
		"search":   OperationFunc(t.search),
		"trending": OperationFunc(t.trending),
	}, nil
}

type tmdb struct {
	key  string
	base string
	hc   *http.Client
}

// tmdbItem is one movie or TV show of a results page (title / release_date
// for a movie, name / first_air_date for a show).
type tmdbItem struct {
	ID           int64    `json:"id"`
	MediaType    string   `json:"media_type"`
	Title        string   `json:"title"`
	Name         string   `json:"name"`
	ReleaseDate  string   `json:"release_date"`
	FirstAirDate string   `json:"first_air_date"`
	Overview     string   `json:"overview"`
	VoteAverage  *float64 `json:"vote_average"`
	PosterPath   string   `json:"poster_path"`
}

type tmdbPage struct {
	Results []tmdbItem `json:"results"`
}

// get calls path with q, the language and the key. A read access token
// (v4, a JWT with dots) goes in the Authorization header; a v3 api_key in
// the query.
func (t *tmdb) get(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("language", tmdbLanguage)
	bearer := strings.Contains(t.key, ".")
	if !bearer {
		q.Set("api_key", t.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("tmdb: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+t.key)
	}
	return sendJSON(ctx, t.hc, req, out)
}

func (t *tmdb) search(ctx context.Context, in Input) (Output, error) {
	q, err := argTMDBQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	typ, err := argTMDBType(in.Args, false)
	if err != nil {
		return Output{}, err
	}
	limit, err := argInt(in.Args, "limit", tmdbSearchDefault, 1, tmdbSearchMax)
	if err != nil {
		return Output{}, err
	}
	var p tmdbPage
	if err := t.get(ctx, "/3/search/"+typ, url.Values{"query": {q}, "page": {"1"}, "include_adult": {"false"}}, &p); err != nil {
		return Output{}, err
	}
	return tmdbRows(p.Results, typ, limit), nil
}

func (t *tmdb) trending(ctx context.Context, in Input) (Output, error) {
	typ, err := argTMDBType(in.Args, true)
	if err != nil {
		return Output{}, err
	}
	window, err := argTMDBWindow(in.Args)
	if err != nil {
		return Output{}, err
	}
	var p tmdbPage
	if err := t.get(ctx, "/3/trending/"+typ+"/"+window, url.Values{}, &p); err != nil {
		return Output{}, err
	}
	return tmdbRows(p.Results, typ, tmdbTrendingMax), nil
}

// tmdbRows turns a results page into rows (at most limit). typ is the
// page's media type; "all" takes each item's own (people are left out).
func tmdbRows(items []tmdbItem, typ string, limit int) Output {
	out := Output{Rows: []Row{}, Cells: [][]string{}}
	for _, it := range items {
		if len(out.Rows) == limit {
			break
		}
		kind := typ
		if kind == "all" {
			kind = it.MediaType
		}
		title, release := it.Title, it.ReleaseDate
		if kind == "tv" {
			title, release = it.Name, it.FirstAirDate
		}
		title = tmdbClip(title, tmdbTitleRunes)
		if it.ID <= 0 || title == "" || (kind != "movie" && kind != "tv") {
			continue
		}
		var vote string
		if it.VoteAverage != nil && *it.VoteAverage > 0 {
			vote = strconv.FormatFloat(*it.VoteAverage, 'f', 1, 64)
		}
		var poster string
		if strings.HasPrefix(it.PosterPath, "/") && !strings.Contains(it.PosterPath[1:], "/") {
			poster = tmdbPosterURL + it.PosterPath
		}
		link := tmdbPageURL + kind + "/" + strconv.FormatInt(it.ID, 10)
		out.Rows = append(out.Rows, Row{
			Title:    title,
			Subtitle: tmdbJoin(release, tmdbClip(it.Overview, tmdbOverviewRunes)),
			Value:    vote,
			URL:      link,
		})
		out.Cells = append(out.Cells, []string{title, release, vote, link, poster})
	}
	return out
}

// tmdbJoin joins the parts that are not empty with " · ".
func tmdbJoin(parts ...string) string {
	keep := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, " · ")
}

// tmdbClip keeps one line of at most n runes, with "…" when it was cut.
func tmdbClip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
