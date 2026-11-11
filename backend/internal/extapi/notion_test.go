package extapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeNotion is Notion's OAuth endpoints and API on httptest (no live calls).
type fakeNotion struct {
	url string

	mu        sync.Mutex
	expiring  bool // the token answer has a refresh token and an expiry
	refreshOK bool
	revoked   []string
	pages     []map[string]any // created pages' bodies
	lastQuery map[string]any
	versions  []string
	inFlight  atomic.Int32
	maxFlight atomic.Int32
	slow      time.Duration
}

const (
	notionTestID     = "notion-client-id"
	notionTestSecret = "notion-client-secret"
	notionTestDB     = "1a2b3c4d5e6f40718293a4b5c6d7e8f9"
	notionTestDBDash = "1a2b3c4d-5e6f-4071-8293-a4b5c6d7e8f9"
	notionTestDS     = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func newFakeNotion(t *testing.T) (*fakeNotion, *httptest.Server) {
	f := &fakeNotion{refreshOK: true}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	basicOK := func(r *http.Request) bool {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(notionTestID+":"+notionTestSecret))
		return r.Header.Get("Authorization") == want
	}
	mux.HandleFunc("POST /v1/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		if !basicOK(r) || json.NewDecoder(r.Body).Decode(&b) != nil {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch b["grant_type"] {
		case "authorization_code":
			if b["code"] != "good-code" || !strings.HasSuffix(b["redirect_uri"], "/oauth/callback/notion") {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			ans := map[string]any{"access_token": "access-1", "token_type": "bearer", "bot_id": "bot", "workspace_name": "ws"}
			if f.expiring {
				ans["refresh_token"], ans["expires_in"] = "refresh-1", 3600
			}
			writeJSON(w, 200, ans)
		case "refresh_token":
			if !f.refreshOK || b["refresh_token"] != "refresh-1" {
				writeJSON(w, 400, map[string]any{"object": "error", "status": 400, "code": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-2", "refresh_token": "refresh-2", "expires_in": 3600})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /v1/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		if !basicOK(r) || json.NewDecoder(r.Body).Decode(&b) != nil || r.Header.Get("Notion-Version") != notionVersion {
			writeJSON(w, 401, map[string]string{"code": "invalid_client"})
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, b["token"])
		f.mu.Unlock()
		if b["token"] == "gone" {
			writeJSON(w, 400, map[string]any{"object": "error", "code": "invalid_grant"})
			return
		}
		writeJSON(w, 200, map[string]string{"request_id": "r"})
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.versions = append(f.versions, r.Header.Get("Notion-Version"))
		slow := f.slow
		f.mu.Unlock()
		n := f.inFlight.Add(1)
		defer f.inFlight.Add(-1)
		for {
			m := f.maxFlight.Load()
			if n <= m || f.maxFlight.CompareAndSwap(m, n) {
				break
			}
		}
		if slow > 0 {
			time.Sleep(slow)
		}
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "access-2":
			return true
		case "busy":
			writeJSON(w, 429, map[string]any{"object": "error", "status": 429, "code": "rate_limited"})
		case "noaccess":
			writeJSON(w, 404, map[string]any{"object": "error", "status": 404, "code": "object_not_found"})
		default:
			writeJSON(w, 401, map[string]any{"object": "error", "status": 401, "code": "unauthorized"})
		}
		return false
	}
	mux.HandleFunc("GET /v1/databases/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.PathValue("id") != notionTestDBDash {
			writeJSON(w, 404, map[string]any{"object": "error", "code": "object_not_found"})
			return
		}
		writeJSON(w, 200, map[string]any{"object": "database", "id": notionTestDBDash, "data_sources": []map[string]string{{"id": notionTestDS, "name": "Tasks"}}})
	})
	mux.HandleFunc("GET /v1/data_sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"object": "data_source", "id": notionTestDS, "properties": map[string]any{
			"名前":  map[string]string{"id": "title", "type": "title"},
			"メモ":  map[string]string{"id": "a", "type": "rich_text"},
			"数":   map[string]string{"id": "b", "type": "number"},
			"済":   map[string]string{"id": "c", "type": "checkbox"},
			"状態":  map[string]string{"id": "d", "type": "status"},
			"タグ":  map[string]string{"id": "e", "type": "multi_select"},
			"期限":  map[string]string{"id": "f", "type": "date"},
			"リンク": map[string]string{"id": "g", "type": "url"},
			"担当":  map[string]string{"id": "h", "type": "people"},
			"メール": map[string]string{"id": "i", "type": "email"},
			"電話":  map[string]string{"id": "j", "type": "phone_number"},
			"分類":  map[string]string{"id": "k", "type": "select"},
		}})
	})
	mux.HandleFunc("POST /v1/data_sources/{id}/query", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var q map[string]any
		_ = json.NewDecoder(r.Body).Decode(&q)
		f.mu.Lock()
		f.lastQuery = q
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"object": "list", "has_more": false, "results": []map[string]any{
			{"object": "page", "id": "p1", "url": "https://www.notion.so/p1", "properties": map[string]any{
				"名前": map[string]any{"type": "title", "title": []map[string]string{{"plain_text": "牛乳"}, {"plain_text": "を買う"}}},
				"状態": map[string]any{"type": "status", "status": map[string]string{"name": "未着手"}},
				"期限": map[string]any{"type": "date", "date": map[string]any{"start": "2026-11-12", "end": nil}},
				"数":  map[string]any{"type": "number", "number": 2.5},
				"タグ": map[string]any{"type": "multi_select", "multi_select": []map[string]string{{"name": "家"}, {"name": "急ぎ"}}},
			}},
			{"object": "page", "id": "p2", "url": "https://evil.example/p2", "properties": map[string]any{
				"名前": map[string]any{"type": "title", "title": []map[string]string{}},
				"済":  map[string]any{"type": "checkbox", "checkbox": true},
				"式":  map[string]any{"type": "formula", "formula": map[string]any{"type": "number", "number": 3}},
			}},
		}})
	})
	mux.HandleFunc("POST /v1/pages", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		props, _ := b["properties"].(map[string]any)
		if n, ok := props["数"].(map[string]any); ok && n["number"] == float64(-1) {
			writeJSON(w, 400, map[string]any{"object": "error", "status": 400, "code": "validation_error"})
			return
		}
		f.mu.Lock()
		f.pages = append(f.pages, b)
		id := "page-" + string(rune('0'+len(f.pages)))
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"object": "page", "id": id, "url": "https://www.notion.so/" + id, "properties": props})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeNotion) wire(srv *httptest.Server) notionWire {
	return notionWire{AuthURL: srv.URL + "/v1/oauth/authorize", TokenURL: srv.URL + "/v1/oauth/token", RevokeURL: srv.URL + "/v1/oauth/revoke",
		APIBase: srv.URL + "/v1", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

func useNotion(t *testing.T) (*env, *fakeNotion) {
	f, srv := newFakeNotion(t)
	old := notionWireDefault
	notionWireDefault = f.wire(srv)
	t.Cleanup(func() { notionWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Notion] = true
		c.Secrets = map[string]Secrets{Notion: {ClientID: notionTestID, ClientSecret: notionTestSecret}}
	}})
	return e, f
}

