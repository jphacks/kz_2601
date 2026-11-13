package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/googleplay"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	androidpublisher "google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// GooglePlayReviews reads Google Play reviews with the Play Developer API
// (reviews.list, #2147). Off unless LOOP_SOURCE_GOOGLEPLAY=on. Play returns
// only the reviews created or changed in about the last week, so run it at
// least weekly.
//
// Kept: rating, the review text in the reviewer's language (masked by
// ingest), the app version, the Android API level, the date. Never requested
// (partial response): the reviewer's name, the device, its metadata and the
// developer's reply text (#367).
type GooglePlayReviews struct {
	PackageName string
	// CredentialsJSON is a service account key; empty = the job's ADC.
	CredentialsJSON []byte
	// Lister: nil = a Play Developer API client built on first use.
	Lister PlayReviewLister
	// MaxPages bounds one run (100 reviews a page). Default 10.
	MaxPages int
}

// Env names (the key comes from Secret Manager in Cloud Run; never a path).
const (
	EnvGooglePlayOn      = "LOOP_SOURCE_GOOGLEPLAY"
	EnvGooglePlayPackage = "LOOP_GP_PACKAGE_NAME"         // default GOOGLE_PLAY_PACKAGE_NAME
	EnvGooglePlayKey     = "LOOP_GP_SERVICE_ACCOUNT_JSON" // optional; "" = ADC
)

// ErrGooglePlayOff is returned by GooglePlayConfigFromEnv when the source is off.
var ErrGooglePlayOff = errors.New("google play reviews are off (" + EnvGooglePlayOn + " != on)")

// GooglePlayConfigFromEnv reads LOOP_SOURCE_GOOGLEPLAY and the LOOP_GP_* values.
func GooglePlayConfigFromEnv(getenv func(string) string) (GooglePlayReviews, error) {
	if getenv(EnvGooglePlayOn) != "on" {
		return GooglePlayReviews{}, ErrGooglePlayOff
	}
	pkg := strings.TrimSpace(getenv(EnvGooglePlayPackage))
	if pkg == "" {
		pkg = strings.TrimSpace(getenv("GOOGLE_PLAY_PACKAGE_NAME"))
	}
	if pkg == "" {
		return GooglePlayReviews{}, fmt.Errorf("%s=on needs %s (or GOOGLE_PLAY_PACKAGE_NAME)", EnvGooglePlayOn, EnvGooglePlayPackage)
	}
	g := GooglePlayReviews{PackageName: pkg}
	if key := strings.TrimSpace(getenv(EnvGooglePlayKey)); key != "" {
		if err := googleplay.CheckKey([]byte(key)); err != nil {
			return GooglePlayReviews{}, fmt.Errorf("%s: %w", EnvGooglePlayKey, err)
		}
		g.CredentialsJSON = []byte(key)
	}
	return g, nil
}

// PlayReview is the part of one review the loop keeps.
type PlayReview struct {
	ID          string
	Text        string
	Rating      int
	VersionName string
	VersionCode int64
	APILevel    int64
	Modified    time.Time
	Replied     bool
}

// PlayReviewPage is one reviews.list page.
type PlayReviewPage struct {
	Reviews []PlayReview
	Next    string
}

// PlayReviewLister is reviews.list (PlayReviewsClient; a fake in tests).
type PlayReviewLister interface {
	ListReviews(ctx context.Context, packageName, token string, max int) (PlayReviewPage, error)
}

// PlayReviewsClient implements PlayReviewLister with androidpublisher v3.
type PlayReviewsClient struct{ Svc *androidpublisher.Service }

