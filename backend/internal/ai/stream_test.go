package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
)

// feedChunks pushes s into the watcher in n-byte pieces (multi-byte runes
// may be split, as real deltas never are, but the scanner is byte-based).
func feedChunks(w *keyWatcher, s string, n int) ([]byte, int) {
	for i := 0; i < len(s); i += n {
		end := min(i+n, len(s))
		if v, ok := w.Feed(s[i:end]); ok {
			return v, end
		}
	}
	return nil, -1
}

func TestKeyWatcher_AppBlockFromCompactAndPrettyJSON(t *testing.T) {
	compact := abbreviateExample(fewShotKakeibo)
	pretty := fewShotKakeibo
	for name, src := range map[string]string{"compact": compact, "pretty": pretty, "fenced": "```json\n" + compact + "\n```"} {
		for _, n := range []int{1, 7, 64, 100000} {
			w := newKeyWatcher("app")
			v, at := feedChunks(w, src, n)
			if at < 0 {
				t.Fatalf("%s/%d: app block never completed", name, n)
			}
			var m map[string]any
			if err := json.Unmarshal(v, &m); err != nil || m["name"] != "家計簿" || len(m["tabs"].([]any)) != 2 {
				t.Fatalf("%s/%d: bad app block %s (%v)", name, n, v, err)
			}
			if n < len(src) && at > len(src)/2 {
				t.Errorf("%s/%d: app block completed only at byte %d of %d", name, n, at, len(src))
			}
			if _, ok := w.Feed("}"); ok {
				t.Errorf("%s/%d: watcher fired twice", name, n)
			}
		}
	}
}

func TestKeyWatcher_SummaryStringAndNestedKeys(t *testing.T) {
	// "summary" also appears nested inside a value and inside a string: only the top-level one counts.
	src := `{"summary":"Added \"cafe\" to the list.","patch":[{"op":"add","path":"/x","value":{"summary":"inner"}}]}`
	w := newKeyWatcher("summary")
	v, _ := feedChunks(w, src, 3)
	var s string
	if err := json.Unmarshal(v, &s); err != nil || s != `Added "cafe" to the list.` {
		t.Fatalf("summary = %s (%v)", v, err)
	}
	w = newKeyWatcher("summary")
	v, _ = feedChunks(w, `{"patch":[{"op":"test","path":"/a","value":{"summary":"no"}}],"summary":"yes"}`, 5)
	_ = json.Unmarshal(v, &s)
	if s != "yes" {
		t.Fatalf("nested summary must be skipped: %q", s)
	}
	w = newKeyWatcher("app")
	if v, ok := w.Feed(`{"app":{"name":"un终`); ok {
		t.Fatalf("incomplete block must not fire: %s", v)
	}
	w = newKeyWatcher("n")
	if v, ok := w.Feed(`{"n": 12, "x"`); !ok || string(v) != "12" {
		t.Fatalf("scalar value: %s %v", v, ok)
	}
}

// sseServer imitates the Messages API streaming reply for one request.
func sseServer(t *testing.T, text string, chunk int, trailer string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Errorf("stream must be requested: %v", req["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(ev string, v any) {
			b, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
		}
		write("message_start", map[string]any{"type": "message_start", "message": map[string]any{"model": "m", "usage": map[string]any{"input_tokens": 12, "cache_read_input_tokens": 1000, "output_tokens": 1}}})
		write("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		runes := []rune(text) // real deltas never split a rune
		for i := 0; i < len(runes); i += chunk {
			write("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": string(runes[i:min(i+chunk, len(runes))])}})
		}
		_, _ = fmt.Fprint(w, "event: ping\ndata: {\"type\": \"ping\"}\n\n")
		write("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		if trailer != "" {
			_, _ = fmt.Fprint(w, trailer)
			return
		}
		write("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 345}})
		write("message_stop", map[string]any{"type": "message_stop"})
	}))
}

func TestClient_StreamingAssemblesTextAndUsage(t *testing.T) {
	text := abbreviateExample(fewShotKakeibo)
	srv := sseServer(t, text, 10, "")
	defer srv.Close()
	c := NewClient("k", "m", "low")
	c.BaseURL = srv.URL
	var got strings.Builder
	resp, err := c.Complete(context.Background(), Completion{System: "s", Messages: []Message{{Role: "user", Content: "u"}}, OnText: func(s string) { got.WriteString(s) }})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != text || got.String() != text {
		t.Fatalf("text mismatch: %d/%d vs %d", len(resp.Text), got.Len(), len(text))
	}
	if resp.StopReason != "end_turn" || resp.Usage.OutputTokens != 345 || resp.Usage.InputTokens != 12 || resp.Usage.CacheReadInputTokens != 1000 || resp.Model != "m" {
		t.Fatalf("usage/stop: %+v", resp)
	}
}

