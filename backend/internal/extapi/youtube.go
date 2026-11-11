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

// YouTube Data API v3 (#1699): a channel's public numbers and its latest
// uploads, read with the operator's key (public data only; a person's own
// YouTube Analytics needs OAuth and is a separate provider).
//
//	channelStats(channel)          Item: channel name / videos・views / subscribers / channel link
//	latestVideos(channel, limit)   Rows: public uploads, newest first (limit 1-20, default 5)
//
// channel is a channel id (UC + 22 chars) or an @handle. Quota (units per
// call): channelStats 1 (channels.list), latestVideos 2 (channels.list for
// the uploads playlist + playlistItems.list). search.list (100 units) is
// never used.
//
// Terms (YouTube API Services Terms of Service and Developer Policies):
//   - YouTube must be shown as the source (YouTubeAttribution, Branding
//     Guidelines), and the numbers are shown as the API gives them: no
//     recalculated or substituted values; a hidden subscriber count stays
//     hidden.
//   - stored API data must be kept current; statistics of a channel the
//     person has not authorised may be kept at most 30 days. The shared
//     cache keeps answers 1 h / 30 min only (far inside the limit).
//   - the app shows a link to the YouTube Terms of Service and the privacy
//     policy says YouTube API Services are used and links to the Google
//     Privacy Policy (docs/backend/features/external-apis.md).
//
// The key (EXT_YOUTUBE_API_KEY, Secret Manager) goes in the
// X-Goog-Api-Key header, never in a URL or a log.
const (
	YouTube = "youtube"
	// YouTubeAttribution is the source shown with the numbers and rows.
	YouTubeAttribution = "YouTube"

	youTubeHost = "https://www.googleapis.com"
)

var (
	ytKeyRe       = regexp.MustCompile(`^[A-Za-z0-9_-]{20,64}$`)
	ytChannelIDRe = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	ytHandleRe    = regexp.MustCompile(`^@[\p{L}\p{N}._-]{3,30}$`)
	ytUploadsRe   = regexp.MustCompile(`^UU[A-Za-z0-9_-]{22}$`)
	ytVideoIDRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

func init() {
	register(Provider{ID: YouTube, Billing: BillingFree,
		Ops: []OpSpec{
			{Name: "channelStats", Args: []string{"channel"}, Check: checkYTChannel, CacheTTL: time.Hour},
			{Name: "latestVideos", Args: []string{"channel", "limit"}, Check: checkYTLatest, CacheTTL: 30 * time.Minute},
		},
		Hosts: []string{youTubeHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newYouTube(sec.APIKey, youTubeHost, NewHTTPClient([]string{youTubeHost}, DefaultTimeout))
		}})
}

// ytChannelArg reads channel: the query parameter that finds it (id or
// forHandle) and its value.
func ytChannelArg(args map[string]string) (param, value string, err error) {
	c := strings.TrimSpace(args["channel"])
	switch {
	case ytChannelIDRe.MatchString(c):
		return "id", c, nil
	case ytHandleRe.MatchString(c):
		return "forHandle", c, nil
	}
	return "", "", fmt.Errorf("%w: channel must be a channel id (UC…) or an @handle", ErrUnknown)
}

// checkYTChannel rejects a bad channel at CheckArgs, before any limit.
func checkYTChannel(args map[string]string) error {
	_, _, err := ytChannelArg(args)
	return err
}

func checkYTLatest(args map[string]string) error {
	if err := checkYTChannel(args); err != nil {
		return err
	}
	_, err := argInt(args, "limit", 5, 1, 20)
	return err
}

// newYouTube builds the ops against base (the API, or a test server).
func newYouTube(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !ytKeyRe.MatchString(key) {
		return nil, errors.New("EXT_YOUTUBE_API_KEY is not set or not a key")
	}
	y := &youTube{key: key, base: base, hc: hc}
	return map[string]Operation{
		"channelStats": OperationFunc(y.channelStats),
		"latestVideos": OperationFunc(y.latestVideos),
	}, nil
}

type youTube struct {
	key  string
	base string
	hc   *http.Client
}

type ytChannels struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
		Statistics struct {
			ViewCount             string `json:"viewCount"`
			SubscriberCount       string `json:"subscriberCount"`
			HiddenSubscriberCount bool   `json:"hiddenSubscriberCount"`
			VideoCount            string `json:"videoCount"`
		} `json:"statistics"`
		ContentDetails struct {
			RelatedPlaylists struct {
				Uploads string `json:"uploads"`
			} `json:"relatedPlaylists"`
		} `json:"contentDetails"`
	} `json:"items"`
}

