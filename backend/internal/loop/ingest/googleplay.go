package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// Google Play reviews (#2147): the parser of one Play Developer API
// reviews.list page. The reader (auth, paging, LOOP_SOURCE_GOOGLEPLAY) is
// GooglePlayReviews in sources.go; nothing here calls the API.
//
// Kept: the review's text, stars, app version, device, Android API level,
// language, when it was last changed, and whether the developer replied.
// Dropped: the author's name and the device metadata (#367). The reply's
// text is never read: only that there is one.

// SourceGooglePlay is where a Google Play review came from. It stays here
// until the reader is wired in (loop.Source, the draft's source list).
const SourceGooglePlay loop.Source = "google_play_review"

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
