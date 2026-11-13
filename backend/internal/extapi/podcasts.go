package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Podcasts (#1703): find shows with Apple's iTunes Search API
// (https://performance-partners.apple.com/search-api). Free, no key, no
// OAuth: one call is one `runs`. Only itunes.apple.com is reached.
//
//	searchPodcasts(term, limit?)        Rows: one show per result   EXT_PODCASTS_ENABLED=1
//	newEpisodes(collectionId, limit?)   Rows: a show's episodes, newest first
//
// term is 1-100 characters on one line; limit 1-20 (default 10). The store
// is Japan (country=JP, lang=ja_jp).
//
// A row: title the show, subtitle the author, value whether the show has an
// RSS feed ("RSS あり" / "RSS なし"), url the show's Apple Podcasts page
// (the store link, query dropped). Cells (server-side only) are title,
// author, feed (podcastsFeedYes / podcastsFeedNo), artwork URL, store URL
// and the collection id (what a later lookup takes).
//
// newEpisodes takes the collection id searchPodcasts keeps in its cells
// (digits only) and reads the iTunes Lookup API (/lookup?entity=
// podcastEpisode). limit is clamped to 1-20 (default 10). A row: title the
// episode, subtitle the show, value the release date (JST, 2006/1/2), url
// the episode's Apple Podcasts link without its query. The audio URL
// (episodeUrl / previewUrl) is not even decoded, so it is never returned or
// kept (the cache holds only the rows).
//
// The feed URL itself is never returned: reading episodes from a feed waits
// on #1702 (fetching a URL the app or person gives), and a URL could not be
// passed back as an argument anyway (CheckArgs refuses URLs).
//
// Terms (Search API page, read 2026-11-12):
//   - about 20 calls a minute (per caller); large users should cache. The
//     shared cache keeps an answer 6 hours.
//   - the content may only promote the store's content: each row links to
//     the show's store page, and the app shows PodcastsAttribution with the
//     rows. Audio is never fetched or kept on the server.
const (
	Podcasts = "podcasts"
	// PodcastsAttribution is the source shown with the rows.
	PodcastsAttribution = "provided courtesy of iTunes"

	podcastsFeedYes = "RSS あり"
	podcastsFeedNo  = "RSS なし"

	podcastsMaxTerm  = 100
	podcastsMaxTitle = 100
	podcastsMaxID    = 15
)

var podcastsHosts = []string{"https://itunes.apple.com"}

func init() {
	register(Provider{ID: Podcasts, Billing: BillingFree, Hosts: podcastsHosts,
		Ops: []OpSpec{
			{Name: "searchPodcasts", Args: []string{"term", "limit"}, Check: checkPodcastSearch, CacheTTL: 6 * time.Hour},
			{Name: "newEpisodes", Args: []string{"collectionId", "limit"}, Check: checkPodcastEpisodes, CacheTTL: 6 * time.Hour},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newPodcasts(podcastsHosts[0], NewHTTPClient(podcastsHosts, DefaultTimeout)), nil
		}})
}

// checkPodcastSearch: term 1-100 characters, one line, no control
// characters; limit 1-20.
func checkPodcastSearch(a map[string]string) error {
	t := strings.TrimSpace(a["term"])
	if t == "" {
		return errors.New("term is required")
	}
	if len([]rune(t)) > podcastsMaxTerm {
		return fmt.Errorf("term takes at most %d characters", podcastsMaxTerm)
	}
	if strings.IndexFunc(t, unicode.IsControl) >= 0 {
		return errors.New("term must be one line")
	}
	_, err := argInt(a, "limit", 10, 1, 20)
	return err
}

// checkPodcastEpisodes: collectionId 1-15 digits (nothing else); limit an
// integer (clamped to 1-20).
func checkPodcastEpisodes(a map[string]string) error {
	if _, err := podcastsID(a["collectionId"]); err != nil {
		return err
	}
	_, err := podcastsLimit(a)
	return err
}

// podcastsID is the collection id: 1-15 ASCII digits, not all zero.
func podcastsID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > podcastsMaxID || strings.TrimLeft(s, "0123456789") != "" || strings.Trim(s, "0") == "" {
		return "", fmt.Errorf("collectionId must be 1-%d digits", podcastsMaxID)
	}
	return s, nil
}

// podcastsLimit is newEpisodes' limit: an integer, clamped to 1-20.
func podcastsLimit(a map[string]string) (int, error) {
	s := strings.TrimSpace(a["limit"])
	if s == "" {
		return 10, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%w: limit must be an integer", ErrUnknown)
	}
	return min(max(n, 1), 20), nil
}

// newPodcasts builds the ops against base (tests pass an httptest server).
func newPodcasts(base string, hc *http.Client) map[string]Operation {
	p := &podcasts{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"searchPodcasts": OperationFunc(p.search),
		"newEpisodes":    OperationFunc(p.episodes),
	}
}

type podcasts struct {
	base string
	hc   *http.Client
}

