// ai-eval runs internal/ai/eval/cases.json against the real Claude API through
// the same Service the ai server uses (in-memory store, no HTTP), and writes
// per-case metrics + the resulting DSL as JSON. Edit cases are seeded with the
// DSL produced by their `base` create case in the same run. `expect` keys:
// photo / chart (bool), notification ("HH:MM" or "any"), substitution (bool:
// the summary mentions 代替 / "replaced" because a requested part is not in
// the catalog), scope ("public"|"shared"|"private": at least one entity has
// that scope). A case may pin `locale` (ja|en); -locale overrides every case;
// otherwise the locale is detected from the transcript. English edits whose
// base was not produced in the run are seeded with examples/expense_en.json.
//
//	set -a; source ../.env; set +a
//	go run ./cmd/ai-eval -effort low -out ../docs/backend/ai-eval/raw
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

type Case struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"` // create|edit
	Group      string         `json:"group"`
	Base       string         `json:"base,omitempty"`
	Locale     string         `json:"locale,omitempty"` // ja|en; empty = detect
	Transcript string         `json:"transcript"`
	Note       string         `json:"note,omitempty"`
	Expect     map[string]any `json:"expect,omitempty"`
}

type Metrics struct {
	Tabs          int      `json:"tabs"`
	Screens       int      `json:"screens"`
	Components    int      `json:"components"`    // all components incl. container children
	TopLevel      int      `json:"topLevel"`      // screen-direct components
	WithArea      int      `json:"withArea"`      // screen-direct components carrying area
	AreaRatio     float64  `json:"areaRatio"`     // WithArea / TopLevel
	FullScreenCnt int      `json:"fullScreenCnt"` // areas equal to A1:F12
	Entities      int      `json:"entities"`
	Fields        int      `json:"fields"`
	HasPhoto      bool     `json:"hasPhoto"`
	HasChart      bool     `json:"hasChart"`
	Notifications []string `json:"notifications,omitempty"` // repeat@time
	Scopes        []string `json:"scopes,omitempty"`        // entityId:scope (effective)
	Areas         []string `json:"areas"`                   // screenId: area list
}

type Record struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Group         string          `json:"group"`
	Transcript    string          `json:"transcript"`
	Locale        string          `json:"locale,omitempty"`
	OK            bool            `json:"ok"`
	Error         string          `json:"error,omitempty"`
	Mode          string          `json:"mode,omitempty"`
	Attempts      int             `json:"attempts"`
	Regenerations int             `json:"regenerations"`
	AreaFallback  bool            `json:"areaFallback"`
	Downgraded    bool            `json:"downgradedToFull,omitempty"`
	ClaudeMs      int64           `json:"claudeMs"`
	TotalMs       int64           `json:"totalMs"`
	Usage         ai.Usage        `json:"usage"`
	Summary       string          `json:"summary,omitempty"`
	LastProblems  []string        `json:"lastProblems,omitempty"`
	Metrics       *Metrics        `json:"metrics,omitempty"`
	ExpectMisses  []string        `json:"expectMisses,omitempty"`
	Patch         []dsl.PatchOp   `json:"patch,omitempty"`
	DSL           json.RawMessage `json:"dsl,omitempty"`
}

type Run struct {
	Date    string   `json:"date"`
	Model   string   `json:"model"`
	Effort  string   `json:"effort"`
	Tag     string   `json:"tag,omitempty"`
	Cases   []Record `json:"cases"`
	Summary Summary  `json:"summary"`
}

type Summary struct {
	N             int     `json:"n"`
	OK            int     `json:"ok"`
	FirstPass     int     `json:"firstPass"` // ok && regenerations == 0 && !areaFallback
	Regenerated   int     `json:"regenerated"`
	AreaFallback  int     `json:"areaFallback"`
	Failed        int     `json:"failed"`
	ExpectMisses  int     `json:"expectMisses"`
	CreateP50Ms   int64   `json:"createP50Ms"`
	CreateP95Ms   int64   `json:"createP95Ms"`
	EditP50Ms     int64   `json:"editP50Ms"`
	AvgOutTokens  float64 `json:"avgOutputTokens"`
	AvgCreateOut  float64 `json:"avgCreateOutputTokens"`
	AvgEditOut    float64 `json:"avgEditOutputTokens"`
	AvgAreaRatio  float64 `json:"avgAreaRatio"`
	TabbedCreates int     `json:"tabbedCreates"`
}

