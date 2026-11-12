package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Podcasts (#1703): find shows with Apple's iTunes Search API
// (https://performance-partners.apple.com/search-api). Free, no key, no
// OAuth: one call is one `runs`. Only itunes.apple.com is reached.
//
//	searchPodcasts(term, limit?)   Rows: one show per result   EXT_PODCASTS_ENABLED=1
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
)

var podcastsHosts = []string{"https://itunes.apple.com"}

func init() {
	register(Provider{ID: Podcasts, Billing: BillingFree, Hosts: podcastsHosts,
		Ops: []OpSpec{
			{Name: "searchPodcasts", Args: []string{"term", "limit"}, Check: checkPodcastSearch, CacheTTL: 6 * time.Hour},
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

// newPodcasts builds the op against base (tests pass an httptest server).
func newPodcasts(base string, hc *http.Client) map[string]Operation {
	p := &podcasts{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{"searchPodcasts": OperationFunc(p.search)}
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
