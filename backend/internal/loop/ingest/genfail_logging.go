package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	logging "google.golang.org/api/logging/v2"
	"google.golang.org/api/option"
)

// GenLogAPI is a GenLogSource over the Cloud Logging API (entries.list,
// #2146). It asks only for the ai.generate lines of the given Cloud Run
// services in [since, until], newest first, and stops at MaxEntries. The
// reader needs roles/logging.viewer only; credentials are the job's ADC.
type GenLogAPI struct {
	Project  string
	Services []string
	// Lister: nil = a Cloud Logging client built from ADC on first use.
	Lister LogEntryLister
	// MaxEntries bounds one read. Default DefaultGenLogMaxEntries.
	MaxEntries int
	// PageSize per entries.list call (<= 1000). Default 1000.
	PageSize int
}

// DefaultGenLogMaxEntries matches `gcloud logging read`'s usual LIMIT.
const DefaultGenLogMaxEntries = 50000

// maxGenLogPages stops a read that keeps getting empty pages with a token.
const maxGenLogPages = 200

// LogListRequest is one entries.list call.
type LogListRequest struct {
	ResourceNames []string
	Filter        string
	OrderBy       string
	PageSize      int
	PageToken     string
}

// LogListPage is one entries.list page: each entry as JSON (LogEntry shape).
type LogListPage struct {
	Entries []json.RawMessage
	Next    string
}

// LogEntryLister is the entries.list call (LoggingLister; a fake in tests).
type LogEntryLister interface {
	ListEntries(ctx context.Context, req LogListRequest) (LogListPage, error)
}

// LoggingLister implements LogEntryLister with logging/v2.
type LoggingLister struct{ Svc *logging.Service }

// NewLoggingLister builds a read-only client from ADC. opts come after the
// scope (tests: option.WithEndpoint / option.WithoutAuthentication).
func NewLoggingLister(ctx context.Context, opts ...option.ClientOption) (*LoggingLister, error) {
	all := append([]option.ClientOption{option.WithScopes(logging.LoggingReadScope)}, opts...)
	svc, err := logging.NewService(ctx, all...)
	if err != nil {
		return nil, fmt.Errorf("cloud logging: init: %w", err)
	}
	return &LoggingLister{Svc: svc}, nil
}

// ListEntries implements LogEntryLister. Errors keep the status only.
func (l *LoggingLister) ListEntries(ctx context.Context, req LogListRequest) (LogListPage, error) {
	resp, err := l.Svc.Entries.List(&logging.ListLogEntriesRequest{
		ResourceNames: req.ResourceNames, Filter: req.Filter, OrderBy: req.OrderBy,
		PageSize: int64(req.PageSize), PageToken: req.PageToken,
	}).Context(ctx).Do()
	if err != nil {
		return LogListPage{}, fmt.Errorf("cloud logging: %w", err)
	}
	page := LogListPage{Next: resp.NextPageToken, Entries: make([]json.RawMessage, 0, len(resp.Entries))}
	for _, e := range resp.Entries {
		raw, err := json.Marshal(struct {
			Timestamp   string          `json:"timestamp"`
			JSONPayload json.RawMessage `json:"jsonPayload,omitempty"`
		}{e.Timestamp, json.RawMessage(e.JsonPayload)})
		if err != nil {
			continue
		}
		page.Entries = append(page.Entries, raw)
	}
	return page, nil
}

// GenLogFilter is the entries.list filter: ai.generate lines of services in
// [since, until]. All lines are read, not only failures: the detector needs
// every line as the denominator of a rate.
func GenLogFilter(services []string, since, until time.Time) string {
	q := make([]string, len(services))
	for i, s := range services {
		q[i] = strconv.Quote(s)
	}
	return fmt.Sprintf(`resource.type="cloud_run_revision" AND resource.labels.service_name=(%s) AND jsonPayload.event=%q AND timestamp>=%q AND timestamp<=%q`,
		strings.Join(q, " OR "), genEvent, since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano))
}

// GenerateEvents implements GenLogSource.
func (g GenLogAPI) GenerateEvents(ctx context.Context, since, until time.Time) ([]GenEvent, error) {
	if g.Project == "" || len(g.Services) == 0 {
		return nil, errors.New("cloud logging: no project or services")
	}
	lister := g.Lister
	if lister == nil {
		l, err := NewLoggingLister(ctx)
		if err != nil {
			return nil, err
		}
		lister = l
	}
	limit := g.MaxEntries
	if limit <= 0 {
		limit = DefaultGenLogMaxEntries
	}
	size := g.PageSize
	if size <= 0 || size > 1000 {
		size = 1000
	}
	req := LogListRequest{
		ResourceNames: []string{"projects/" + g.Project},
		Filter:        GenLogFilter(g.Services, since, until),
		OrderBy:       "timestamp desc", // newest first: a cut keeps the window
	}
	out := []GenEvent{}
	read := 0
	for pages := 0; ; pages++ {
		if pages >= maxGenLogPages {
			slog.Warn("loop ingest: generation-failure log read hit the page cap", "pages", pages)
			break
		}
		req.PageSize = min(size, limit-read)
		page, err := lister.ListEntries(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, raw := range page.Entries {
			read++
			if ev, ok := decodeGenEntry(raw); ok {
				out = append(out, ev)
			}
		}
		if page.Next == "" {
			break
		}
		if read >= limit {
			slog.Warn("loop ingest: generation-failure log read hit the cap; the oldest baseline lines are skipped", "limit", limit)
			break
		}
		req.PageToken = page.Next
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return genInRange(out, since, until), nil
}
