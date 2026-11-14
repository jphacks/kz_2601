package extapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakePhotos is the Photos Picker API on httptest (no live calls).
type fakePhotos struct {
	calls atomic.Int64
	mu    sync.Mutex
	body  map[string]any
	query url.Values
	set   bool
	// deleted is the DELETE /sessions/{id} paths; gone answers them 404.
	deleted []string
	gone    bool
}

func newFakePhotos(t *testing.T) (*fakePhotos, map[string]Operation) {
	t.Helper()
	f := &fakePhotos{}
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	gerr := func(w http.ResponseWriter, status int, reason string) {
		writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "status": reason}})
	}
	session := func() map[string]any {
		return map[string]any{"id": "sess_0123456789", "pickerUri": "https://photos.google.com/picker/s/abc",
			"expireTime": "2026-11-14T03:00:00Z", "mediaItemsSet": f.set}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1":
		case "expired":
			gerr(w, 401, "UNAUTHENTICATED")
			return
		case "forbidden":
			gerr(w, 403, "PERMISSION_DENIED")
			return
		case "disabled":
			gerr(w, 403, "SERVICE_DISABLED")
			return
		case "busy":
			gerr(w, 503, "UNAVAILABLE")
			return
		default:
			gerr(w, 401, "UNAUTHENTICATED")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method != http.MethodDelete {
			f.query = r.URL.Query()
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			b, _ := io.ReadAll(r.Body)
			f.body = map[string]any{}
			_ = json.Unmarshal(b, &f.body)
			writeJSON(w, 200, session())
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/sessions/"):
			f.deleted = append(f.deleted, r.URL.Path)
			if f.gone {
				gerr(w, 404, "NOT_FOUND")
				return
			}
			writeJSON(w, 200, map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/sess_0123456789":
			writeJSON(w, 200, session())
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sessions/"):
			gerr(w, 404, "NOT_FOUND")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/mediaItems":
			writeJSON(w, 200, map[string]any{"mediaItems": []any{
				map[string]any{"id": "item_aaaaaaaa", "createTime": "2026-11-13T01:02:03Z", "type": "PHOTO",
					"mediaFile": map[string]any{"baseUrl": "https://lh3.googleusercontent.com/secret", "mimeType": "image/jpeg", "filename": "IMG_0001.JPG",
						"mediaFileMetadata": map[string]any{"width": 4032, "height": 3024}}},
				map[string]any{"id": "item_bbbbbbbb", "type": "VIDEO", "mediaFile": map[string]any{}},
			}})
		default:
			gerr(w, 404, "NOT_FOUND")
		}
	}))
	t.Cleanup(srv.Close)
	w := gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke", APIBase: srv.URL + "/v1", Client: srv.Client()}
	p, err := newGooglePhotos(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	return f, map[string]Operation{"startPicking": OperationFunc(p.startPicking), "pickedItems": OperationFunc(p.pickedItems)}
}

func photosIn(token string, args map[string]string) Input {
	return Input{Token: &Token{AccessToken: token, Scopes: []string{gphotosScope}}, Args: args}
}

