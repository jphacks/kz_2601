package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Shared-app usage (#2298, -report shared): the shared.usage lines summed per
// week, per participant (uid_hash) and per shared app (app_hash). It counts
// what participants cause, to decide #1450; nothing is charged.

const sharedEvent = "shared.usage"

// sharedKinds are the kinds counted, in table order.
var sharedKinds = []string{"read", "write", "notify"}

type usageLine struct {
	At    time.Time
	UID   string
	App   string
	Kind  string
	Count int
}

func parseShared(r io.Reader) ([]usageLine, error) {
	entries, err := readEntries(r)
	if err != nil {
		return nil, err
	}
	var out []usageLine
	for _, e := range entries {
		p, ts := e.JSONPayload, e.Timestamp
		if p == nil {
			p, ts = &e.payload, e.Time
		}
		if p.Event != sharedEvent || p.UIDHash == "" || p.AppHash == "" || p.Count <= 0 || !knownKind(p.Kind) {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		out = append(out, usageLine{At: at, UID: p.UIDHash, App: p.AppHash, Kind: p.Kind, Count: p.Count})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func knownKind(k string) bool {
	for _, x := range sharedKinds {
		if x == k {
			return true
		}
	}
	return false
}

// sizeBuckets group shared apps by their participants that week.
var sizeBuckets = []struct {
	Name     string
	Min, Max int // Max 0 = no upper bound
}{{"1", 1, 1}, {"2-5", 2, 5}, {"6-20", 6, 20}, {"21-100", 21, 100}, {"101+", 101, 0}}

// sharedWeek is one week: per kind, the participants' counts (for the
// distribution) and the totals per app size.
type sharedWeek struct {
	Participants int
	PerUser      map[string][]float64 // kind -> one count per participant with any
	Total        map[string]int       // kind -> sum
	Apps         []int                // per bucket
	BucketTotal  []map[string]int     // per bucket, kind -> sum
}

type sharedReport struct {
	Weeks []time.Time
	Week  map[time.Time]*sharedWeek
}

func aggregateShared(lines []usageLine) *sharedReport {
	r := &sharedReport{Week: map[time.Time]*sharedWeek{}}
	if len(lines) == 0 {
		return r
	}
	perUser := map[time.Time]map[string]map[string]int{} // week -> uid -> kind -> n
	perApp := map[time.Time]map[string]map[string]int{}  // week -> app -> kind -> n
	members := map[time.Time]map[string]map[string]bool{}
	add := func(m map[time.Time]map[string]map[string]int, w time.Time, id, kind string, n int) {
		if m[w] == nil {
			m[w] = map[string]map[string]int{}
		}
		if m[w][id] == nil {
			m[w][id] = map[string]int{}
		}
		m[w][id][kind] += n
	}
	for _, l := range lines {
		w := week(l.At)
		add(perUser, w, l.UID, l.Kind, l.Count)
		add(perApp, w, l.App, l.Kind, l.Count)
		if members[w] == nil {
			members[w] = map[string]map[string]bool{}
		}
		if members[w][l.App] == nil {
			members[w][l.App] = map[string]bool{}
		}
		members[w][l.App][l.UID] = true
	}
	for w := week(lines[0].At); !w.After(week(lines[len(lines)-1].At)); w = w.AddDate(0, 0, 7) {
		r.Weeks = append(r.Weeks, w)
		sw := &sharedWeek{PerUser: map[string][]float64{}, Total: map[string]int{},
			Apps: make([]int, len(sizeBuckets)), BucketTotal: make([]map[string]int, len(sizeBuckets))}
		for i := range sw.BucketTotal {
			sw.BucketTotal[i] = map[string]int{}
		}
		sw.Participants = len(perUser[w])
		for _, kinds := range perUser[w] {
			for k, n := range kinds {
				sw.PerUser[k] = append(sw.PerUser[k], float64(n))
				sw.Total[k] += n
			}
		}
		for app, kinds := range perApp[w] {
			b := bucketOf(len(members[w][app]))
			sw.Apps[b]++
			for k, n := range kinds {
				sw.BucketTotal[b][k] += n
			}
		}
		r.Week[w] = sw
	}
	return r
}

func bucketOf(n int) int {
	for i, b := range sizeBuckets {
		if n >= b.Min && (b.Max == 0 || n <= b.Max) {
			return i
		}
	}
	return 0
}

func num(xs []float64, p float64) string {
	v, ok := quantile(xs, p)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.0f", v)
}

func renderShared(r *sharedReport) string {
	if len(r.Weeks) == 0 {
		return "共有のアプリの記録がありません(shared.usage・SHARED_USAGE_LOG=on の時だけ出る)。\n"
	}
	var b strings.Builder
	f := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	wk := func(t time.Time) string { return t.Format("2006-01-02") }
	f("# 共有のアプリの使われ方(週ごと・月曜 0:00 JST 始まり)\n\n")
	f("読み取り(read)は 1 回の取得で読んだ記録の数、書き込み(write)は作った・変えた・消した記録の数、通知(notify)は届いた宛先付きの通知の数。\n\n")
	f("## 参加者 1 人あたりの回数(その種類を 1 回以上した人の中の分布)\n\n")
	f("| 週 | 参加者 | 種類 | した人 | 合計 | p50 | p90 | 最大 |\n|---|---|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		w := r.Week[t]
		for _, k := range sharedKinds {
			xs := w.PerUser[k]
			f("| %s | %d | %s | %d | %d | %s | %s | %s |\n", wk(t), w.Participants, k, len(xs), w.Total[k], num(xs, 0.5), num(xs, 0.9), num(xs, 1))
		}
	}
	f("\n## 参加者の人数ごとの合計(その週に記録があった参加者の数でアプリを分ける)\n\n")
	f("| 週 | 人数 | アプリ | read | write | notify |\n|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		w := r.Week[t]
		for i, sb := range sizeBuckets {
			if w.Apps[i] == 0 {
				continue
			}
			bt := w.BucketTotal[i]
			f("| %s | %s | %d | %d | %d | %d |\n", wk(t), sb.Name, w.Apps[i], bt["read"], bt["write"], bt["notify"])
		}
	}
	return b.String()
}
