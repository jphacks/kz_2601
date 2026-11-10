package ingest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type deletedUsers map[string]bool

func (d deletedUsers) UserDeleted(_ context.Context, uid string) (bool, error) { return d[uid], nil }

func allowAll() Policy {
	return Policy{ConsentVersions: map[string]bool{"feedback-2027-01": true},
		AISources: map[loop.Source]bool{loop.SourceInApp: true, loop.SourceCrash: true, loop.SourceStoreReview: true}}
}

func TestInAppIngestMasksAndHolds(t *testing.T) {
	ctx := context.Background()
	fs := store.NewMemory()
	send := func(uid, msg, consent string, shot bool) {
		t.Helper()
		fb := store.Feedback{UID: uid, Kind: store.FeedbackBug, Message: msg, ConsentVersion: consent,
			Device: &store.FeedbackDevice{AppVersion: "1.4.2", Build: "120", OS: "iOS 26.1", Model: "iPhone17,1", Locale: "ja_JP"}}
		if shot {
			fb.Screenshot = []byte{0xff, 0xd8}
		}
		if _, err := fs.CreateFeedback(ctx, fb, 0); err != nil {
			t.Fatal(err)
		}
	}
	send("u1", "写真を追加しようとして一覧の画面から追加のボタンを押すと、毎回アプリが落ちてしまいます。返事は a@example.com へ", "feedback-2027-01", true)
	send("u2", "並べ替えが効かない", "feedback-2026-11", false)      // old consent: held
	send("gone", "消した人のフィードバック", "feedback-2027-01", false) // deleted account: skipped
	send("u3", "a@example.com b@example.com", "feedback-2027-01", false)

	ls := loop.NewMemory()
	now := time.Now().UTC()
	r := &Runner{Store: ls, Policy: allowAll(), Users: deletedUsers{"gone": true}, Now: func() time.Time { return now }}
	st, err := r.Run(ctx, InApp{Reader: fs})
	if err != nil {
		t.Fatal(err)
	}
	if st.Fetched != 4 || st.Created != 3 || st.Skipped != 1 || st.Held != 2 {
		t.Fatalf("stats %+v", st)
	}
	items, _ := ls.ListItems(ctx, loop.ItemQuery{})
	byUID := map[string]loop.Item{}
	for _, it := range items {
		byUID[it.UID] = it
		if strings.Contains(it.Text, "example.com") || strings.Contains(it.Text, "a@") {
			t.Fatalf("unmasked text stored: %q", it.Text)
		}
		if it.OSMajor != "iOS 26" || it.AppVersion != "1.4.2" {
			t.Fatalf("device: %+v", it)
		}
		if !it.ExpiresAt.Equal(now.Add(loop.Retention)) {
			t.Fatalf("expiresAt %v", it.ExpiresAt)
		}
	}
	if it := byUID["u1"]; !it.AIAllowed || it.Status != loop.ItemNew || !it.HasScreenshot || it.MaskCounts["[メール]"] != 1 {
		t.Fatalf("u1: %+v", it)
	}
	if it := byUID["u2"]; it.AIAllowed || it.HoldReason != loop.HoldConsent {
		t.Fatalf("u2: %+v", it)
	}
	if it := byUID["u3"]; it.AIAllowed || it.HoldReason != loop.HoldMasked {
		t.Fatalf("u3: %+v", it)
	}
	if _, ok := byUID["gone"]; ok {
		t.Fatal("deleted account ingested")
	}

	// second run: nothing new, nothing duplicated
	st, err = r.Run(ctx, InApp{Reader: fs})
	if err != nil {
		t.Fatal(err)
	}
	if st.Created != 0 {
		t.Fatalf("re-run created %d", st.Created)
	}
	if n, _ := ls.ListItems(ctx, loop.ItemQuery{}); len(n) != 3 {
		t.Fatalf("items %d", len(n))
	}
}

func TestDefaultPolicyKeepsInAppFromAI(t *testing.T) {
	p := PolicyFromEnv(func(string) string { return "" })
	it := Normalize(Raw{Source: loop.SourceInApp, SourceID: "fb_1", Text: "落ちる", ConsentVersion: "feedback-2026-11"}, p, time.Now())
	if it.AIAllowed || it.HoldReason != loop.HoldConsent {
		t.Fatalf("default policy let in-app through: %+v", it)
	}
	it = Normalize(Raw{Source: loop.SourceStoreReview, SourceID: "r1", Text: "落ちる"}, p, time.Now())
	if it.AIAllowed || it.HoldReason != loop.HoldSource {
		t.Fatalf("default policy let reviews through: %+v", it)
	}
	it = Normalize(Raw{Source: loop.SourceCrash, SourceID: "c1", Text: "クラッシュの急増"}, p, time.Now())
	if !it.AIAllowed {
		t.Fatalf("crash held: %+v", it)
	}
	p = PolicyFromEnv(func(k string) string {
		return map[string]string{"LOOP_CONSENT_VERSIONS": "feedback-2027-01, x", "LOOP_AI_SOURCES": "app_store_review"}[k]
	})
	if !p.ConsentVersions["feedback-2027-01"] || !p.AISources[loop.SourceStoreReview] || p.AISources[loop.SourceCrash] {
		t.Fatalf("env policy %+v", p)
	}
}