func TestGooglePhotosStartPicking(t *testing.T) {
	f, ops := newFakePhotos(t)
	out, err := ops["startPicking"].Do(t.Context(), photosIn("access-1", map[string]string{"maxItems": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Row{Title: "Google フォトで写真を選ぶ", Subtitle: "11/14 12:00 まで", Value: "sess_0123456789", URL: "https://photos.google.com/picker/s/abc"}
	if out.Item == nil || *out.Item != want {
		t.Fatalf("item %+v", out.Item)
	}
	if cfg, _ := f.body["pickingConfig"].(map[string]any); cfg["maxItemCount"] != "10" {
		t.Fatalf("body %+v", f.body)
	}
	if _, err := ops["startPicking"].Do(t.Context(), photosIn("access-1", nil)); err != nil || len(f.body) != 0 {
		t.Fatalf("no maxItems: %v %+v", err, f.body)
	}
}

func TestGooglePhotosPickedItems(t *testing.T) {
	f, ops := newFakePhotos(t)
	args := map[string]string{"sessionId": "sess_0123456789", "limit": "1"}
	out, err := ops["pickedItems"].Do(t.Context(), photosIn("access-1", args))
	if err != nil {
		t.Fatal(err)
	}
	if out.Item == nil || out.Item.Title != "まだ選んでいません" || len(out.Rows) != 0 {
		t.Fatalf("not yet: %+v", out)
	}
	f.set = true
	out, err = ops["pickedItems"].Do(t.Context(), photosIn("access-1", args))
	if err != nil {
		t.Fatal(err)
	}
	if f.query.Get("sessionId") != "sess_0123456789" || f.query.Get("pageSize") != "1" {
		t.Fatalf("query %v", f.query)
	}
	want := Row{Title: "IMG_0001.JPG", Subtitle: "2026-11-13 10:02・写真・4032×3024", Value: "item_aaaaaaaa"}
	if len(out.Rows) != 1 || out.Rows[0] != want || out.Item.Title != "1 件を選びました" {
		t.Fatalf("rows %+v item %+v", out.Rows, out.Item)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "googleusercontent") {
		t.Fatal("the baseUrl (needs the token) reached the answer")
	}
	delete(args, "limit")
	out, _ = ops["pickedItems"].Do(t.Context(), photosIn("access-1", args))
	if len(out.Rows) != 2 || out.Rows[1] != (Row{Title: "(名前なし)", Subtitle: "動画", Value: "item_bbbbbbbb"}) {
		t.Fatalf("rows %+v", out.Rows)
	}
}

// pickedItems ends the session once the items are read (not before), and a
// session already gone (404) does not fail the answer.
func TestGooglePhotosPickedItemsDeletesSession(t *testing.T) {
	f, ops := newFakePhotos(t)
	args := map[string]string{"sessionId": "sess_0123456789"}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("access-1", args)); err != nil || len(f.deleted) != 0 {
		t.Fatalf("not yet picked: %v deleted %v", err, f.deleted)
	}
	f.set = true
	out, err := ops["pickedItems"].Do(t.Context(), photosIn("access-1", args))
	if err != nil || len(out.Rows) != 2 {
		t.Fatalf("picked: %v %+v", err, out)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "/v1/sessions/sess_0123456789" {
		t.Fatalf("deleted %v", f.deleted)
	}
	f.gone = true
	out, err = ops["pickedItems"].Do(t.Context(), photosIn("access-1", args))
	if err != nil || len(out.Rows) != 2 || len(f.deleted) != 2 {
		t.Fatalf("404 on delete: %v %+v deleted %v", err, out, f.deleted)
	}
}

// a transport failure is the shared upstream error (retryable, 502) and
// carries no token.
func TestGooglePhotosTransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	w := gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke", APIBase: srv.URL + "/v1", Client: srv.Client()}
	p, err := newGooglePhotos(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.pickedItems(t.Context(), photosIn("access-1", map[string]string{"sessionId": "sess_0123456789"}))
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
		t.Fatalf("transport: %v", err)
	}
	if strings.Contains(err.Error(), "access-1") {
		t.Fatalf("token in the error: %v", err)
	}
	if _, err := p.g.Refresh(t.Context(), Token{RefreshToken: "refresh-1"}); Reason(err) != ReasonUpstream || strings.Contains(err.Error(), gcalTestSecret) {
		t.Fatalf("token endpoint transport: %v", err)
	}
}

func TestGooglePhotosErrors(t *testing.T) {
	f, ops := newFakePhotos(t)
	args := map[string]string{"sessionId": "sess_0123456789"}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("expired", args)); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err) // Service turns it into not connected (409)
	}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("forbidden", args)); Reason(err) != ReasonNotConnected || StatusOf(Reason(err)) != http.StatusConflict {
		t.Fatalf("403: %v", err)
	}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("disabled", args)); Reason(err) != ReasonUpstream {
		t.Fatalf("disabled: %v", err)
	}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("busy", args)); Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
		t.Fatalf("503: %v", err)
	}
	if _, err := ops["pickedItems"].Do(t.Context(), photosIn("access-1", map[string]string{"sessionId": "sess_unknown00"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("404: %v", err)
	}
	if _, err := ops["startPicking"].Do(t.Context(), Input{Args: nil}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	narrow := Input{Token: &Token{AccessToken: "access-1", Scopes: []string{"https://www.googleapis.com/auth/calendar"}}, Args: args}
	n := f.calls.Load()
	if _, err := ops["pickedItems"].Do(t.Context(), narrow); !errors.Is(err, ErrNotConnected) || f.calls.Load() != n {
		t.Fatalf("narrow scope: %v", err)
	}
}

func TestGooglePhotosBadArgs(t *testing.T) {
	f, ops := newFakePhotos(t)
	p, _ := Lookup(GooglePhotos)
	for _, c := range []struct {
		op   string
		args map[string]string
	}{
		{"startPicking", map[string]string{"maxItems": "0"}},
		{"startPicking", map[string]string{"maxItems": "2001"}},
		{"pickedItems", map[string]string{}},
		{"pickedItems", map[string]string{"sessionId": "../x"}},
		{"pickedItems", map[string]string{"sessionId": "sess_0123456789", "limit": "101"}},
	} {
		if _, err := p.CheckArgs(c.op, c.args); !errors.Is(err, ErrUnknown) {
			t.Errorf("CheckArgs %s %v: %v", c.op, c.args, err)
		}
		if _, err := ops[c.op].Do(t.Context(), photosIn("access-1", c.args)); !errors.Is(err, ErrBadArgs) {
			t.Errorf("Do %s %v: %v", c.op, c.args, err)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatalf("bad args reached the API %d times", f.calls.Load())
	}
}

func TestGooglePhotosProvider(t *testing.T) {
	p, ok := Lookup(GooglePhotos)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.ClientFrom != GoogleCalendar || p.NameJa == "" {
		t.Fatalf("provider %+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 || len(o.Scopes) != 1 || o.Scopes[0] != gphotosScope {
			t.Fatalf("op %+v", o)
		}
	}
	if _, err := newGooglePhotos(Secrets{}, googlePhotosWire); err == nil {
		t.Fatal("no client accepted")
	}
}