func main() {
	model := flag.String("model", "claude-sonnet-5-5", "Claude model id")
	effort := flag.String("effort", "low", "output_config.effort")
	casesPath := flag.String("cases", "internal/ai/eval/cases.json", "cases file")
	outDir := flag.String("out", "../docs/backend/ai-eval/raw", "raw output directory")
	only := flag.String("only", "", "comma-separated case ids to run (edits still need their base: pass it too or it falls back to examples/kakeibo.json)")
	tag := flag.String("tag", "", "suffix for the output filename")
	locale := flag.String("locale", "", "ja|en for every case (overrides per-case locale; empty = per-case or detected from the transcript)")
	timeout := flag.Duration("timeout", 60*time.Second, "per Claude call timeout")
	flag.Parse()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set")
	}
	raw, err := os.ReadFile(*casesPath)
	if err != nil {
		log.Fatal(err)
	}
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		log.Fatal(err)
	}
	filter := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			filter[id] = true
		}
	}

	store := appconfigclient.NewMemory()
	svc := &ai.Service{
		Claude:      ai.NewClient(apiKey, *model, *effort),
		Store:       store,
		Model:       *model,
		CallTimeout: *timeout,
	}
	ctx := context.Background()
	run := Run{Date: time.Now().Format("2006-01-02"), Model: *model, Effort: *effort, Tag: *tag}
	created := map[string]json.RawMessage{} // create case id -> resulting DSL

	for _, c := range cases {
		if len(filter) > 0 && !filter[c.ID] {
			continue
		}
		appID := fmt.Sprintf("eval-%s-%d", c.ID, time.Now().UnixNano())
		mode := ai.ModeCreate
		loc := ai.Locale(c.Locale)
		if *locale != "" {
			loc = ai.Locale(*locale)
		}
		if loc == "" {
			loc = ai.DetectLocale(c.Transcript)
		}
		if c.Kind == "edit" {
			mode = ai.ModeEdit
			base, ok := created[c.Base]
			if !ok {
				seed := "internal/ai/examples/kakeibo.json"
				if loc == ai.LocaleEN {
					seed = "internal/ai/examples/expense_en.json"
				}
				base, err = os.ReadFile(seed)
				if err != nil {
					log.Fatal(err)
				}
				log.Printf("[%s] base %q not produced in this run; seeding %s", c.ID, c.Base, seed)
			}
			if _, err := store.Put(ctx, appID, 0, base, appconfigclient.Source{Kind: "seed"}); err != nil {
				log.Fatalf("[%s] seed: %v", c.ID, err)
			}
		}
		log.Printf("[%s] %s effort=%s locale=%s %q", c.ID, c.Kind, *effort, loc, c.Transcript)
		rec := Record{ID: c.ID, Kind: c.Kind, Group: c.Group, Transcript: c.Transcript, Locale: string(loc)}
		started := time.Now()
		res, err := svc.Generate(ctx, ai.Request{AppID: appID, Transcript: c.Transcript, Mode: mode, Locale: loc})
		rec.TotalMs = time.Since(started).Milliseconds()
		if err != nil {
			rec.Error = err.Error()
			var ge *ai.Error
			if e, ok := err.(*ai.Error); ok {
				ge = e
				rec.Attempts = ge.Attempts
				rec.LastProblems = ge.Problems
			}
			log.Printf("[%s] FAIL %v", c.ID, err)
		} else {
			rec.OK = true
			rec.Mode = res.Mode
			rec.Attempts = res.Meta.Attempts
			rec.Regenerations = res.Meta.Regenerations
			rec.AreaFallback = res.Meta.AreaFallback
			rec.Downgraded = res.Meta.DowngradedToFull
			rec.ClaudeMs = res.LatencyMs.Claude
			rec.Usage = res.Meta.Usage
			rec.Summary = res.Summary
			rec.LastProblems = res.Meta.LastProblems
			rec.Patch = res.Patch
			rec.DSL = res.DSL
			if app, perr := dsl.Parse(res.DSL); perr == nil {
				rec.Metrics = measure(app)
				rec.ExpectMisses = checkExpect(c.Expect, rec.Metrics, rec.Summary)
			}
			if c.Kind == "create" {
				created[c.ID] = res.DSL
			}
			log.Printf("[%s] ok attempts=%d fallback=%v claude=%dms out=%d tabs=%d screens=%d area=%.2f misses=%v summary=%q",
				c.ID, rec.Attempts, rec.AreaFallback, rec.ClaudeMs, rec.Usage.OutputTokens,
				rec.Metrics.Tabs, rec.Metrics.Screens, rec.Metrics.AreaRatio, rec.ExpectMisses, rec.Summary)
		}
		run.Cases = append(run.Cases, rec)
	}
	run.Summary = summarize(run.Cases)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	name := fmt.Sprintf("%s-%s-%s", run.Date, *model, *effort)
	if *tag != "" {
		name += "-" + *tag
	}
	out, _ := json.MarshalIndent(run, "", "  ")
	path := filepath.Join(*outDir, name+".json")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatal(err)
	}
	printTable(run)
	fmt.Printf("\nwrote %s\n", path)
}

