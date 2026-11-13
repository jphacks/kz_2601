// product-metrics turns the product-metric log events (#297) into weekly
// Markdown tables: generation success, edits per app, time to first
// generation, D1/D7 return and share rate. Weeks start Monday 0:00 JST.
// Reads only the log export; no API, Firestore or GCP calls.
//
// Input: `gcloud logging read --format=json` output (a JSON array of entries
// with timestamp + jsonPayload), or JSON lines of entries or of bare slog
// records (`time` + attrs, e.g. a local `make dev` log).
//
//	go run ./cmd/product-metrics -in logs.json
//	scripts/product-metrics-weekly.sh prod   # reads Cloud Logging, then runs this
//
// Definitions: docs/metrics/product-metrics.md
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

var jst = time.FixedZone("JST", 9*60*60)

// event is the subset of a metric line the tables use.
type event struct {
	At      time.Time
	Event   string
	UID     string // uid_hash
	App     string // app_hash
	Mode    string
	Outcome string
	Passed  bool
}

type payload struct {
	Event            string `json:"event"`
	UIDHash          string `json:"uid_hash"`
	AppHash          string `json:"app_hash"`
	Mode             string `json:"mode"`
	Outcome          string `json:"outcome"`
	ValidationPassed bool   `json:"validation_passed"`
	Kind             string `json:"kind"`  // shared.usage
	Count            int    `json:"count"` // shared.usage
	Time             string `json:"time"`  // bare slog record
}

type entry struct {
	Timestamp   string   `json:"timestamp"`
	JSONPayload *payload `json:"jsonPayload"`
	payload              // bare slog record: attrs at the top level
}

// metricEvents are the lines counted; anything else in the input is ignored.
var metricEvents = map[string]bool{
	"ai.generate": true, "session.start": true, "app.opened": true,
	"app.shared": true, "app.published": true, "share.imported": true,
}