func (e *env) connectNotion(f *fakeNotion) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Notion, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/v1/oauth/authorize?") || q.Get("owner") != "user" || q.Get("response_type") != "code" ||
		q.Get("client_id") != notionTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/notion" ||
		q.Get("state") == "" || q.Get("client_secret") != "" || q.Has("scope") {
		e.t.Fatalf("consent url %s", auth)
	}
	if back := e.svc.Callback(ctx, Notion, q.Get("state"), "good-code"); back != "tailorit://connected?provider=notion&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect → the token never expires (no refresh) → query → add → a 401
// needs a reconnect → disconnect revokes at Notion.
func TestNotionFlow(t *testing.T) {
	e, f := useNotion(t)
	ctx := context.Background()
	if _, err := e.call(e.req(Notion, "queryDatabase", map[string]string{"databaseId": notionTestDB})); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}
	e.connectNotion(f)
	tok, err := e.svc.Conns.Token(ctx, user, Notion)
	if err != nil || tok.AccessToken != "access-1" || !tok.Expiry.IsZero() || tok.RefreshToken != "" {
		t.Fatalf("token %v expiry %v", err, tok.Expiry)
	}

	// 80 days later the token is still used as is (Notion's do not expire)
	e.c.t = e.c.t.AddDate(0, 0, 80)
	res, err := e.call(e.req(Notion, "queryDatabase", map[string]string{"databaseId": notionTestDBDash, "subtitleProperty": "状態", "valueProperty": "期限"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0].Title != "牛乳を買う" || rows[0].Subtitle != "未着手" || rows[0].Value != "2026-11-12" ||
		rows[0].URL != "https://www.notion.so/p1" || rows[1].Title != gcalNoTitle || rows[1].URL != "" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(Notion, "addRow", map[string]string{"databaseId": notionTestDB, "title": "卵を買う", "properties": `{"状態":"未着手","数":3}`}))
	if err != nil || res.Output.ResultID != "page-1" || res.Output.Item.Title != "卵を買う" || len(f.pages) != 1 {
		t.Fatalf("addRow %+v %v", res.Output, err)
	}
	for _, v := range f.versions {
		if v != notionVersion {
			t.Fatalf("Notion-Version %q", v)
		}
	}

	// the token is refused (the person removed the integration) → error
	if err := e.svc.Conns.Save(ctx, user, Notion, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Notion, "queryDatabase", map[string]string{"databaseId": notionTestDB})); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("401: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Notion); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect, then cut: the access token is revoked at Notion
	e.connectNotion(f)
	if err := e.svc.Revoke(ctx, user, Notion); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Notion); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "access-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// When Notion does send a refresh token and an expiry, they are used; a