// NewPlayReviewsClient builds a client from a service account key, or from
// ADC when credentialsJSON is empty. opts come last (tests: an endpoint).
func NewPlayReviewsClient(ctx context.Context, credentialsJSON []byte, opts ...option.ClientOption) (*PlayReviewsClient, error) {
	all := []option.ClientOption{option.WithScopes(androidpublisher.AndroidpublisherScope)}
	if len(credentialsJSON) > 0 {
		all = append(all, option.WithAuthCredentialsJSON(option.ServiceAccount, credentialsJSON))
	}
	svc, err := androidpublisher.NewService(ctx, append(all, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("google play: init: %w", err)
	}
	return &PlayReviewsClient{Svc: svc}, nil
}

// playReviewFields is the partial response: no authorName, device, deviceMetadata
// or the reply's text.
const playReviewFields = "reviews(reviewId,comments(userComment(text,lastModified,starRating,androidOsVersion,appVersionCode,appVersionName),developerComment(lastModified))),tokenPagination(nextPageToken)"

// ListReviews implements PlayReviewLister. The text is the original (no
// translationLanguage). Errors keep the status only.
func (c *PlayReviewsClient) ListReviews(ctx context.Context, packageName, token string, max int) (PlayReviewPage, error) {
	call := c.Svc.Reviews.List(packageName).MaxResults(int64(max)).Fields(googleapi.Field(playReviewFields)).Context(ctx)
	if token != "" {
		call = call.Token(token)
	}
	resp, err := call.Do()
	if err != nil {
		var ge *googleapi.Error
		if errors.As(err, &ge) {
			return PlayReviewPage{}, fmt.Errorf("google play: status %d", ge.Code)
		}
		return PlayReviewPage{}, fmt.Errorf("google play: %w", err)
	}
	page := PlayReviewPage{}
	if tp := resp.TokenPagination; tp != nil {
		page.Next = tp.NextPageToken
	}
	for _, r := range resp.Reviews {
		pr := PlayReview{ID: r.ReviewId}
		for _, cm := range r.Comments {
			if cm == nil {
				continue
			}
			if u := cm.UserComment; u != nil && pr.Text == "" {
				pr.Text, pr.Rating = u.Text, int(u.StarRating)
				pr.VersionName, pr.VersionCode, pr.APILevel = u.AppVersionName, u.AppVersionCode, u.AndroidOsVersion
				if u.LastModified != nil {
					pr.Modified = time.Unix(u.LastModified.Seconds, u.LastModified.Nanos).UTC()
				}
			}
			if cm.DeveloperComment != nil {
				pr.Replied = true
			}
		}
		page.Reviews = append(page.Reviews, pr)
	}
	return page, nil
}

// Source implements Fetcher.
func (GooglePlayReviews) Source() loop.Source { return loop.SourceGooglePlayReview }

// Fetch implements Fetcher: every page up to MaxPages, keeping the reviews
// last changed at or after since. Replied reviews are kept (a reply is not a
// fix); the reply itself is never read. An edited review keeps its first
// copy (the item id is the review id).
func (g GooglePlayReviews) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	if g.PackageName == "" {
		return nil, errors.New("google play: no package name")
	}
	lister := g.Lister
	if lister == nil {
		c, err := NewPlayReviewsClient(ctx, g.CredentialsJSON)
		if err != nil {
			return nil, err
		}
		lister = c
	}
	pages := g.MaxPages
	if pages <= 0 {
		pages = 10
	}
	out := []Raw{}
	token := ""
	for p := 0; p < pages; p++ {
		page, err := lister.ListReviews(ctx, g.PackageName, token, 100)
		if err != nil {
			return nil, err
		}
		for _, r := range page.Reviews {
			if r.ID == "" || r.Modified.Before(since) {
				continue
			}
			out = append(out, PlayRaw(r))
		}
		if page.Next == "" {
			break
		}
		token = page.Next
	}
	return out, nil
}

// PlayRaw is the inbox form of one review. Play joins a title and a body
// with a tab.
func PlayRaw(r PlayReview) Raw {
	raw := Raw{Source: loop.SourceGooglePlayReview, SourceID: r.ID,
		Text: strings.TrimSpace(strings.ReplaceAll(r.Text, "\t", "\n")),
		Kind: fmt.Sprintf("rating-%d", r.Rating), Rating: r.Rating, CreatedAt: r.Modified}
	switch {
	case r.VersionName != "":
		raw.AppVersion = r.VersionName
	case r.VersionCode > 0:
		raw.AppVersion = fmt.Sprintf("build %d", r.VersionCode)
	}
	if r.APILevel > 0 {
		raw.OSMajor = fmt.Sprintf("Android API %d", r.APILevel)
	}
	return raw
}

// Google Play reviews (#2147): the parser of one Play Developer API
// reviews.list page as raw JSON (e.g. a saved answer). The API reader is
// GooglePlayReviews above; nothing here calls the API.
//
// Kept: the review's text, stars, app version, device, Android API level,
// language, when it was last changed, and whether the developer replied.
// Dropped: the author's name and the device metadata (#367). The reply's
// text is never read: only that there is one.

