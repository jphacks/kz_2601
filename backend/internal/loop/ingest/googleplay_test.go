package ingest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"google.golang.org/api/option"
)

// Hand-written fixtures in the shape of reviews.list. Not real data. The
// server returns the author name and the device even though we never ask for
// them, to prove they are dropped.
const gpPage1 = `{
  "reviews": [
    {"reviewId": "r-ja", "authorName": "山田太郎",
     "comments": [{"userComment": {"text": "保存できない\t記録の画面で保存ボタンを押しても何も起きず、入力した内容が消えてしまいます。連絡は taro@example.com まで", "lastModified": {"seconds": "1794301200"},
       "starRating": 2, "reviewerLanguage": "ja", "device": "akita", "androidOsVersion": 35, "appVersionCode": 120, "appVersionName": "1.4.0",
       "deviceMetadata": {"productName": "Pixel 8a", "manufacturer": "Google"}}}]},
    {"reviewId": "r-replied", "authorName": "Hanako",
     "comments": [
       {"userComment": {"text": "落ちる", "lastModified": {"seconds": "1794214800"}, "starRating": 1, "androidOsVersion": 34, "appVersionCode": 119}},
       {"developerComment": {"text": "ご不便をおかけしています。サポート窓口 support@example.com", "lastModified": {"seconds": "1794218400"}}}]},
    {"reviewId": "r-en", "authorName": "John",
     "comments": [{"userComment": {"text": "Love it, but sync is slow", "lastModified": {"seconds": "1794128400"}, "starRating": 4, "reviewerLanguage": "en"}}]}
  ],
  "tokenPagination": {"nextPageToken": "tok2"}
}`

const gpPage2 = `{
  "reviews": [
    {"reviewId": "r-old", "comments": [{"userComment": {"text": "old", "lastModified": {"seconds": "1793491200"}, "starRating": 3}}]}
  ]
}`

