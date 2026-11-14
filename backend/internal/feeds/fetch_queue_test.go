package feeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A caller that stops waiting does not cancel the fetch: it finishes, is
// cached, and the next caller reads it without another request.
func TestFetchOutlivesCaller(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		<-release
		_, _ = w.Write([]byte(rss2))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	s := New(Options{Client: srv.Client(), Now: (&clock{t: testNow}).now, Endpoints: &Endpoints{
		RSS: map[string]RSSFeed{"digital-news": {Name: "RSS2", URL: srv.URL + "/rss2.xml"}}}})
	q := Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Fatalf("caller that gave up: %v, want ErrUpstream", err)
	}
	close(release)
	res, err := s.Get(context.Background(), q, 5) // joins the fetch still in flight or reads its result
	if err != nil || res.Stale || len(res.Items) == 0 {
		t.Fatalf("after the fetch: %+v %v", res, err)
	}
	if _, err := s.Get(context.Background(), q, 5); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("source hit %d times, want 1", hits)
	}
}

// A fetch that ended by its context is not remembered as a failure.
func TestContextFailureNotCached(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	q := Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}
	k := q.key()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &call{done: make(chan struct{})}
	s.inflight[k] = c
	s.run(ctx, k, q, nil, c, nil)
	if !errors.Is(c.err, ErrUpstream) {
		t.Errorf("cancelled fetch: %v", c.err)
	}
	if s.cache[k] != nil {
		t.Fatalf("cancelled fetch was cached: %+v", s.cache[k])
	}
	if _, err := s.Get(context.Background(), q, 5); err != nil {
		t.Errorf("next call: %v", err)
	}
}

// Callers waiting for a gap provider are served in arrival order.
func TestGapQueueFIFO(t *testing.T) {
	src := newSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{})
	turn := s.turns[dsl.FeedArxiv]
	turn <- struct{}{} // someone else's fetch is running
	ids := []string{"cs.AI", "cs.CL", "cs.CV"}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedArxiv, FeedID: id}, 5); err != nil {
				t.Errorf("%s: %v", id, err)
			}
		}()
		time.Sleep(30 * time.Millisecond) // let it queue up before the next one
	}
	<-turn
	wg.Wait()
	var got []string
	for _, v := range src.requests() {
		got = append(got, v.Get("search_query"))
	}
	want := []string{"cat:cs.AI", "cat:cs.CL", "cat:cs.CV"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// diet has its own per-minute budget, and a user's search words reach the
// source only so often per hour.
func TestDietBudgetAndUserCap(t *testing.T) {
	src := newSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{PerMinute: 1, DietPerMinute: 2, UserMissesPerHour: 2})
	ctx := context.Background()
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedArxiv, FeedID: "cs.AI"}, 5); err != nil {
		t.Fatal(err)
	}
	// the shared budget is used up; diet still has its own
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "予算", UID: "u1"}, 5); err != nil {
		t.Fatalf("diet after the shared budget: %v", err)
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "決算", UID: "u1"}, 5); err != nil {
		t.Fatalf("second diet: %v", err)
	}
	// diet's budget is used up too
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "税", UID: "u2"}, 5); !errors.Is(err, ErrBusy) {
		t.Errorf("third diet in a minute: %v, want ErrBusy", err)
	}
	c.add(time.Minute)
	// u1 used both search-word fetches of this hour; cached words still answer
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "税", UID: "u1"}, 5); !errors.Is(err, ErrUserLimit) {
		t.Errorf("u1 third word: %v, want ErrUserLimit", err)
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "予算", UID: "u1"}, 5); err != nil {
		t.Errorf("cached word: %v", err)
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "税", UID: "u2"}, 5); err != nil {
		t.Errorf("u2: %v", err)
	}
	c.add(time.Hour)
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "年金", UID: "u1"}, 5); err != nil {
		t.Errorf("next hour: %v", err)
	}
}

// export.arxiv.org may write the links as http://arxiv.org/...
func TestArxivHTTPLinkUpgraded(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><id>http://arxiv.org/abs/2611.00001v1</id><title>A</title>
 <link href="http://arxiv.org/abs/2611.00001v1" rel="alternate" type="text/html"/></entry>
<entry><id>http://arxiv.org/abs/2611.00002v1</id><title>B</title></entry>
<entry><id>http://export.arxiv.org/abs/2611.00003v1</id><title>C</title>
 <link href="http://evil.example/abs/x" rel="alternate"/></entry>
</feed>`)
	items, err := parseArxiv(body)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://arxiv.org/abs/2611.00001v1", "https://arxiv.org/abs/2611.00002v1", "https://export.arxiv.org/abs/2611.00003v1"}
	for i, it := range items {
		if it.URL != want[i] {
			t.Errorf("%s: url = %q, want %q", it.Title, it.URL, want[i])
		}
	}
}