// SourceGooglePlay is where a Google Play review came from.
const SourceGooglePlay = loop.SourceGooglePlayReview

// GooglePlayReview is one review in the common shape.
type GooglePlayReview struct {
	ID         string
	Text       string
	Rating     int
	AppVersion string // appVersionName ("" when Play does not know it)
	Device     string // Play's device code name ("a14")
	AndroidAPI int    // androidOsVersion (API level, 0 = unknown)
	Language   string // reviewerLanguage ("ja_JP", "en")
	// Replied: the developer answered. The answer is not part of Text.
	Replied   bool
	CreatedAt time.Time // the review's lastModified (Play has no created time)
}

// GooglePlayPage is one reviews.list answer.
type GooglePlayPage struct {
	Reviews       []GooglePlayReview
	NextPageToken string
}

// playSeconds reads a Timestamp's seconds, given as a string or a number.
type playSeconds int64

func (s *playSeconds) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	n, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("google play: bad seconds %q", b)
	}
	*s = playSeconds(n)
	return nil
}

type playTimestamp struct {
	Seconds playSeconds `json:"seconds"`
	Nanos   int32       `json:"nanos"`
}

func (t *playTimestamp) time() time.Time {
	if t == nil || (t.Seconds == 0 && t.Nanos == 0) {
		return time.Time{}
	}
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).UTC()
}

type playAnswer struct {
	Reviews []struct {
		ReviewID string `json:"reviewId"`
		Comments []struct {
			UserComment *struct {
				Text             string         `json:"text"`
				LastModified     *playTimestamp `json:"lastModified"`
				StarRating       int            `json:"starRating"`
				ReviewerLanguage string         `json:"reviewerLanguage"`
				Device           string         `json:"device"`
				AndroidOSVersion int            `json:"androidOsVersion"`
				AppVersionName   string         `json:"appVersionName"`
			} `json:"userComment"`
			DeveloperComment *struct {
				LastModified *playTimestamp `json:"lastModified"`
			} `json:"developerComment"`
		} `json:"comments"`
	} `json:"reviews"`
	TokenPagination struct {
		NextPageToken string `json:"nextPageToken"`
	} `json:"tokenPagination"`
}

// ParseGooglePlayReviews reads one reviews.list page. A review without the
// person's text (a reply only) is skipped; stars outside 1-5 read as 0.
func ParseGooglePlayReviews(r io.Reader) (GooglePlayPage, error) {
	var a playAnswer
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&a); err != nil {
		return GooglePlayPage{}, fmt.Errorf("google play: decode: %w", err)
	}
	out := GooglePlayPage{Reviews: []GooglePlayReview{}, NextPageToken: a.TokenPagination.NextPageToken}
	for _, rv := range a.Reviews {
		var g GooglePlayReview
		var found bool
		for _, c := range rv.Comments {
			if c.DeveloperComment != nil {
				g.Replied = true
			}
			u := c.UserComment
			if u == nil || found {
				continue
			}
			found = true
			g.Text = strings.TrimSpace(u.Text)
			g.Rating = u.StarRating
			if g.Rating < 1 || g.Rating > 5 {
				g.Rating = 0
			}
			g.AppVersion = strings.TrimSpace(u.AppVersionName)
			g.Device = strings.TrimSpace(u.Device)
			g.AndroidAPI = max(u.AndroidOSVersion, 0)
			g.Language = strings.TrimSpace(u.ReviewerLanguage)
			g.CreatedAt = u.LastModified.time()
		}
		if !found || g.Text == "" {
			continue
		}
		if rv.ReviewID == "" {
			return GooglePlayPage{}, errors.New("google play: a review without reviewId")
		}
		g.ID = rv.ReviewID
		out.Reviews = append(out.Reviews, g)
	}
	return out, nil
}

// Raw is the review as an inbox item. Device and language stay on
// GooglePlayReview: Raw has no field for them yet.
func (g GooglePlayReview) Raw() Raw {
	r := Raw{Source: SourceGooglePlay, SourceID: g.ID, Text: g.Text, Rating: g.Rating,
		AppVersion: g.AppVersion, CreatedAt: g.CreatedAt, Kind: "rating-" + strconv.Itoa(g.Rating)}
	if g.AndroidAPI > 0 {
		r.OSMajor = "Android API " + strconv.Itoa(g.AndroidAPI)
	}
	return r
}
