package runner_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// feedRules: a private app whose daily rule saves the pressure and the
// weather of 東京都.
const feedRules = `{
  "schemaVersion": 1,
  "app": {"name": "気圧", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "plog", "name": "気圧", "fields": [
      {"id": "day", "name": "日", "type": "date", "required": true},
      {"id": "note", "name": "メモ", "type": "string"},
      {"id": "low", "name": "最低", "type": "number"},
      {"id": "high", "name": "最高", "type": "number"},
      {"id": "falling", "name": "低下", "type": "boolean"}
    ]},
    {"id": "wlog", "name": "天気", "fields": [
      {"id": "day", "name": "日", "type": "date", "required": true},
      {"id": "weather", "name": "天気", "type": "string"},
      {"id": "high", "name": "最高", "type": "integer"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "気圧", "components": [{"type": "list", "entity": "plog", "item": {"title": "note"}}]}
  ],
  "serverRules": [
    {"id": "save", "on": {"every": "daily", "time": "07:00"},
     "do": [{"saveFeed": {"provider": "pressure", "region": "130000", "entity": "plog", "fields": {"date": "day", "summary": "note", "min": "low", "max": "high", "drop": "falling"}}},
       {"saveFeed": {"provider": "weather", "region": "130000", "entity": "wlog", "fields": {"date": "day", "summary": "weather", "max": "high"}}}]}
  ]
}`

// fakeFeeds answers like feeds.Service and records the queries.
type fakeFeeds struct {
	mu   sync.Mutex
	got  []feeds.Query
	err  error
	resp map[string]*feeds.Result
}

func (f *fakeFeeds) Get(_ context.Context, q feeds.Query, _ int) (*feeds.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, q)
	if f.err != nil {
		return nil, f.err
	}
	return f.resp[q.Provider], nil
}

func fp(v float64) *float64 { return &v }

func sampleFeeds() *fakeFeeds {
	yes := true
	return &fakeFeeds{resp: map[string]*feeds.Result{
		"pressure": {Provider: "pressure", Items: []feeds.Item{
			{Title: "6時〜12時", Min: fp(1010.2), Max: fp(1013.4)},
			{Title: "12時〜18時", Min: fp(1003.1), Max: fp(1009.8), Drop: &yes, Subtitle: "24 時間で 7 hPa 下がります"},
		}},
		"weather": {Provider: "weather", Items: []feeds.Item{{Date: "2026-11-08", Title: "くもり", Min: fp(9), Max: fp(16.6)}}},
	}}
}

// A daily saveFeed rule saves one record per provider with the snapshot,
// dated the run's day; only the provider and the fixed region are asked.
func TestRunnerSaveFeed(t *testing.T) {
	f := sampleFeeds()
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEED"+h.sfx, "uid-feed"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Asia/Tokyo")
		h.tick()
		due := h.oneSchedule(id).NextRunAt
		if res := h.tickAt(due); res.Done != 1 {
			t.Fatalf("tick: %+v", res)
		}
		day := due.In(schedule.Location("Asia/Tokyo")).Format("2006-01-02")
		plog := h.records(store.PrivateRecords(uid, id, "plog"))
		if len(plog) != 1 {
			t.Fatalf("plog: %+v", plog)
		}
		d := plog[0].Data
		if d["note"] != "24 時間で 7 hPa 下がります" || num(d["low"]) != 1003.1 || num(d["high"]) != 1013.4 || d["falling"] != true || d["day"] != day ||
			plog[0].Origin == nil || plog[0].Origin.RuleID != "save" {
			t.Fatalf("plog record: %+v", plog[0])
		}
		wlog := h.records(store.PrivateRecords(uid, id, "wlog"))
		if len(wlog) != 1 || wlog[0].Data["weather"] != "くもり" || num(wlog[0].Data["high"]) != 17 || wlog[0].Data["low"] != nil {
			t.Fatalf("wlog: %+v", wlog)
		}
		f.mu.Lock()
		for _, q := range f.got {
			if q.Region != "130000" || q.FeedID != "" || q.Q != "" {
				t.Errorf("query carries more than the region: %+v", q)
			}
		}
		f.mu.Unlock()
		if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK {
			t.Fatalf("events: %+v", evs)
		}
	})
}

// A provider switched off (FEEDS_DISABLED) saves nothing; the run is ok.
func TestRunnerSaveFeedDisabled(t *testing.T) {
	f := &fakeFeeds{err: feeds.ErrDisabled}
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDOFF"+h.sfx, "uid-feedoff"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Asia/Tokyo")
		h.tick()
		if res := h.tickAt(h.oneSchedule(id).NextRunAt); res.Done != 1 {
			t.Fatalf("tick: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "plog"))); n != 0 {
			t.Fatalf("plog: %d records", n)
		}
		if evs := h.scheduleEvents(id); len(evs) != 1 || evs[0].Outcome != quota.OutcomeOK {
			t.Fatalf("events: %+v", evs)
		}
	})
}

// A failing source retries the run (nothing saved yet).
func TestRunnerSaveFeedRetries(t *testing.T) {
	f := &fakeFeeds{err: feeds.ErrUpstream}
	stores(t, options{feeds: f}, func(t *testing.T, h *harness) {
		id, uid := "FEEDERR"+h.sfx, "uid-feederr"+h.sfx
		h.putDSLTZ(uid, "/v1/apps/"+id, []byte(feedRules), "Asia/Tokyo")
		h.tick()
		if res := h.tickAt(h.oneSchedule(id).NextRunAt); res.Done != 0 {
			t.Fatalf("tick: %+v", res)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "plog"))); n != 0 {
			t.Fatalf("plog: %d records", n)
		}
	})
}