// podcastsAnswer is the Search API's JSON (only what becomes rows).
type podcastsAnswer struct {
	Results []struct {
		Kind              string `json:"kind"`
		CollectionID      int64  `json:"collectionId"`
		CollectionName    string `json:"collectionName"`
		ArtistName        string `json:"artistName"`
		FeedURL           string `json:"feedUrl"`
		CollectionViewURL string `json:"collectionViewUrl"`
		ArtworkURL600     string `json:"artworkUrl600"`
		ArtworkURL100     string `json:"artworkUrl100"`
	} `json:"results"`
}

func (p *podcasts) search(ctx context.Context, in Input) (Output, error) {
	limit, err := argInt(in.Args, "limit", 10, 1, 20)
	if err != nil {
		return Output{}, err
	}
	q := url.Values{
		"term":    {strings.TrimSpace(in.Args["term"])},
		"media":   {"podcast"},
		"entity":  {"podcast"},
		"country": {"JP"},
		"lang":    {"ja_jp"},
		"limit":   {strconv.Itoa(limit)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/search?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("podcasts: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	var a podcastsAnswer
	if err := sendJSON(ctx, p.hc, req, &a); err != nil {
		return Output{}, err
	}
	rows, cells := []Row{}, [][]string{}
	for _, r := range a.Results {
		if len(rows) == limit {
			break
		}
		title := ytText(r.CollectionName, podcastsMaxTitle)
		store := podcastsLink(r.CollectionViewURL, "podcasts.apple.com", "itunes.apple.com")
		if (r.Kind != "" && r.Kind != "podcast") || title == "" || store == "" || r.CollectionID <= 0 {
			// a row always links to its store page (the terms: promote the store)
			continue
		}
		feed := podcastsFeedNo
		if u, err := url.Parse(r.FeedURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			feed = podcastsFeedYes
		}
		art := podcastsLink(r.ArtworkURL600, ".mzstatic.com")
		if art == "" {
			art = podcastsLink(r.ArtworkURL100, ".mzstatic.com")
		}
		author := ytText(r.ArtistName, podcastsMaxTitle)
		rows = append(rows, Row{Title: title, Subtitle: author, Value: feed, URL: store})
		cells = append(cells, []string{title, author, feed, art, store, strconv.FormatInt(r.CollectionID, 10)})
	}
	return Output{Rows: rows, Cells: cells}, nil
}

// podcastsEpisodes is the Lookup API's JSON. Only these fields are decoded:
// episodeUrl / previewUrl (the audio) are left out on purpose.
type podcastsEpisodes struct {
	Results []struct {
		WrapperType    string `json:"wrapperType"`
		Kind           string `json:"kind"`
		TrackName      string `json:"trackName"`
		CollectionName string `json:"collectionName"`
		ReleaseDate    string `json:"releaseDate"`
		TrackViewURL   string `json:"trackViewUrl"`
	} `json:"results"`
}

func (p *podcasts) episodes(ctx context.Context, in Input) (Output, error) {
	id, err := podcastsID(in.Args["collectionId"])
	if err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrUnknown, err)
	}
	limit, err := podcastsLimit(in.Args)
	if err != nil {
		return Output{}, err
	}
	q := url.Values{
		"id":      {id},
		"entity":  {"podcastEpisode"},
		"country": {"JP"},
		"limit":   {strconv.Itoa(limit)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/lookup?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("podcasts: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	var a podcastsEpisodes
	if err := sendJSON(ctx, p.hc, req, &a); err != nil {
		return Output{}, err
	}
	type ep struct {
		row Row
		at  time.Time
	}
	eps := []ep{}
	for _, r := range a.Results {
		// the first result is the show itself
		if r.WrapperType != "podcastEpisode" && r.Kind != "podcast-episode" {
			continue
		}
		title := ytText(r.TrackName, podcastsMaxTitle)
		link := podcastsLink(r.TrackViewURL, "podcasts.apple.com", "itunes.apple.com")
		if title == "" || link == "" {
			continue // a row always links to the store
		}
		e := ep{row: Row{Title: title, Subtitle: ytText(r.CollectionName, podcastsMaxTitle), URL: link}}
		if t, err := time.Parse(time.RFC3339, r.ReleaseDate); err == nil {
			e.at, e.row.Value = t, t.In(ytJST).Format("2006/1/2")
		}
		eps = append(eps, e)
	}
	slices.SortStableFunc(eps, func(x, y ep) int { return y.at.Compare(x.at) })
	rows, cells := []Row{}, [][]string{}
	for _, e := range eps {
		if len(rows) == limit {
			break
		}
		rows = append(rows, e.row)
		at := ""
		if !e.at.IsZero() {
			at = e.at.UTC().Format(time.RFC3339)
		}
		cells = append(cells, []string{e.row.Title, e.row.Subtitle, at, e.row.URL})
	}
	return Output{Rows: rows, Cells: cells}, nil
}

// podcastsLink keeps an https link on one of hosts (a leading "." matches
// any subdomain), without its query or fragment; anything else is "".
func podcastsLink(s string, hosts ...string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	for _, want := range hosts {
		if h == want || (strings.HasPrefix(want, ".") && strings.HasSuffix(h, want)) {
			return (&url.URL{Scheme: "https", Host: h, Path: u.Path}).String()
		}
	}
	return ""
}
