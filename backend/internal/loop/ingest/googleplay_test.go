package ingest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func readPlay(t *testing.T, name string) GooglePlayPage {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "googleplay", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	p, err := ParseGooglePlayReviews(f)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseGooglePlayReviews(t *testing.T) {
	for _, tc := range []struct {
		file string
		next string
		want []GooglePlayReview
	}{
		{"normal", "next-page-1", []GooglePlayReview{{
			ID: "gp:AOqpTOE-normal-1", Text: "予定のアプリを作ったら保存ボタンが押せませんでした。", Rating: 2,
			AppVersion: "1.2.0", Device: "a14", AndroidAPI: 34, Language: "ja_JP",
			CreatedAt: time.Date(2025, 11, 12, 0, 0, 0, 500000000, time.UTC),
		}}},
		{"replied", "", []GooglePlayReview{{
			ID: "gp:AOqpTOE-replied-1", Text: "声で直せるのが便利。たまに固まります。", Rating: 4,
			AppVersion: "1.1.3", Device: "akita", AndroidAPI: 35, Language: "ja", Replied: true,
			CreatedAt: time.Date(2025, 11, 11, 0, 0, 0, 0, time.UTC),
		}}},
		{"english", "", []GooglePlayReview{{
			ID: "gp:AOqpTOE-english-1", Text: "The voice edit is great, but the app crashes when I rotate the screen.", Rating: 3,
			AppVersion: "1.1.2", Device: "husky", AndroidAPI: 34, Language: "en",
			CreatedAt: time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
		}}},
		{"noversion", "", []GooglePlayReview{{
			ID: "gp:AOqpTOE-noversion-1", Text: "使い方がよく分からない", Rating: 1, Language: "ja_JP",
			CreatedAt: time.Date(2025, 11, 9, 0, 0, 0, 0, time.UTC),
		}}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			p := readPlay(t, tc.file)
			if p.NextPageToken != tc.next {
				t.Errorf("next %q", p.NextPageToken)
			}
			if !slices.Equal(p.Reviews, tc.want) {
				t.Fatalf("got  %+v\nwant %+v", p.Reviews, tc.want)
			}
		})
	}
}

// The reply's text and the author's name never reach the review.
func TestGooglePlayKeepsReplyAndAuthorOut(t *testing.T) {
	for _, name := range []string{"normal", "replied", "english", "noversion"} {
		for _, r := range readPlay(t, name).Reviews {
			for _, s := range []string{"ご報告ありがとうございます", "ありがとうございます。", "山田", "佐藤", "John", "Galaxy", "Samsung"} {
				if strings.Contains(r.Text, s) || strings.Contains(r.Device, s) {
					t.Errorf("%s: %q leaked into %+v", name, s, r)
				}
			}
		}
	}
}

func TestGooglePlayRaw(t *testing.T) {
	r := readPlay(t, "normal").Reviews[0].Raw()
	if r.Source != SourceGooglePlay || r.SourceID != "gp:AOqpTOE-normal-1" || r.Rating != 2 || r.Kind != "rating-2" ||
		r.AppVersion != "1.2.0" || r.OSMajor != "Android API 34" || r.UID != "" || r.Text != "予定のアプリを作ったら保存ボタンが押せませんでした。" ||
		!r.CreatedAt.Equal(time.Date(2025, 11, 12, 0, 0, 0, 500000000, time.UTC)) {
		t.Fatalf("%+v", r)
	}
	if r := readPlay(t, "noversion").Reviews[0].Raw(); r.AppVersion != "" || r.OSMajor != "" {
		t.Fatalf("%+v", r)
	}
}

func TestParseGooglePlayReviewsEdges(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		n          int
		ok         bool
	}{
		{"empty page", `{}`, 0, true},
		{"no reviews", `{"reviews": []}`, 0, true},
		{"blank text", `{"reviews":[{"reviewId":"a","comments":[{"userComment":{"text":"  \t "}}]}]}`, 0, true},
		{"stars out of range", `{"reviews":[{"reviewId":"a","comments":[{"userComment":{"text":"x","starRating":9}}]}]}`, 1, true},
		{"no id", `{"reviews":[{"comments":[{"userComment":{"text":"x"}}]}]}`, 0, false},
		{"bad seconds", `{"reviews":[{"reviewId":"a","comments":[{"userComment":{"text":"x","lastModified":{"seconds":"soon"}}}]}]}`, 0, false},
		{"not json", `<html>`, 0, false},
	} {
		p, err := ParseGooglePlayReviews(strings.NewReader(tc.body))
		if (err == nil) != tc.ok || len(p.Reviews) != tc.n {
			t.Errorf("%s: %v %+v", tc.name, err, p)
		}
		if tc.name == "stars out of range" && err == nil && (p.Reviews[0].Rating != 0 || !p.Reviews[0].CreatedAt.IsZero()) {
			t.Errorf("%s: %+v", tc.name, p.Reviews[0])
		}
	}
}
