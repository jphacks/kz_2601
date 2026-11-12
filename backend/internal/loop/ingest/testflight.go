package ingest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// TestFlightFeedback reads TestFlight beta feedback with the App Store Connect
// API (GET /v1/apps/{id}/betaFeedbackScreenshotSubmissions and
// .../betaFeedbackCrashSubmissions). Off unless LOOP_SOURCE_TESTFLIGHT=on;
// it uses the same LOOP_ASC_* key as AppStoreReviews.
//
// Kept: the tester's comment (masked by ingest), the build number, the OS
// major version, created date, and whether a screenshot exists. Never
// requested (sparse fieldsets): the tester's email and name, the tester
// relationship, device model, locale, time zone, the screenshots and the crash
// log (#367).
type TestFlightFeedback struct {
	Cfg AppStoreConfig
	// BaseURL: "" = https://api.appstoreconnect.apple.com
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
	// MaxPages bounds one run per kind (200 submissions a page). Default 5.
	MaxPages int
}

// EnvTestFlightOn turns the TestFlight source on.
const EnvTestFlightOn = "LOOP_SOURCE_TESTFLIGHT"

// ErrTestFlightOff is returned by TestFlightConfigFromEnv when the source is off.
var ErrTestFlightOff = errors.New("testflight feedback is off (" + EnvTestFlightOn + " != on)")

// TestFlightConfigFromEnv reads LOOP_SOURCE_TESTFLIGHT and the LOOP_ASC_* key.
func TestFlightConfigFromEnv(getenv func(string) string) (AppStoreConfig, error) {
	if getenv(EnvTestFlightOn) != "on" {
		return AppStoreConfig{}, ErrTestFlightOff
	}
	return ascConfigFromEnv(getenv, EnvTestFlightOn)
}

// Source implements Fetcher.
func (TestFlightFeedback) Source() loop.Source { return loop.SourceTestFlight }

// tfKind is one of the two betaFeedback submission types.
type tfKind struct {
	typ    string // JSON:API type and path segment
	kind   string // Raw.Kind
	prefix string // SourceID prefix (the two id spaces are separate)
	shot   bool
	// empty is the fixed text when the tester wrote nothing ("" = held as empty).
	empty string
}

var tfKinds = []tfKind{
	{typ: "betaFeedbackScreenshotSubmissions", kind: "testflight-screenshot", prefix: "s:", shot: true},
	{typ: "betaFeedbackCrashSubmissions", kind: "testflight-crash", prefix: "c:", empty: "TestFlight でクラッシュ(テスターのコメントなし)"},
}

// tfFields is what we ask for: no email, no tester, no device details.
const tfFields = "createdDate,comment,osVersion,build"

type tfPage struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Comment     string    `json:"comment"`
			OSVersion   string    `json:"osVersion"`
			CreatedDate time.Time `json:"createdDate"`
		} `json:"attributes"`
		Relationships struct {
			Build struct {
				Data *struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"build"`
		} `json:"relationships"`
	} `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Version string `json:"version"`
		} `json:"attributes"`
	} `json:"included"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// Fetch implements Fetcher: both kinds, newest first, stopping at the first
// submission older than since.
func (t TestFlightFeedback) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	base := t.BaseURL
	if base == "" {
		base = "https://api.appstoreconnect.apple.com"
	}
	hc := t.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	pages := t.MaxPages
	if pages <= 0 {
		pages = 5
	}
	out := []Raw{}
	for _, k := range tfKinds {
		q := url.Values{}
		q.Set("fields["+k.typ+"]", tfFields)
		q.Set("include", "build")
		q.Set("fields[builds]", "version")
		q.Set("sort", "-createdDate")
		q.Set("limit", "200")
		next := base + "/v1/apps/" + url.PathEscape(t.Cfg.AppID) + "/" + k.typ + "?" + q.Encode()
	pageLoop:
		for p := 0; p < pages && next != ""; p++ {
			if !strings.HasPrefix(next, base+"/") {
				return nil, errors.New("testflight: next link points elsewhere")
			}
			var page tfPage
			if err := ascGet(ctx, hc, t.Cfg, t.Now, next, "testflight", &page); err != nil {
				return nil, err
			}
			builds := map[string]string{}
			for _, in := range page.Included {
				if in.Type == "builds" {
					builds[in.ID] = in.Attributes.Version
				}
			}
			for _, d := range page.Data {
				at := d.Attributes
				if at.CreatedDate.Before(since) {
					break pageLoop
				}
				r := Raw{Source: loop.SourceTestFlight, SourceID: k.prefix + d.ID, Text: strings.TrimSpace(at.Comment),
					Kind: k.kind, HasScreenshot: k.shot, CreatedAt: at.CreatedDate}
				if r.Text == "" {
					r.Text = k.empty
				}
				if at.OSVersion != "" {
					r.OSMajor = OSMajor(at.OSVersion)
				}
				if b := d.Relationships.Build.Data; b != nil && builds[b.ID] != "" {
					r.AppVersion = "build " + builds[b.ID]
				}
				out = append(out, r)
			}
			next = page.Links.Next
		}
	}
	return out, nil
}