func TestClient_StreamErrorEventIsAPIError(t *testing.T) {
	srv := sseServer(t, `{"a":`, 10, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n")
	defer srv.Close()
	c := NewClient("k", "m", "low")
	c.BaseURL = srv.URL
	c.MaxRetries = 2
	calls := 0
	_, err := c.Complete(context.Background(), Completion{Messages: []Message{{Role: "user", Content: "u"}}, OnText: func(string) { calls++ }})
	ae, ok := err.(*APIError)
	if !ok || ae.Type != "stream_overloaded_error" || ae.Status != 500 {
		t.Fatalf("want stream APIError, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("a stream that already delivered text must not be retried (deltas=%d)", calls)
	}
}

// streamingFake replays each reply through OnText in small chunks.
type streamingFake struct{ fakeCaller }

func (f *streamingFake) Complete(ctx context.Context, req Completion) (*Response, error) {
	resp, err := f.fakeCaller.Complete(ctx, req)
	if err == nil && req.OnText != nil {
		for i := 0; i < len(resp.Text); i += 16 {
			req.OnText(resp.Text[i:min(i+16, len(resp.Text))])
		}
	}
	return resp, err
}

func TestGenerateStream_EmitsAppEventBeforeResult(t *testing.T) {
	f := &streamingFake{fakeCaller{replies: []string{abbreviateExample(fewShotKakeibo)}}}
	svc := &Service{Claude: f, Store: appconfigclient.NewMemory(), Model: "test-model"}
	var events []Event
	res, err := svc.GenerateStream(context.Background(), Request{AppID: "s1", Transcript: "家計簿アプリを作って"}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "app" || events[0].Name != "家計簿" || events[0].Attempt != 1 ||
		strings.Join(events[0].Tabs, ",") != "ホーム,集計" || events[0].Summary != "家計簿アプリを作っています。ホームと集計の2タブです。" || events[0].Locale != LocaleJA {
		t.Fatalf("events: %+v", events)
	}
	if f.calls[0].OnText == nil {
		t.Fatal("streaming request must set OnText")
	}
	// The abbreviated reply was completed: the stored DSL carries the defaults.
	for _, want := range []string{`"schemaVersion":1`, `"voiceEntry":"floating"`, `"presentation":"sheet"`, `"swipeToDelete":true`, `"value":null`} {
		if !strings.Contains(string(res.DSL), want) {
			t.Errorf("saved DSL lacks %s", want)
		}
	}
	if res.Meta.OutputChars == 0 || res.Summary == "" {
		t.Fatalf("meta/summary: %+v %q", res.Meta, res.Summary)
	}
}

func TestGenerateStream_EditSummaryFirst(t *testing.T) {
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "e1", 1, []byte(fewShotKakeibo), appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	reply := `{"summary":"カテゴリにカフェを追加しました。","patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"}]}`
	f := &streamingFake{fakeCaller{replies: []string{reply}}}
	svc := &Service{Claude: f, Store: store, Model: "test-model"}
	var events []Event
	res, err := svc.GenerateStream(context.Background(), Request{AppID: "e1", Transcript: "カテゴリにカフェを追加して"}, func(ev Event) { events = append(events, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "summary" || events[0].Summary != "カテゴリにカフェを追加しました。" {
		t.Fatalf("events: %+v", events)
	}
	if res.Mode != "patch" || !strings.Contains(f.calls[0].System, `{"summary":"...","patch":`) {
		t.Fatalf("edit prompt must ask for summary first: mode=%s", res.Mode)
	}
}

func TestHandler_SSE(t *testing.T) {
	f := &streamingFake{fakeCaller{replies: []string{abbreviateExample(fewShotKakeibo)}}}
	svc := &Service{Claude: f, Store: appconfigclient.NewMemory(), Model: "test-model"}
	srv := httptest.NewServer(NewHandler(svc))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/generate?stream=1", "application/json", strings.NewReader(`{"appId":"h1","transcript":"家計簿アプリを作って"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	var names []string
	var last string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			names = append(names, strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") {
			last = strings.TrimPrefix(line, "data: ")
		}
	}
	if strings.Join(names, ",") != "app,result" {
		t.Fatalf("events %v", names)
	}
	var out Result
	if err := json.Unmarshal([]byte(last), &out); err != nil || out.Version != 2 || out.Mode != "full" {
		t.Fatalf("result frame: %v %+v", err, out)
	}

	// error path: unfixable output -> event: error with the JSON-mode body
	f2 := &streamingFake{fakeCaller{replies: []string{`{"app":{"name":"x"},"initialScreen":"nope","entities":[],"screens":[]}`}}}
	svc2 := &Service{Claude: f2, Store: appconfigclient.NewMemory(), Model: "test-model", MaxRegenerations: -1}
	srv2 := httptest.NewServer(NewHandler(svc2))
	defer srv2.Close()
	resp2, err := http.Post(srv2.URL+"/v1/generate?stream=1", "application/json", strings.NewReader(`{"appId":"h2","transcript":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	var body strings.Builder
	sc = bufio.NewScanner(resp2.Body)
	for sc.Scan() {
		body.WriteString(sc.Text() + "\n")
	}
	if !strings.Contains(body.String(), "event: error\n") || !strings.Contains(body.String(), `"code":"generation_failed"`) || !strings.Contains(body.String(), `"status":400`) {
		t.Fatalf("error frame: %s", body.String())
	}
}
