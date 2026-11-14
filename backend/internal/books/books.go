// Package books looks up a book by ISBN for the DSL lookupBook action. It
// follows the external feeds model (internal/feeds, #348 / #339): one fixed,
// key-less source (openBD), a URL built here from a checked ISBN only, the
// feeds client that refuses non-public addresses, a timeout and a body cap,
// a per-instance budget for upstream fetches, one fetch per ISBN at a time,
// a one-day cache and a kill switch. Nothing about the user is sent.
//
// docs/backend/features/book-lookup.md
package books

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
)

// Book is the body of GET /v1/lookup/book. Values are the source's, only
// trimmed of control characters and capped in length; publishedDate is the
// source's YYYYMMDD / YYYYMM / YYYY written as YYYY-MM-DD / YYYY-MM / YYYY.
type Book struct {
	ISBN          string    `json:"isbn"`
	Title         string    `json:"title"`
	Authors       string    `json:"authors,omitempty"` // as the source writes it, e.g. "山田玲子／著 水野菜生／英訳"
	Publisher     string    `json:"publisher,omitempty"`
	PublishedDate string    `json:"publishedDate,omitempty"`
	CoverURL      string    `json:"coverUrl,omitempty"` // https on the source's cover host only
	Source        Source    `json:"source"`
	FetchedAt     time.Time `json:"fetchedAt"`
}

// Source names where the data came from (shown with the filled form).
type Source struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	License string `json:"license"`
}

// Errors returned by Lookup.
var (
	ErrInvalidISBN = errors.New("books: not a valid ISBN-10 / ISBN-13")
	ErrNotFound    = errors.New("books: no record for this ISBN")
	ErrDisabled    = errors.New("books: lookup disabled")
	ErrBusy        = errors.New("books: outgoing request budget used up")
	ErrUpstream    = errors.New("books: source unavailable")
)

const (
	userAgent      = "TailorIt-books/1 (+https://github.com/jphacks/kz_2601)"
	requestTimeout = 8 * time.Second
	// maxBody: a full openBD record (ONIX + summary) is about 3-15 KiB.
	maxBody = 256 << 10
	// foundTTL / missTTL: a found book is kept a day; "no record" is kept a
	// little less so a newly registered book shows up the same day.
	foundTTL = 24 * time.Hour
	missTTL  = 6 * time.Hour
	failTTL  = time.Minute
	// maxEntries caps the cache (an entry is a few hundred bytes).
	maxEntries = 10000
	// DefaultUpstreamPerMinute caps fetches to the source per instance.
	DefaultUpstreamPerMinute = 30

	maxTitle     = 300
	maxAuthors   = 300
	maxPublisher = 120
)

// Endpoint is the only place the service fetches from. Tests point it at an
// httptest server; production uses DefaultEndpoint.
type Endpoint struct {
	GetURL    string // + "?isbn={13 digits}"
	CoverHost string // coverUrl is kept only on https://{CoverHost}/
}

// DefaultEndpoint: openBD v1 (terms checked 2026-11-08, book-lookup.md).
func DefaultEndpoint() Endpoint {
	return Endpoint{GetURL: "https://api.openbd.jp/v1/get", CoverHost: "cover.openbd.jp"}
}

// openBD: use is limited to introducing / promoting books; since 2023-08 the
// records come from the National Diet Library's CC BY 4.0 data (book-lookup.md).
var openBD = Source{Name: "openBD", URL: "https://openbd.jp/",
	License: "openBD の利用条件(本の紹介のため)・書誌は国立国会図書館の書誌データ(CC BY 4.0)を含む"}

// Options configures New. Zero values mean production defaults.
type Options struct {
	Client    *http.Client     // default: feeds.NewClient() (public addresses only)
	Endpoint  *Endpoint        // default: DefaultEndpoint()
	Now       func() time.Time // default: time.Now
	Disabled  bool             // kill switch
	PerMinute int              // 0 = DefaultUpstreamPerMinute
	Logger    *slog.Logger     // feeds.fetch lines; default: slog.Default()
}

// EnabledFromEnv: the lookup is off unless BOOK_LOOKUP_ENABLED is 1 / true
// (openBD's terms limit use to introducing books: owner decision 2026-11-08,
// book-lookup.md). BOOK_LOOKUP_DISABLED (1 / true) and FEEDS_DISABLED=all
// (every outgoing read) switch it off even then. appconfig (the endpoint) and
// ai (whether the AI may generate lookupBook) read the same switch.
func EnabledFromEnv(getenv func(string) string) bool {
	if !envOn(getenv("BOOK_LOOKUP_ENABLED")) || envOn(getenv("BOOK_LOOKUP_DISABLED")) {
		return false
	}
	for _, p := range strings.Split(getenv("FEEDS_DISABLED"), ",") {
		if strings.TrimSpace(p) == "all" {
			return false
		}
	}
	return true
}

func envOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// OptionsFromEnv: Disabled unless EnabledFromEnv. BOOK_LOOKUP_UPSTREAM_PER_MINUTE
// overrides the upstream budget.
func OptionsFromEnv(getenv func(string) string) Options {
	o := Options{Disabled: !EnabledFromEnv(getenv)}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(getenv("BOOK_LOOKUP_UPSTREAM_PER_MINUTE")), "%d", &n); err == nil && n > 0 {
		o.PerMinute = n
	}
	return o
}

// Service is safe for concurrent use.
type Service struct {
	client    *http.Client
	ep        Endpoint
	now       func() time.Time
	log       *slog.Logger // nil: slog.Default()
	disabled  bool
	perMinute int

	mu       sync.Mutex
	cache    map[string]*entry
	inflight map[string]chan struct{}
	window   time.Time
	used     int
}

type entry struct {
	book    *Book // nil = no record (or failed)
	failed  bool
	expires time.Time
}

// New builds a Service.
func New(o Options) *Service {
	s := &Service{client: o.Client, now: o.Now, log: o.Logger, disabled: o.Disabled, perMinute: o.PerMinute,
		cache: map[string]*entry{}, inflight: map[string]chan struct{}{}}
	if s.client == nil {
		s.client = feeds.NewClient()
	}
	if o.Endpoint != nil {
		s.ep = *o.Endpoint
	} else {
		s.ep = DefaultEndpoint()
	}
	c := *s.client
	c.CheckRedirect = s.checkRedirect
	s.client = &c
	if s.now == nil {
		s.now = time.Now
	}
	if s.perMinute <= 0 {
		s.perMinute = DefaultUpstreamPerMinute
	}
	return s
}

// Enabled reports whether the kill switch is off.
func (s *Service) Enabled() bool { return !s.disabled }

// Lookup returns the book for an ISBN-10 / ISBN-13. The ISBN is checked
// (digits and checksum) before anything else, so a bad one never leaves the
// server.
func (s *Service) Lookup(ctx context.Context, rawISBN string) (*Book, error) {
	isbn, ok := NormalizeISBN(rawISBN)
	if !ok {
		return nil, ErrInvalidISBN
	}
	if s.disabled {
		return nil, ErrDisabled
	}
	for {
		s.mu.Lock()
		now := s.now()
		if e := s.cache[isbn]; e != nil && now.Before(e.expires) {
			s.mu.Unlock()
			return e.result()
		}
		if ch := s.inflight[isbn]; ch != nil {
			s.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return nil, ErrUpstream
			}
		}
		if !s.takeBudget(now) {
			s.mu.Unlock()
			return nil, ErrBusy
		}
		ch := make(chan struct{})
		s.inflight[isbn] = ch
		s.mu.Unlock()

		// The fetch outlives the caller (requestTimeout bounds it): a client
		// that disconnects must not fail the waiters nor cache a failure.
		start := time.Now()
		book, n, err := s.fetch(context.WithoutCancel(ctx), isbn)
		// no ISBN in the line: "found" says whether openBD had a record
		feeds.LogFetch(ctx, s.log, feeds.FetchLine{Provider: Provider, Err: fetchErr(err), Bytes: n,
			Duration: time.Since(start), Attrs: []slog.Attr{slog.Bool("found", book != nil)}})

		s.mu.Lock()
		now = s.now()
		e := &entry{book: book}
		switch {
		case errors.Is(err, context.Canceled):
			// not the source's answer: nothing is cached, the waiters retry
			delete(s.inflight, isbn)
			close(ch)
			s.mu.Unlock()
			return nil, ErrUpstream
		case err == nil:
			e.expires = now.Add(foundTTL)
		case errors.Is(err, ErrNotFound):
			e.expires = now.Add(missTTL)
		default:
			e.failed, e.expires = true, now.Add(failTTL)
		}
		s.store(isbn, e, now)
		delete(s.inflight, isbn)
		close(ch)
		s.mu.Unlock()
		return e.result()
	}
}

func (e *entry) result() (*Book, error) {
	switch {
	case e.failed:
		return nil, ErrUpstream
	case e.book == nil:
		return nil, ErrNotFound
	}
	b := *e.book
	return &b, nil
}

