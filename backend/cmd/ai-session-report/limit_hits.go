package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// limit-hits (#2296): per week, how many people got a 429 (the daily limit,
// rate_limited) on /v1/generate or /v1/chat, counted by uid_hash, and how many
// chat turns / creates / edits they made that JST day before the first 429.
// Input for choosing the limit (#1771); prints counts only.
func init() {
	registerLogMode("limit-hits", logMode{
		summary: "people who reached the daily limit (429) per week and the chat / create / edit counts before it",
		run:     runLimitHits,
	})
}

func runLimitHits(args []string, stdout io.Writer) error {
	fs := newModeFlags("limit-hits", os.Stderr)
	var logs listFlag
	var per string
	fs.Var(&logs, "logs", "`gcloud logging read --format=json` exports with http.request and ai.generate lines (comma-separated or repeated)")
	fs.StringVar(&per, "period", "", "YYYY-MM-DD..YYYY-MM-DD in JST, days included (weeks start on Monday)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("limit-hits: unexpected argument %q", fs.Arg(0))
	}
	if per == "" {
		return errors.New("limit-hits: -period is required")
	}
	p, err := parsePeriod(per)
	if err != nil {
		return err
	}
	evs, err := readAllLogs(logs)
	if err != nil {
		return err
	}
	renderLimitHits(stdout, p, tallyLimitHits(evs, p))
	return nil
}

// limitPath reports the AI endpoints with a daily limit.
func limitPath(path string) bool { return path == "/v1/generate" || path == "/v1/chat" }

// limitDay is one user's JST day up to the first 429 (or the whole day).
type limitDay struct {
	Chats, Creates, Edits int
	Limited               int // 429 responses that day
}

func (d *limitDay) hit() bool { return d.Limited > 0 }

// limitWeek is one Monday-start JST week.
type limitWeek struct {
	Start  time.Time
	Users  map[string]bool // used /v1/generate or /v1/chat
	Hit    map[string]bool // got a 429 there
	Days   []limitDay      // the days that ended in a 429
	Status int             // 429 responses
}

// weekStart is the Monday 00:00 JST of t's week.
func weekStart(t time.Time) time.Time {
	t = t.In(jst)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, jst)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// tallyLimitHits walks every line in p. Counts before the first 429 of the
// user's JST day: chat = a /v1/chat request answered below 400, create /
// edit = ai.generate with mode create / edit (any outcome: each one spends
// the limit).
func tallyLimitHits(evs []logEvent, p period) []*limitWeek {
	type dayKey struct{ uid, day string }
	days := map[dayKey]*limitDay{}
	dayWeek := map[dayKey]time.Time{}
	weeks := map[time.Time]*limitWeek{}
	week := func(t time.Time) *limitWeek {
		s := weekStart(t)
		if weeks[s] == nil {
			weeks[s] = &limitWeek{Start: s, Users: map[string]bool{}, Hit: map[string]bool{}}
		}
		return weeks[s]
	}
	for _, e := range evs {
		if e.UIDHash == "" || !p.has(e.At) {
			continue
		}
		k := dayKey{e.UIDHash, e.At.In(jst).Format(time.DateOnly)}
		d := days[k]
		if d == nil {
			d = &limitDay{}
			days[k] = d
			dayWeek[k] = weekStart(e.At)
		}
		w := week(e.At)
		switch {
		case e.Event == evRequest && limitPath(e.Path):
			w.Users[e.UIDHash] = true
			switch {
			case e.Status == 429:
				d.Limited++
				w.Status++
				w.Hit[e.UIDHash] = true
			case e.Path == "/v1/chat" && e.Status > 0 && e.Status < 400 && !d.hit():
				d.Chats++
			}
		case e.Event == evGenerate && !d.hit():
			switch e.Mode {
			case "create":
				d.Creates++
			case "edit":
				d.Edits++
			}
		}
	}
	keys := make([]dayKey, 0, len(days))
	for k, d := range days {
		if d.hit() {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].day != keys[j].day {
			return keys[i].day < keys[j].day
		}
		return keys[i].uid < keys[j].uid
	})
	for _, k := range keys {
		w := weeks[dayWeek[k]]
		w.Days = append(w.Days, *days[k])
	}
	// every week of the period, empty ones too
	var out []*limitWeek
	for s := weekStart(p.From); s.Before(p.To); s = s.AddDate(0, 0, 7) {
		w := weeks[s]
		if w == nil {
			w = &limitWeek{Start: s}
		}
		out = append(out, w)
	}
	return out
}

// nearestRank is the q-th percentile (0 < q <= 1) by the nearest-rank method.
func nearestRank(xs []int, q float64) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	i := int(math.Ceil(q*float64(len(s)))) - 1
	return s[max(i, 0)]
}

// dist is "p50 / p90 / max" of the days' counts, or "-" with no day.
func dist(days []limitDay, f func(limitDay) int) string {
	if len(days) == 0 {
		return "-"
	}
	xs := make([]int, len(days))
	for i, d := range days {
		xs[i] = f(d)
	}
	return fmt.Sprintf("%d / %d / %d", nearestRank(xs, 0.5), nearestRank(xs, 0.9), nearestRank(xs, 1))
}

func renderLimitHits(w io.Writer, p period, weeks []*limitWeek) {
	_, _ = fmt.Fprintf(w, "## daily limit (429) by week: %s\n\n", p.Label)
	_, _ = fmt.Fprintln(w, "| week (Mon, JST) | users | reached | reached days | 429s | chats before (p50 / p90 / max) | creates before | edits before | total before |")
	_, _ = fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, wk := range weeks {
		_, _ = fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %s | %s | %s | %s |\n",
			wk.Start.Format(time.DateOnly), len(wk.Users), len(wk.Hit), len(wk.Days), wk.Status,
			dist(wk.Days, func(d limitDay) int { return d.Chats }),
			dist(wk.Days, func(d limitDay) int { return d.Creates }),
			dist(wk.Days, func(d limitDay) int { return d.Edits }),
			dist(wk.Days, func(d limitDay) int { return d.Chats + d.Creates + d.Edits }))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, strings.Join([]string{
		"users: distinct uid_hash that called /v1/generate or /v1/chat; reached: of them, got a 429 there.",
		"reached days: user × JST day with a 429; the counts are that day's before the first 429 (chat = /v1/chat < 400, create / edit = ai.generate mode).",
		"A week cut by -period counts only its days inside the period.",
	}, "\n"))
}