// refused refresh needs a reconnect.
func TestNotionRefresh(t *testing.T) {
	e, f := useNotion(t)
	ctx := context.Background()
	f.expiring = true
	e.connectNotion(f)
	tok, _ := e.svc.Conns.Token(ctx, user, Notion)
	if tok.RefreshToken != "refresh-1" || tok.Expiry.IsZero() {
		t.Fatalf("token expiry %v", tok.Expiry)
	}
	e.c.t = e.c.t.Add(2 * time.Hour)
	if _, err := e.call(e.req(Notion, "queryDatabase", map[string]string{"databaseId": notionTestDB})); err != nil {
		t.Fatal(err)
	}
	tok, _ = e.svc.Conns.Token(ctx, user, Notion)
	if tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-2" {
		t.Fatal("not refreshed")
	}
	f.mu.Lock()
	f.refreshOK = false
	f.mu.Unlock()
	if err := e.svc.Conns.Save(ctx, user, Notion, Token{AccessToken: "access-2", RefreshToken: "refresh-1", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Notion, "queryDatabase", map[string]string{"databaseId": notionTestDB})); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Notion); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func notionTestClient(t *testing.T) (*notion, *fakeNotion) {
	f, srv := newFakeNotion(t)
	c, err := newNotion(Secrets{ClientID: notionTestID, ClientSecret: notionTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestNotionAddRow(t *testing.T) {
	c, f := notionTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "access-1"}
	props := `{"メモ":"牛乳 2 本","数":"12.5","済":true,"状態":"進行中","タグ":["家","急ぎ"],"期限":"2026-11-12","リンク":"https://example.com/a","メール":"a@example.com","電話":"+81 90-0000-0000","分類":""}`
	out, err := c.addRow(ctx, Input{Token: tok, RequestID: "r1", Args: map[string]string{"databaseId": notionTestDB, "title": "買い物", "properties": props}})
	if err != nil {
		t.Fatal(err)
	}
	if out.ResultID != "page-1" || out.Item.Title != "買い物" || out.Item.URL != "https://www.notion.so/page-1" {
		t.Fatalf("out %+v", out)
	}
	b := f.pages[0]
	parent := b["parent"].(map[string]any)
	p := b["properties"].(map[string]any)
	got := func(name, key string) any { return p[name].(map[string]any)[key] }
	if parent["type"] != "data_source_id" || parent["data_source_id"] != notionTestDS ||
		got("名前", "title").([]any)[0].(map[string]any)["text"].(map[string]any)["content"] != "買い物" ||
		got("メモ", "rich_text").([]any)[0].(map[string]any)["text"].(map[string]any)["content"] != "牛乳 2 本" ||
		got("数", "number") != 12.5 || got("済", "checkbox") != true ||
		got("状態", "status").(map[string]any)["name"] != "進行中" ||
		len(got("タグ", "multi_select").([]any)) != 2 ||
		got("期限", "date").(map[string]any)["start"] != "2026-11-12" ||
		got("リンク", "url") != "https://example.com/a" || got("メール", "email") != "a@example.com" ||
		got("電話", "phone_number") != "+81 90-0000-0000" || got("分類", "select") != nil {
		t.Fatalf("body %v", b)
	}
	// the same request again adds no second row
	again, err := c.addRow(ctx, Input{Token: tok, RequestID: "r1", Args: map[string]string{"databaseId": notionTestDB, "title": "買い物"}})
	if err != nil || again.ResultID != "page-1" || len(f.pages) != 1 {
		t.Fatalf("retry %+v %v (%d pages)", again, err, len(f.pages))
	}
	// at the same time too: one row
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.addRow(ctx, Input{Token: tok, RequestID: "r2", Args: map[string]string{"databaseId": notionTestDB, "title": "同時"}})
		}()
	}
	wg.Wait()
	if len(f.pages) != 2 {
		t.Fatalf("concurrent retries made %d pages", len(f.pages))
	}
	// another request id is another row; a time date keeps its offset
	if _, err := c.addRow(ctx, Input{Token: tok, RequestID: "r3", Args: map[string]string{"databaseId": notionTestDB, "title": "x", "properties": `{"期限":"2026-11-12T10:00:00+09:00","タグ":"a, b"}`}}); err != nil {
		t.Fatal(err)
	}
	p = f.pages[2]["properties"].(map[string]any)
	if got("期限", "date").(map[string]any)["start"] != "2026-11-12T10:00:00+09:00" || len(got("タグ", "multi_select").([]any)) != 2 {
		t.Fatalf("body %v", p)
	}
	// Notion refusing a value is a bad request, and is not remembered
	_, err = c.addRow(ctx, Input{Token: tok, RequestID: "r4", Args: map[string]string{"databaseId": notionTestDB, "title": "x", "properties": `{"数":-1}`}})
	if Reason(err) != ReasonBadRequest {
		t.Fatalf("validation_error: %v", err)
	}
	if _, ok := c.memo.done[notionMemoKey(Input{Token: tok, RequestID: "r4"}, notionTestDBDash)]; ok {
		t.Fatal("a failed write was remembered")
	}
}