type ytPlaylistItems struct {
	Items []struct {
		Snippet struct {
			Title string `json:"title"`
		} `json:"snippet"`
		ContentDetails struct {
			VideoID          string `json:"videoId"`
			VideoPublishedAt string `json:"videoPublishedAt"`
		} `json:"contentDetails"`
		Status struct {
			PrivacyStatus string `json:"privacyStatus"`
		} `json:"status"`
	} `json:"items"`
}

func (y *youTube) get(ctx context.Context, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, y.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("youtube: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Goog-Api-Key", y.key)
	return sendJSON(ctx, y.hc, req, out)
}

// channel reads the one channel of args with parts.
func (y *youTube) channel(ctx context.Context, args map[string]string, parts string) (*ytChannels, error) {
	param, value, err := ytChannelArg(args)
	if err != nil {
		return nil, err
	}
	var c ytChannels
	if err := y.get(ctx, "/youtube/v3/channels", url.Values{"part": {parts}, param: {value}, "maxResults": {"1"}}, &c); err != nil {
		return nil, err
	}
	if len(c.Items) == 0 || !ytChannelIDRe.MatchString(c.Items[0].ID) {
		// no such channel: the caller's argument, not the provider's fault
		return nil, fmt.Errorf("%w: no such channel", ErrUnknown)
	}
	return &c, nil
}

func (y *youTube) channelStats(ctx context.Context, in Input) (Output, error) {
	c, err := y.channel(ctx, in.Args, "snippet,statistics")
	if err != nil {
		return Output{}, err
	}
	ch := c.Items[0]
	st := ch.Statistics
	subs := "登録者 非公開"
	if !st.HiddenSubscriberCount {
		n, ok := ytCount(st.SubscriberCount)
		if !ok {
			return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("youtube: no subscriber count")}
		}
		subs = "登録者 " + n + " 人"
	}
	var sub []string
	if n, ok := ytCount(st.VideoCount); ok {
		sub = append(sub, "動画 "+n+" 本")
	}
	if n, ok := ytCount(st.ViewCount); ok {
		sub = append(sub, "再生 "+n+" 回")
	}
	return Output{Item: &Row{
		Title:    ytText(ch.Snippet.Title, 100),
		Subtitle: strings.Join(sub, "・"),
		Value:    subs,
		URL:      "https://www.youtube.com/channel/" + ch.ID,
	}}, nil
}

func (y *youTube) latestVideos(ctx context.Context, in Input) (Output, error) {
	limit, err := argInt(in.Args, "limit", 5, 1, 20)
	if err != nil {
		return Output{}, err
	}
	c, err := y.channel(ctx, in.Args, "contentDetails")
	if err != nil {
		return Output{}, err
	}
	uploads := c.Items[0].ContentDetails.RelatedPlaylists.Uploads
	if !ytUploadsRe.MatchString(uploads) {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("youtube: no uploads playlist")}
	}
	var p ytPlaylistItems
	// a few more than limit: private / deleted uploads are skipped
	q := url.Values{"part": {"snippet,contentDetails,status"}, "playlistId": {uploads}, "maxResults": {strconv.Itoa(min(limit+5, 50))}}
	if err := y.get(ctx, "/youtube/v3/playlistItems", q, &p); err != nil {
		var pe *ProviderError
		if errors.As(err, &pe) && pe.Status == http.StatusNotFound {
			return Output{Rows: []Row{}}, nil // a channel without uploads
		}
		return Output{}, err
	}
	rows := make([]Row, 0, limit)
	for _, it := range p.Items {
		if len(rows) == limit {
			break
		}
		id := it.ContentDetails.VideoID
		if it.Status.PrivacyStatus != "public" || !ytVideoIDRe.MatchString(id) {
			continue
		}
		r := Row{Title: ytText(it.Snippet.Title, 100), URL: "https://www.youtube.com/watch?v=" + id}
		if t, err := time.Parse(time.RFC3339, it.ContentDetails.VideoPublishedAt); err == nil {
			r.Subtitle = t.In(ytJST).Format("2006/1/2 15:04")
		}
		rows = append(rows, r)
	}
	return Output{Rows: rows}, nil
}

var ytJST = time.FixedZone("JST", 9*60*60)

// ytCount writes a count as the API gave it, with thousands separators
// (never rounded or recalculated: Developer Policies III.E.4.h).
func ytCount(s string) (string, bool) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return "", false
	}
	d := strconv.FormatUint(n, 10)
	var b strings.Builder
	for i, r := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String(), true
}

// ytText trims s to at most n runes.
func ytText(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return s
}