func measure(a *dsl.App) *Metrics {
	m := &Metrics{Tabs: len(a.App.Tabs), Screens: len(a.Screens), Entities: len(a.Entities)}
	for i := range a.Entities {
		e := &a.Entities[i]
		m.Fields += len(e.Fields)
		m.Scopes = append(m.Scopes, e.ID+":"+string(e.EffectiveScope()))
	}
	for _, n := range a.App.Notifications {
		m.Notifications = append(m.Notifications, n.Repeat+"@"+n.Time)
	}
	for _, s := range a.Screens {
		var areas []string
		for _, c := range s.Components {
			m.TopLevel++
			m.Components++
			if c.Area != "" {
				m.WithArea++
				areas = append(areas, c.Area)
				if c.Area == "A1:F12" {
					m.FullScreenCnt++
				}
			} else {
				areas = append(areas, "-")
			}
			m.Components += len(c.Children)
			scan := append([]dsl.Component{c}, c.Children...)
			for _, x := range scan {
				switch x.Type {
				case dsl.CompPhotoField, dsl.CompImage:
					m.HasPhoto = true
				case dsl.CompChart:
					m.HasChart = true
				}
				if x.Action != nil && x.Action.Type == dsl.ActionScheduleNotif && x.Action.Notification != nil {
					m.Notifications = append(m.Notifications, "action:"+x.Action.Notification.Repeat+"@"+x.Action.Notification.Time)
				}
			}
		}
		m.Areas = append(m.Areas, s.ID+": "+strings.Join(areas, " "))
	}
	if m.TopLevel > 0 {
		m.AreaRatio = float64(m.WithArea) / float64(m.TopLevel)
	}
	return m
}

func checkExpect(exp map[string]any, m *Metrics, summary string) []string {
	var miss []string
	for k, v := range exp {
		switch k {
		case "substitution": // a part missing from the catalog was requested: summary must say so
			if mentionsSubstitution(summary) != v.(bool) {
				miss = append(miss, fmt.Sprintf("substitution=%v", !v.(bool)))
			}
		case "photo":
			if m.HasPhoto != v.(bool) {
				miss = append(miss, fmt.Sprintf("photo=%v", m.HasPhoto))
			}
		case "chart":
			if m.HasChart != v.(bool) {
				miss = append(miss, fmt.Sprintf("chart=%v", m.HasChart))
			}
		case "notification":
			want := fmt.Sprint(v)
			found := false
			for _, n := range m.Notifications {
				if want == "any" || strings.HasSuffix(n, "@"+want) {
					found = true
				}
			}
			if !found {
				miss = append(miss, fmt.Sprintf("notification=%v (want %s)", m.Notifications, want))
			}
		case "scope":
			want := fmt.Sprint(v)
			found := false
			for _, s := range m.Scopes {
				if strings.HasSuffix(s, ":"+want) {
					found = true
				}
			}
			if !found {
				miss = append(miss, fmt.Sprintf("scope=%v (want %s)", m.Scopes, want))
			}
		}
	}
	sort.Strings(miss)
	return miss
}

// mentionsSubstitution: the ja template says 代替, the en one says "replaced".
func mentionsSubstitution(summary string) bool {
	return strings.Contains(summary, "代替") || strings.Contains(strings.ToLower(summary), "replaced")
}

func percentile(v []int64, p float64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := int(float64(len(s)-1)*p + 0.5)
	return s[i]
}

