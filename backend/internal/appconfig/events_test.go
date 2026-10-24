package appconfig

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

type sseEvent struct {
	name string
	data map[string]any
}

// openEvents starts GET /events through the same middleware chain as
// cmd/appconfig (so Flush must pass through the httpx wrappers).
func openEvents(t *testing.T, srv *httptest.Server, uid, appID string) (<-chan sseEvent, func()) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/apps/"+appID+"/events", nil)
	req.Header.Set(auth.DebugUIDHeader, uid)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("events: %d %s", res.StatusCode, b)
	}
	out := make(chan sseEvent, 16)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(res.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.data)
			case line == "" && ev.name != "":
				out <- ev
				ev = sseEvent{}
			}
		}
	}()
	return out, func() { _ = res.Body.Close() }
}

func next(t *testing.T, ch <-chan sseEvent) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("stream closed")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
	}
	return sseEvent{}
}

func TestEvents(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "10s")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory()))
	srv := httptest.NewServer(httpx.Chain(api, httpx.WithRequestID, httpx.Log(logger), httpx.Recover(logger), httpx.ErrorEnvelope))
	defer srv.Close()
	e := &env{t: t, h: srv.Config.Handler}

	const appID = "RT-APP-1"
	app := "/v1/apps/" + appID
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(sharedKakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	// u2 installs the shared app and listens
	events, stop := openEvents(t, srv, "u2", appID)
	defer stop()
	hello := next(t, events)
	if hello.name != "hello" || hello.data["version"] != float64(1) {
		t.Fatalf("hello: %+v", hello)
	}
	if ents, _ := hello.data["entities"].([]any); len(ents) != 1 || ents[0] != "expense" {
		t.Fatalf("hello entities: %+v", hello)
	}

	// u1's post on the public entity reaches u2
	if r := e.do("u1", http.MethodPost, app+"/entities/expense/records", `{"data":{"amount":100}}`, nil); r.code != http.StatusCreated {
		t.Fatalf("post: %d %s", r.code, r.raw)
	}
	if ev := next(t, events); ev.name != "records" || ev.data["entity"] != "expense" {
		t.Fatalf("records: %+v", ev)
	}

	// u2's own DSL change: dsl event then bye (resubscribe with the new entity set)
	if r := e.do("u2", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("u2 put: %d %s", r.code, r.raw)
	}
	if ev := next(t, events); ev.name != "dsl" || ev.data["version"] != float64(2) {
		t.Fatalf("dsl: %+v", ev)
	}
	if ev := next(t, events); ev.name != "bye" {
		t.Fatalf("bye: %+v", ev)
	}
}

func TestEventsPlannedEnd(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("EVENTS_MAX_STREAM", "300ms")
	srv := httptest.NewServer(auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory())))
	defer srv.Close()

	events, stop := openEvents(t, srv, "u1", "RT-APP-2") // unknown appId is seeded like GET /dsl
	defer stop()
	if ev := next(t, events); ev.name != "hello" {
		t.Fatalf("hello: %+v", ev)
	}
	if ev := next(t, events); ev.name != "bye" {
		t.Fatalf("planned end must say bye: %+v", ev)
	}
}