func TestGooglePlayReviews(t *testing.T) {
	ctx := context.Background()
	since := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/androidpublisher/v3/applications/jp.kz2601.metamon/reviews" {
			t.Errorf("path %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("fields") != playReviewFields || strings.Contains(q.Get("fields"), "authorName") || strings.Contains(q.Get("fields"), "device") ||
			q.Get("maxResults") != "100" || q.Has("translationLanguage") {
			t.Errorf("query %v", q)
		}
		switch q.Get("token") {
		case "":
			_, _ = fmt.Fprint(w, gpPage1)
		case "tok2":
			_, _ = fmt.Fprint(w, gpPage2)
		default:
			t.Errorf("token %q", q.Get("token"))
		}
	}))
	defer srv.Close()
	c, err := NewPlayReviewsClient(ctx, nil, option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	g, err := GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on", "GOOGLE_PLAY_PACKAGE_NAME": "jp.kz2601.metamon"}.get)
	if err != nil {
		t.Fatal(err)
	}
	g.Lister = c
	raws, err := g.Fetch(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(raws) != 3 {
		t.Fatalf("calls %d raws %+v", calls, raws)
	}
	ja, replied, en := raws[0], raws[1], raws[2]
	if ja.SourceID != "r-ja" || ja.Source != loop.SourceGooglePlayReview || ja.Text != "保存できない\n記録の画面で保存ボタンを押しても何も起きず、入力した内容が消えてしまいます。連絡は taro@example.com まで" ||
		ja.Rating != 2 || ja.Kind != "rating-2" || ja.AppVersion != "1.4.0" || ja.OSMajor != "Android API 35" ||
		!ja.CreatedAt.Equal(time.Unix(1794301200, 0)) {
		t.Fatalf("ja %+v", ja)
	}
	// a replied review is kept; the reply is never copied
	if replied.SourceID != "r-replied" || replied.Text != "落ちる" || replied.AppVersion != "build 119" {
		t.Fatalf("replied %+v", replied)
	}
	// other languages stay in the reviewer's words (no translation)
	if en.Text != "Love it, but sync is slow" || en.AppVersion != "" || en.OSMajor != "" {
		t.Fatalf("en %+v", en)
	}
	for _, r := range raws {
		if r.UID != "" || strings.Contains(r.Text+r.Kind+r.AppVersion+r.OSMajor, "山田") || strings.Contains(r.Text, "support@") ||
			strings.Contains(r.Text+r.OSMajor, "Pixel") || strings.Contains(r.OSMajor, "akita") {
			t.Fatalf("kept reviewer data or the reply: %+v", r)
		}
	}

	// masked on the way in; off the AI unless LOOP_AI_SOURCES has it
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	p := allowAll()
	p.AISources[loop.SourceGooglePlayReview] = true
	if it := Normalize(ja, p, now); strings.Contains(it.Text, "example.com") || it.MaskCounts == nil || !it.AIAllowed {
		t.Fatalf("not masked: %+v", it)
	}
	if it := Normalize(ja, PolicyFromEnv(func(string) string { return "" }), now); it.HoldReason != loop.HoldSource {
		t.Fatalf("google play reached the AI by default: %+v", it)
	}
}

type fakePlay struct {
	pages map[string]PlayReviewPage
	err   error
	calls int
}

func (f *fakePlay) ListReviews(_ context.Context, _, token string, _ int) (PlayReviewPage, error) {
	f.calls++
	if f.err != nil {
		return PlayReviewPage{}, f.err
	}
	return f.pages[token], nil
}

func TestGooglePlayPagesAndErrors(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	// a token that never ends stops at MaxPages; a review without an id is skipped
	loopy := &fakePlay{pages: map[string]PlayReviewPage{
		"":  {Reviews: []PlayReview{{ID: "a", Text: "x", Modified: at}, {Text: "no id", Modified: at}}, Next: "t"},
		"t": {Reviews: []PlayReview{{ID: "b", Text: "y", Modified: at}}, Next: "t"},
	}}
	raws, err := GooglePlayReviews{PackageName: "p", Lister: loopy, MaxPages: 3}.Fetch(ctx, time.Time{})
	if err != nil || loopy.calls != 3 || len(raws) != 3 {
		t.Fatalf("raws %d calls %d err %v", len(raws), loopy.calls, err)
	}
	boom := errors.New("google play: status 403")
	if _, err := (GooglePlayReviews{PackageName: "p", Lister: &fakePlay{err: boom}}).Fetch(ctx, time.Time{}); !errors.Is(err, boom) {
		t.Fatalf("error %v", err)
	}
	if _, err := (GooglePlayReviews{Lister: loopy}).Fetch(ctx, time.Time{}); err == nil {
		t.Fatal("no package accepted")
	}

	// the error keeps the status only, never the body
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error": {"code": 403, "message": "reviewer taro@example.com"}}`)
	}))
	defer denied.Close()
	c, err := NewPlayReviewsClient(ctx, nil, option.WithEndpoint(denied.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(denied.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListReviews(ctx, "p", "", 100); err == nil || err.Error() != "google play: status 403" {
		t.Fatalf("error %v", err)
	}
}

func TestGooglePlayConfigFromEnv(t *testing.T) {
	if _, err := GooglePlayConfigFromEnv(env{}.get); !errors.Is(err, ErrGooglePlayOff) {
		t.Fatalf("off: %v", err)
	}
	// the App Store switch does not turn Google Play on
	if _, err := GooglePlayConfigFromEnv(env{EnvAppStoreOn: "on", "GOOGLE_PLAY_PACKAGE_NAME": "p"}.get); !errors.Is(err, ErrGooglePlayOff) {
		t.Fatalf("appstore switch leaked: %v", err)
	}
	if _, err := GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on"}.get); err == nil || !strings.Contains(err.Error(), EnvGooglePlayPackage) {
		t.Fatalf("no package: %v", err)
	}
	g, err := GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on", EnvGooglePlayPackage: "loop.pkg", "GOOGLE_PLAY_PACKAGE_NAME": "app.pkg"}.get)
	if err != nil || g.PackageName != "loop.pkg" || g.CredentialsJSON != nil {
		t.Fatalf("cfg %+v %v", g, err)
	}
	if g.Source() != loop.SourceGooglePlayReview {
		t.Fatal(g.Source())
	}
	_, err = GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on", EnvGooglePlayPackage: "p", EnvGooglePlayKey: `{"type":"authorized_user","secret":"s3cr3t"}`}.get)
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Fatalf("bad key: %v", err)
	}
	g, err = GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on", EnvGooglePlayPackage: "p", EnvGooglePlayKey: `{"type":"service_account"}`}.get)
	if err != nil || len(g.CredentialsJSON) == 0 {
		t.Fatalf("key: %+v %v", g, err)
	}
}