func summarize(recs []Record) Summary {
	var s Summary
	var createMs, editMs []int64
	var outAll, outCreate, outEdit, nCreate, nEdit int
	var areaSum float64
	var areaN int
	for _, r := range recs {
		s.N++
		if !r.OK {
			s.Failed++
			continue
		}
		s.OK++
		if r.Regenerations == 0 && !r.AreaFallback {
			s.FirstPass++
		}
		if r.Regenerations > 0 {
			s.Regenerated++
		}
		if r.AreaFallback {
			s.AreaFallback++
		}
		if len(r.ExpectMisses) > 0 {
			s.ExpectMisses++
		}
		outAll += r.Usage.OutputTokens
		if r.Kind == "create" {
			createMs = append(createMs, r.ClaudeMs)
			outCreate += r.Usage.OutputTokens
			nCreate++
			if r.Metrics != nil && r.Metrics.Tabs >= 2 {
				s.TabbedCreates++
			}
		} else {
			editMs = append(editMs, r.ClaudeMs)
			outEdit += r.Usage.OutputTokens
			nEdit++
		}
		if r.Metrics != nil {
			areaSum += r.Metrics.AreaRatio
			areaN++
		}
	}
	s.CreateP50Ms = percentile(createMs, 0.5)
	s.CreateP95Ms = percentile(createMs, 0.95)
	s.EditP50Ms = percentile(editMs, 0.5)
	if s.OK > 0 {
		s.AvgOutTokens = float64(outAll) / float64(s.OK)
	}
	if nCreate > 0 {
		s.AvgCreateOut = float64(outCreate) / float64(nCreate)
	}
	if nEdit > 0 {
		s.AvgEditOut = float64(outEdit) / float64(nEdit)
	}
	if areaN > 0 {
		s.AvgAreaRatio = areaSum / float64(areaN)
	}
	return s
}

func printTable(run Run) {
	fmt.Printf("\n## %s %s effort=%s\n\n", run.Date, run.Model, run.Effort)
	fmt.Println("| id | kind | loc | ok | att | fb | claude ms | in/out tok | tabs | scr | comp | area% | notif | misses | summary |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range run.Cases {
		ok := "OK"
		if !r.OK {
			ok = "FAIL"
		}
		tabs, scr, comp, area, notif := "-", "-", "-", "-", "-"
		if r.Metrics != nil {
			tabs = fmt.Sprint(r.Metrics.Tabs)
			scr = fmt.Sprint(r.Metrics.Screens)
			comp = fmt.Sprint(r.Metrics.Components)
			area = fmt.Sprintf("%.0f%%", r.Metrics.AreaRatio*100)
			if len(r.Metrics.Notifications) > 0 {
				notif = strings.Join(r.Metrics.Notifications, ",")
			}
		}
		fb := "-"
		if r.AreaFallback {
			fb = "area"
		}
		if r.Downgraded {
			fb += "+full"
		}
		misses := "-"
		if len(r.ExpectMisses) > 0 {
			misses = strings.Join(r.ExpectMisses, "; ")
		}
		sum := r.Summary
		if !r.OK {
			sum = r.Error
		}
		fmt.Printf("| %s | %s | %s | %s | %d | %s | %d | %d/%d | %s | %s | %s | %s | %s | %s | %s |\n",
			r.ID, r.Kind, r.Locale, ok, r.Attempts, fb, r.ClaudeMs, r.Usage.InputTokens+r.Usage.CacheReadInputTokens+r.Usage.CacheCreationInputTokens,
			r.Usage.OutputTokens, tabs, scr, comp, area, notif, misses, strings.ReplaceAll(sum, "|", "/"))
	}
	s := run.Summary
	fmt.Printf("\nn=%d ok=%d firstPass=%d regenerated=%d areaFallback=%d failed=%d expectMisses=%d\n", s.N, s.OK, s.FirstPass, s.Regenerated, s.AreaFallback, s.Failed, s.ExpectMisses)
	fmt.Printf("create p50=%dms p95=%dms | edit p50=%dms | avg out tokens all=%.0f create=%.0f edit=%.0f | avg area ratio=%.2f | tabbed creates=%d\n",
		s.CreateP50Ms, s.CreateP95Ms, s.EditP50Ms, s.AvgOutTokens, s.AvgCreateOut, s.AvgEditOut, s.AvgAreaRatio, s.TabbedCreates)
}