func parse(r io.Reader) ([]event, error) {
	entries, err := readEntries(r)
	if err != nil {
		return nil, err
	}
	var out []event
	for _, e := range entries {
		p, ts := e.JSONPayload, e.Timestamp
		if p == nil {
			p, ts = &e.payload, e.Time
		}
		if !metricEvents[p.Event] || p.UIDHash == "" {
			continue // anonymous lines cannot be attributed to a user
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		out = append(out, event{At: at, Event: p.Event, UID: p.UIDHash, App: p.AppHash, Mode: p.Mode,
			Outcome: p.Outcome, Passed: p.ValidationPassed})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// readEntries decodes a JSON array of entries or JSON lines (entries or bare
// slog records); lines that do not decode are skipped.
func readEntries(r io.Reader) ([]entry, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var entries []entry
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(t, &entries); err != nil {
			return nil, fmt.Errorf("decode JSON array: %w", err)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			var e entry
			if json.Unmarshal(line, &e) == nil {
				entries = append(entries, e)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func day(t time.Time) time.Time {
	y, m, d := t.In(jst).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, jst)
}

func week(t time.Time) time.Time {
	d := day(t)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// built: a create/edit that produced an app (passed validation and changed it).
func built(e event) bool {
	return e.Event == "ai.generate" && (e.Mode == "create" || e.Mode == "edit") &&
		(e.Outcome == "validated" || e.Outcome == "regenerated")
}

// genRow is one week of ai.generate (create/edit, caller errors excluded).
type genRow struct {
	N, Validated, Regenerated, Unchanged, Refused, Failed int
}

type editRow struct {
	Apps, Edits int
	PerApp      []int
}

type newRow struct {
	Users, Generated int
	Minutes          []float64
}

type retRow struct{ Users, D1, D7, D1Due, D7Due int }

type shareRow struct {
	BuiltUsers, SharedUsers, Shares, Imports, Published int
}

type report struct {
	From, To time.Time // first / last event
	Weeks    []time.Time
	Gen      map[time.Time]*genRow
	Edit     map[time.Time]*editRow
	New      map[time.Time]*newRow
	Ret      map[time.Time]*retRow
	Share    map[time.Time]*shareRow
}

func get[T any](m map[time.Time]*T, k time.Time) *T {
	if v, ok := m[k]; ok {
		return v
	}
	v := new(T)
	m[k] = v
	return v
}

func aggregate(evs []event) *report {
	r := &report{Gen: map[time.Time]*genRow{}, Edit: map[time.Time]*editRow{}, New: map[time.Time]*newRow{},
		Ret: map[time.Time]*retRow{}, Share: map[time.Time]*shareRow{}}
	if len(evs) == 0 {
		return r
	}
	r.From, r.To = evs[0].At, evs[len(evs)-1].At
	lastDay := day(r.To)

	firstSeen := map[string]time.Time{}
	firstBuild := map[string]time.Time{} // first create that built an app
	day0 := map[string]time.Time{}       // first day with any built app
	active := map[string]map[time.Time]bool{}
	edits := map[time.Time]map[string]int{} // week -> app -> edits
	builtUsers := map[time.Time]map[string]bool{}
	sharedUsers := map[time.Time]map[string]bool{}
	set := func(m map[time.Time]map[string]bool, w time.Time, u string) {
		if m[w] == nil {
			m[w] = map[string]bool{}
		}
		m[w][u] = true
	}

	for _, e := range evs {
		w, d := week(e.At), day(e.At)
		if _, ok := firstSeen[e.UID]; !ok {
			firstSeen[e.UID] = e.At
		}
		if active[e.UID] == nil {
			active[e.UID] = map[time.Time]bool{}
		}
		active[e.UID][d] = true
		switch e.Event {
		case "ai.generate":
			if e.Mode != "create" && e.Mode != "edit" || e.Outcome == "bad_request" {
				break
			}
			g := get(r.Gen, w)
			g.N++
			switch e.Outcome {
			case "validated":
				g.Validated++
			case "regenerated":
				g.Regenerated++
			case "unchanged":
				g.Unchanged++
			case "refused":
				g.Refused++
			default:
				g.Failed++
			}
			if !built(e) {
				break
			}
			set(builtUsers, w, e.UID)
			if _, ok := day0[e.UID]; !ok {
				day0[e.UID] = d
			}
			if e.Mode == "create" {
				if _, ok := firstBuild[e.UID]; !ok {
					firstBuild[e.UID] = e.At
				}
			} else if e.App != "" {
				if edits[w] == nil {
					edits[w] = map[string]int{}
				}
				edits[w][e.App]++
			}
		case "app.shared":
			get(r.Share, w).Shares++
			set(sharedUsers, w, e.UID)
		case "share.imported":
			get(r.Share, w).Imports++
		case "app.published":
			get(r.Share, w).Published++
		}
	}

	for w, apps := range edits {
		er := get(r.Edit, w)
		for _, n := range apps {
			er.Apps++
			er.Edits += n
			er.PerApp = append(er.PerApp, n)
		}
	}
	for u, t0 := range firstSeen {
		nr := get(r.New, week(t0))
		nr.Users++
		if t1, ok := firstBuild[u]; ok {
			nr.Generated++
			nr.Minutes = append(nr.Minutes, t1.Sub(t0).Minutes())
		}
	}
	for u, d0 := range day0 {
		rr := get(r.Ret, week(d0))
		rr.Users++
		if d1 := d0.AddDate(0, 0, 1); !d1.After(lastDay) {
			rr.D1Due++
			if active[u][d1] {
				rr.D1++
			}
		}
		if d7 := d0.AddDate(0, 0, 7); !d7.After(lastDay) {
			rr.D7Due++
			if active[u][d7] {
				rr.D7++
			}
		}
	}
	for w, users := range builtUsers {
		sr := get(r.Share, w)
		sr.BuiltUsers = len(users)
		for u := range sharedUsers[w] {
			if users[u] {
				sr.SharedUsers++
			}
		}
	}

	for w := week(r.From); !w.After(week(r.To)); w = w.AddDate(0, 0, 7) {
		r.Weeks = append(r.Weeks, w)
	}
	return r
}

func pct(n, d int) string {
	if d == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", float64(n)*100/float64(d))
}

// quantile is nearest-rank (same as scripts/beta-weekly.sh).
func quantile(xs []float64, p float64) (float64, bool) {
	if len(xs) == 0 {
		return 0, false
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(float64(len(s))*p+0.999999) - 1
	return s[max(0, min(i, len(s)-1))], true
}

func mins(xs []float64, p float64) string {
	v, ok := quantile(xs, p)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.1f", v)
}

// render returns the Markdown tables.
func render(r *report) string {
	if len(r.Weeks) == 0 {
		return "指標のイベントがありません(uid_hash 付きの ai.generate / session.start / app.* / share.imported)。\n"
	}
	var b strings.Builder
	f := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	wk := func(t time.Time) string { return t.Format("2006-01-02") }
	first := week(r.From)
	mark := func(t time.Time) string {
		if t.Equal(first) {
			return wk(t) + " *"
		}
		return wk(t)
	}

	f("# プロダクトの指標(週ごと・月曜 0:00 JST 始まり)\n\n")
	f("データ: %s 〜 %s(JST)。* はデータの始まりの週(それ以前からいるユーザーが「新しい」に混ざる)。\n\n",
		r.From.In(jst).Format("2006-01-02 15:04"), r.To.In(jst).Format("2006-01-02 15:04"))

	f("## 生成の成功率(ai.generate・create と edit・bad_request を除く)\n\n")
	f("| 週 | 生成 | 成功率 | 1 回で通過 | 作り直して通過 | 変更なし | 断った | 失敗 |\n|---|---|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		g := get(r.Gen, t)
		f("| %s | %d | %s | %s | %s | %s | %s | %s |\n", wk(t), g.N, pct(g.Validated+g.Regenerated+g.Unchanged, g.N),
			pct(g.Validated, g.N), pct(g.Regenerated, g.N), pct(g.Unchanged, g.N), pct(g.Refused, g.N), pct(g.Failed, g.N))
	}

	f("\n## 直しの回数(アプリごと・edit で変更が入った回数)\n\n")
	f("| 週 | 直したアプリ | 直しの合計 | 平均 | 中央値 |\n|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		e := get(r.Edit, t)
		avg := "—"
		if e.Apps > 0 {
			avg = fmt.Sprintf("%.1f", float64(e.Edits)/float64(e.Apps))
		}
		per := make([]float64, len(e.PerApp))
		for i, n := range e.PerApp {
			per[i] = float64(n)
		}
		f("| %s | %d | %d | %s | %s |\n", wk(t), e.Apps, e.Edits, avg, mins(per, 0.5))
	}

	f("\n## 初回の生成までの時間(最初に見えた時から、最初に create で作れた時まで)\n\n")
	f("| 週(最初に見えた週) | 新しいユーザー | 作れた | 作れた率 | p50(分) | p90(分) |\n|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		n := get(r.New, t)
		f("| %s | %d | %d | %s | %s | %s |\n", mark(t), n.Users, n.Generated, pct(n.Generated, n.Users), mins(n.Minutes, 0.5), mins(n.Minutes, 0.9))
	}

	f("\n## 翌日・7 日後に開いた率(最初に作れた日を 0 日目とするコホート)\n\n")
	f("| 週(0 日目の週) | ユーザー | D1 | D7 | D1 の対象 | D7 の対象 |\n|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		x := get(r.Ret, t)
		f("| %s | %d | %s | %s | %d | %d |\n", mark(t), x.Users, pct(x.D1, x.D1Due), pct(x.D7, x.D7Due), x.D1Due, x.D7Due)
	}

	f("\n## 共有した率(その週に作れたユーザーのうち共有リンクを作った人)\n\n")
	f("| 週 | 作れたユーザー | 共有したユーザー | 共有した率 | 共有リンク | 取り込み | 「探す」に掲載 |\n|---|---|---|---|---|---|---|\n")
	for _, t := range r.Weeks {
		s := get(r.Share, t)
		f("| %s | %d | %d | %s | %d | %d | %d |\n", wk(t), s.BuiltUsers, s.SharedUsers, pct(s.SharedUsers, s.BuiltUsers), s.Shares, s.Imports, s.Published)
	}
	f("\nPro の確認カードの表示と購入: 未計測(App 内課金を出すまで作らない。docs/metrics/product-metrics.md)。\n")
	return b.String()
}

func main() {
	in := flag.String("in", "-", "log export (JSON array or JSON lines); - for stdin")
	which := flag.String("report", "product", "product (the weekly product tables) or shared (shared.usage per participant, #2298)")
	flag.Parse()
	if *which != "product" && *which != "shared" {
		log.Fatalf("unknown -report %q (product or shared)", *which)
	}
	var r io.Reader = os.Stdin
	if *in != "-" {
		fh, err := os.Open(*in)
		if err != nil {
			log.Fatal(err)
		}
		defer func() { _ = fh.Close() }()
		r = fh
	}
	if *which == "shared" {
		us, err := parseShared(r)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := os.Stdout.WriteString(renderShared(aggregateShared(us))); err != nil {
			log.Fatal(err)
		}
		return
	}
	evs, err := parse(r)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.WriteString(render(aggregate(evs))); err != nil {
		log.Fatal(err)
	}
}