func TestExpiryNeverOutlivesSource(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-150 * 24 * time.Hour) // 30 days left of 180
	it := Normalize(Raw{Source: loop.SourceInApp, SourceID: "fb_x", Text: "x", CreatedAt: old, SourceRetention: store.FeedbackRetention}, allowAll(), now)
	if want := old.Add(store.FeedbackRetention); !it.ExpiresAt.Equal(want) {
		t.Fatalf("expires %v want %v", it.ExpiresAt, want)
	}
}

func TestCrashSpike(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	ls := loop.NewMemory()
	r := &Runner{Store: ls, Policy: allowAll(), Now: func() time.Time { return at.Add(time.Hour) }}
	src := Crashes{Src: StaticCrashes{{IssueID: "abc123", Type: "EXC_BAD_ACCESS", Count: 42, AppVersion: "1.4.2", At: at}}}
	if _, err := r.Run(ctx, src); err != nil {
		t.Fatal(err)
	}
	items, _ := ls.ListItems(ctx, loop.ItemQuery{})
	if len(items) != 1 || items[0].Count != 42 || !strings.Contains(items[0].Text, "EXC_BAD_ACCESS") || !items[0].AIAllowed {
		t.Fatalf("%+v", items)
	}
}

func TestOSMajor(t *testing.T) {
	for in, want := range map[string]string{"iOS 26.1": "iOS 26", "26.1.2": "26", "iPadOS 18": "iPadOS 18", "": ""} {
		if got := OSMajor(in); got != want {
			t.Errorf("OSMajor(%q) = %q want %q", in, got, want)
		}
	}
}

func testKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestAppStoreReviews(t *testing.T) {
	key, pemKey := testKey(t)
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var srv *httptest.Server
	calls := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		verifyES256(t, tok, &key.PublicKey)
		if r.URL.Path != "/v1/apps/123/customerReviews" {
			t.Errorf("path %s", r.URL.Path)
		}
		review := func(id string, at time.Time, body string) map[string]any {
			return map[string]any{"type": "customerReviews", "id": id, "attributes": map[string]any{
				"rating": 2, "title": "落ちる", "body": body, "reviewerNickname": "nick-should-drop", "territory": "JPN",
				"createdDate": at.Format(time.RFC3339)}}
		}
		var page map[string]any
		if r.URL.Query().Get("cursor") == "" {
			page = map[string]any{"data": []any{review("r1", since.Add(48*time.Hour), "連絡先 a@example.com")},
				"links": map[string]any{"next": srv.URL + "/v1/apps/123/customerReviews?cursor=2"}}
		} else {
			page = map[string]any{"data": []any{review("r2", since.Add(time.Hour), "保存できない"), review("r0", since.Add(-time.Hour), "old")}}
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	cfg, err := AppStoreConfigFromEnv(func(k string) string {
		return map[string]string{EnvAppStoreOn: "on", EnvAppStoreIssuer: "iss", EnvAppStoreKeyID: "kid", EnvAppStoreKey: pemKey, EnvAppStoreAppID: "123"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	a := AppStoreReviews{Cfg: cfg, BaseURL: srv.URL}
	raws, err := a.Fetch(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(raws) != 2 || raws[0].SourceID != "r1" || raws[1].SourceID != "r2" || raws[0].Rating != 2 {
		t.Fatalf("calls %d raws %+v", calls, raws)
	}
	for _, r := range raws {
		if strings.Contains(r.Text, "nick-should-drop") || r.UID != "" {
			t.Fatalf("nickname kept: %+v", r)
		}
	}
	it := Normalize(raws[0], allowAll(), time.Now())
	if strings.Contains(it.Text, "example.com") {
		t.Fatalf("review not masked: %q", it.Text)
	}
}

func TestAppStoreOffAndBadConfig(t *testing.T) {
	if _, err := AppStoreConfigFromEnv(func(string) string { return "" }); err != ErrAppStoreOff {
		t.Fatalf("off: %v", err)
	}
	_, err := AppStoreConfigFromEnv(func(k string) string { return map[string]string{EnvAppStoreOn: "on"}[k] })
	if err == nil {
		t.Fatal("missing values accepted")
	}
	_, err = AppStoreConfigFromEnv(func(k string) string {
		return map[string]string{EnvAppStoreOn: "on", EnvAppStoreIssuer: "i", EnvAppStoreKeyID: "k", EnvAppStoreKey: "not pem", EnvAppStoreAppID: "1"}[k]
	})
	if err == nil {
		t.Fatal("bad key accepted")
	}
}

func TestAppStoreRefusesForeignNextLink(t *testing.T) {
	key, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[],"links":{"next":"https://elsewhere.example/v1/x"}}`)
	}))
	defer srv.Close()
	a := AppStoreReviews{Cfg: AppStoreConfig{IssuerID: "i", KeyID: "k", Key: key, AppID: "1"}, BaseURL: srv.URL}
	if _, err := a.Fetch(context.Background(), time.Time{}); err == nil {
		t.Fatal("followed a foreign link")
	}
}

func verifyES256(t *testing.T, tok string, pub *ecdsa.PublicKey) {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", tok)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("sig: %v", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, sum[:], r, s) {
		t.Fatal("bad signature")
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	_ = json.Unmarshal(claims, &c)
	if c["aud"] != "appstoreconnect-v1" || c["iss"] != "iss" {
		t.Fatalf("claims %v", c)
	}
}