func TestNotionArgs(t *testing.T) {
	c, f := notionTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "access-1"}
	db := notionTestDB
	bad := []map[string]string{
		{"title": "x"}, // no database
		{"databaseId": "https://notion.so/x", "title": "x"}, // not an id
		{"databaseId": db}, // no title
		{"databaseId": db, "title": strings.Repeat("あ", 2001)},
		{"databaseId": db, "title": "a\x00b"},
		{"databaseId": db, "title": "x", "properties": "[1]"},
		{"databaseId": db, "title": "x", "properties": `{"a":1} {}`},
		{"databaseId": db, "title": "x", "properties": `{"無い":"x"}`},
		{"databaseId": db, "title": "x", "properties": `{"担当":"x"}`}, // people cannot be set
		{"databaseId": db, "title": "x", "properties": `{"数":"多い"}`},
		{"databaseId": db, "title": "x", "properties": `{"済":"たぶん"}`},
		{"databaseId": db, "title": "x", "properties": `{"期限":"明日"}`},
		{"databaseId": db, "title": "x", "properties": `{"リンク":"javascript:alert(1)"}`},
		{"databaseId": db, "title": "x", "properties": `{"リンク":"http://example.com"}`},
		{"databaseId": db, "title": "x", "properties": `{"メール":"nope"}`},
		{"databaseId": db, "title": "x", "properties": `{"電話":"call me"}`},
		{"databaseId": db, "title": "x", "properties": `{"タグ":[1]}`},
		{"databaseId": db, "title": "x", "properties": `{"状態":` + `"` + strings.Repeat("s", 101) + `"}`},
		{"databaseId": db, "title": "x", "properties": `{"メモ":5}`},
		{"databaseId": db, "title": "x", "properties": `{"a1":1,"a2":1,"a3":1,"a4":1,"a5":1,"a6":1,"a7":1,"a8":1,"a9":1,"a10":1,"a11":1,"a12":1,"a13":1,"a14":1,"a15":1,"a16":1,"a17":1,"a18":1,"a19":1,"a20":1,"a21":1}`},
		{"databaseId": db, "title": "x", "properties": `{"メモ":"` + strings.Repeat("m", 9000) + `"}`},
	}
	for i, a := range bad {
		if _, err := c.addRow(ctx, Input{Token: tok, RequestID: "bad", Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("addRow case %d: %v", i, err)
		}
	}
	badQ := []map[string]string{
		{},
		{"databaseId": "xyz"},
		{"databaseId": db, "limit": "0"},
		{"databaseId": db, "limit": "51"},
		{"databaseId": db, "limit": "x"},
		{"databaseId": db, "valueProperty": strings.Repeat("v", 201)},
	}
	for i, a := range badQ {
		if _, err := c.queryDatabase(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("queryDatabase case %d: %v", i, err)
		}
	}
	if len(f.pages) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Notion")
	}
	out, err := c.queryDatabase(ctx, Input{Token: tok, Args: map[string]string{"databaseId": db, "subtitleProperty": "タグ", "valueProperty": "数"}})
	if err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery; q["page_size"] != float64(20) || q["result_type"] != "page" {
		t.Fatalf("query %v", q)
	}
	if r := out.Rows[0]; r.Subtitle != "家, 急ぎ" || r.Value != "2.5" {
		t.Fatalf("row %+v", r)
	}
	out, _ = c.queryDatabase(ctx, Input{Token: tok, Args: map[string]string{"databaseId": db, "subtitleProperty": "済", "valueProperty": "式", "limit": "5"}})
	if r := out.Rows[1]; r.Subtitle != "✓" || r.Value != "3" || f.lastQuery["page_size"] != float64(5) {
		t.Fatalf("row %+v", r)
	}
}