// store keeps the cache under maxEntries: expired entries go first, then
// arbitrary ones (map order). Caller holds mu.
func (s *Service) store(isbn string, e *entry, now time.Time) {
	if len(s.cache) >= maxEntries {
		for k, old := range s.cache {
			if !now.Before(old.expires) {
				delete(s.cache, k)
			}
		}
		for k := range s.cache {
			if len(s.cache) < maxEntries {
				break
			}
			delete(s.cache, k)
		}
	}
	s.cache[isbn] = e
}

// takeBudget counts one upstream fetch against the per-minute cap. Caller holds mu.
func (s *Service) takeBudget(now time.Time) bool {
	if now.Sub(s.window) >= time.Minute {
		s.window, s.used = now, 0
	}
	if s.used >= s.perMinute {
		return false
	}
	s.used++
	return true
}

func (s *Service) host() string {
	u, err := url.Parse(s.ep.GetURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// checkRedirect: at most 3, each to the source's own scheme + host.
func (s *Service) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return fmt.Errorf("%w: too many redirects", feeds.ErrRedirectBlocked)
	}
	if req.URL.User != nil || req.URL.Scheme+"://"+req.URL.Host != s.host() {
		return fmt.Errorf("%w: %s", feeds.ErrRedirectBlocked, req.URL.Host)
	}
	return nil
}

// Provider is the provider label of the lookup's feeds.fetch lines.
const Provider = "books"

// fetchErr: "no record" is a working answer in the fetch line.
func fetchErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// fetch asks the source for one ISBN (13 checked digits: no other input
// reaches the URL) and reports the body size. The errors carry no ISBN, but
// a transport error (*url.Error) holds the URL: log feeds.FetchStatus only.
func (s *Service) fetch(ctx context.Context, isbn string) (*Book, int, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.ep.GetURL+"?isbn="+isbn, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, &feeds.StatusError{Code: resp.StatusCode}
	}
	if resp.ContentLength > maxBody {
		return nil, 0, fmt.Errorf("%w (%d bytes)", feeds.ErrTooLarge, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, len(body), err
	}
	if len(body) > maxBody {
		return nil, len(body), fmt.Errorf("%w (over %d bytes)", feeds.ErrTooLarge, maxBody)
	}
	book, err := s.parseOpenBD(body, isbn)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			err = fmt.Errorf("%w: %w", feeds.ErrBadResponse, err)
		}
		return nil, len(body), err
	}
	book.FetchedAt = s.now().UTC()
	return book, len(body), nil
}

// openBD v1 answers [record] or [null]; only summary is read.
type openBDRecord struct {
	Summary struct {
		ISBN      string `json:"isbn"`
		Title     string `json:"title"`
		Publisher string `json:"publisher"`
		PubDate   string `json:"pubdate"`
		Cover     string `json:"cover"`
		Author    string `json:"author"`
	} `json:"summary"`
}

func (s *Service) parseOpenBD(body []byte, isbn string) (*Book, error) {
	var recs []*openBDRecord
	if err := json.Unmarshal(body, &recs); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(recs) == 0 || recs[0] == nil {
		return nil, ErrNotFound
	}
	sm := recs[0].Summary
	title := clean(sm.Title, maxTitle)
	if title == "" {
		return nil, ErrNotFound
	}
	if got, ok := NormalizeISBN(sm.ISBN); ok && got != isbn {
		return nil, errors.New("record is for another ISBN")
	}
	return &Book{
		ISBN:          isbn,
		Title:         title,
		Authors:       clean(sm.Author, maxAuthors),
		Publisher:     clean(sm.Publisher, maxPublisher),
		PublishedDate: pubDate(sm.PubDate),
		CoverURL:      s.coverURL(sm.Cover),
		Source:        openBD,
	}, nil
}

// pubDate writes YYYYMMDD / YYYYMM / YYYY as YYYY-MM-DD / YYYY-MM / YYYY;
// anything else (free text, out of range) is dropped.
func pubDate(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, r := range raw {
		if r < '0' || r > '9' {
			return ""
		}
	}
	var layout, out string
	switch len(raw) {
	case 8:
		layout, out = "20060102", "2006-01-02"
	case 6:
		layout, out = "200601", "2006-01"
	case 4:
		layout, out = "2006", "2006"
	default:
		return ""
	}
	t, err := time.Parse(layout, raw)
	if err != nil || t.Year() < 1000 {
		return ""
	}
	return t.Format(out)
}

func (s *Service) coverURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" || u.Scheme != "https" || u.Host != s.ep.CoverHost || u.User != nil {
		return ""
	}
	return u.String()
}

// clean drops control / invisible characters, folds whitespace and caps the
// length in runes.
func clean(s string, maxRunes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > maxRunes {
		out = string([]rune(out)[:maxRunes])
	}
	return out
}