func TestNotionErrors(t *testing.T) {
	c, f := notionTestClient(t)
	ctx := context.Background()
	q := map[string]string{"databaseId": notionTestDB}
	_, err := c.queryDatabase(ctx, Input{Token: &Token{AccessToken: "busy"}, Args: q})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("429: %v", err)
	}
	if _, err := c.queryDatabase(ctx, Input{Token: &Token{AccessToken: "noaccess"}, Args: q}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("not shared: %v", err)
	}
	if _, err := c.queryDatabase(ctx, Input{Token: &Token{AccessToken: "access-1"}, Args: map[string]string{"databaseId": strings.Repeat("0", 32)}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("unknown database: %v", err)
	}
	if _, err := c.queryDatabase(ctx, Input{Token: &Token{AccessToken: "dead"}, Args: q}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.queryDatabase(ctx, Input{Args: q}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.addRow(ctx, Input{Args: map[string]string{"databaseId": notionTestDB, "title": "x"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("addRow no token: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "gone"}); err != nil {
		t.Fatalf("revoke gone: %v", err)
	}
	if err := c.Revoke(ctx, Token{}); err != nil || len(f.revoked) != 1 {
		t.Fatalf("revoke nothing: %v %v", err, f.revoked)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "a"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad-code", "", "https://runner.example/oauth/callback/notion"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
}

// At most notionMaxConcurrent requests reach Notion at once (its rate limit).
func TestNotionConcurrency(t *testing.T) {
	c, f := notionTestClient(t)
	f.slow = 20 * time.Millisecond
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.queryDatabase(context.Background(), Input{Token: &Token{AccessToken: "access-1"}, Args: map[string]string{"databaseId": notionTestDB}})
		}()
	}
	wg.Wait()
	if m := f.maxFlight.Load(); m > notionMaxConcurrent || m < 2 {
		t.Fatalf("max in flight %d", m)
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret the provider stays off; off by default
// and never in the review build.
func TestNotionDefinition(t *testing.T) {
	p, ok := Lookup(Notion)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "NOTION" {
		t.Fatalf("%+v", p)
	}
	w := notionWireDefault
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/databases/x", w.APIBase + "/pages"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	c, err := newNotion(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil {
		t.Fatal("built without a secret")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Notion] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
	if id, err := notionID("x", notionTestDB); err != nil || id != notionTestDBDash {
		t.Fatalf("id %q %v", id, err)
	}
}
