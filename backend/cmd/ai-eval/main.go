// ai-eval runs internal/ai/eval/cases.json against the real Claude API through
// the same Service the ai server uses (in-memory store, no HTTP), and writes
// per-case metrics + the resulting DSL as JSON. Edit cases are seeded with the
// DSL produced by their `base` case (create or an earlier edit) in the same run. `expect` keys:
// photo / chart / calendar (bool), notification ("HH:MM" or "any"), substitution (bool:
// the summary mentions 代替 / できません / "replaced" because a requested part is not in
// the catalog or not possible, #257), scope ("public"|"shared"|"private": at least one entity has
// that scope), editMembers (bool, #252: some shared entity has edit members),
// authorShown (bool, #250: a list/table/timeline shows $author), nameInput (bool, #250:
// a shared entity has a typed-in person-name field), meFilter (bool, #251: a component filters by $author),
// noShared (bool, #77: no shared/public entity), calc / computed / statValue / settlement / ref (bool, #100:
// any calculation / Field.computed / stat.value / settlement leaf / ref field;
// -logic-pass runs the 2nd calculations-only call). Navigation (#114):
// patch (bool: stored as a JSON Patch, not a full rewrite), unreachable
// (number of screens no tab / initial screen reaches, dsl.Unreachable),
// nav (["from>to", ...]: a button / toolbar / item tap on `from` opens `to`),
// tabs / tabsMin (number), tabOrder ("a,b": the first tab screens in order),
// tabHas (component type on a tab root other than the first tab), has ([type, ...]:
// each component type appears on some screen), lacks ([type, ...]: none of them appears, #212).
// Health (#167): health ("steps"|"weight": a healthStat with that metric),
// healthField (bool: some field has health "weight").
// Warikan (#231): refs (bool: some field is type refs), deletable ([entity id, ...]: swipeToDelete list or deleteRecord).
// Grid (#234): grid (bool: some gridTable is present).
// Omission (#257): noOmission (bool: the model met or honestly declined every
// requested part itself; Meta.Omissions = parts the server had to note).
// Field cap (#221): keepEntity / keep ([field id, ...]) / growTo (see checkKeep).
// Parent -> child (#256): prefillRef (bool: some navigate opens a child form with prefillRef).
// `base` may be "example:<name>" to seed internal/ai/examples/<name>.json. A case may pin `locale` (ja|en); -locale overrides every case;
// otherwise the locale is detected from the transcript. English edits whose
// base was not produced in the run are seeded with examples/expense_en.json.
// M0 (#281): shared (bool: some entity is shared or public, whichever the model
// picks). Keys and types are listed in expect.go; -check validates a cases
// file without the API, and every run ends with the pass rate per group.
//
//	go run ./cmd/ai-eval -check -cases internal/ai/eval/m0disc_cases.json  # no API calls
//	set -a; source ../.env; set +a
//	go run ./cmd/ai-eval -effort low -out ../docs/backend/ai-eval/raw
//	go run ./cmd/ai-eval -stream -repeat 2 -only kakeibo_a,walk_a   # SSE path, time to first event
//	go run ./cmd/ai-eval -compare-before 'raw/*-before-*.json' -compare-after 'raw/*-after-*.json'  # no API calls
//	go run ./cmd/ai-eval -chat          # clarify phase (chat.go, chat_cases.json)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
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
	Needs      []string       `json:"needs,omitempty"` // audit_cases: human checklist only, not checked
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
	EditMembers   []string `json:"editMembers,omitempty"`   // shared entity ids with edit:members (#252)
	AuthorShown   []string `json:"authorShown,omitempty"`   // screen:type showing $author (#250)
	NameInputs    []string `json:"nameInputs,omitempty"`    // shared entity.field that looks like a typed name (#250)
	MeFilter      []string `json:"meFilter,omitempty"`      // screen:type filtering by $author (#251)
	Areas         []string `json:"areas"`                   // screenId: area list
	// logic (#100)
	Computed   []string `json:"computed,omitempty"` // entity.field of computed fields
	StatValues int      `json:"statValues,omitempty"`
	Settlement bool     `json:"settlement,omitempty"`
	RefFields  int      `json:"refFields,omitempty"`
	ProgressTo bool     `json:"progressTarget,omitempty"` // progressBar entity+target (aggregate-based rate)
	// #177
	HasCalendar bool `json:"hasCalendar,omitempty"`
	// #234: gridTable columnField:rowField
	Grid        []string `json:"grid,omitempty"`
	StatUnitDup []string `json:"statUnitDup,omitempty"` // non-count stat labels ending in "(field.unit)"
	// #167
	Health      []string `json:"health,omitempty"`      // healthStat metric:period
	HealthField bool     `json:"healthField,omitempty"` // a field with health "weight"
	// #256: screen>childForm.refField of navigate.prefillRef
	PrefillRefs []string `json:"prefillRefs,omitempty"`
}

// HasCalc reports whether the app computes anything beyond plain aggregate stats.
func (m *Metrics) HasCalc() bool {
	return len(m.Computed) > 0 || m.StatValues > 0 || m.Settlement || m.ProgressTo
}

type Record struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Group         string          `json:"group"`
	Transcript    string          `json:"transcript"`
	Locale        string          `json:"locale,omitempty"`
	Repeat        int             `json:"repeat,omitempty"`
	OK            bool            `json:"ok"`
	Error         string          `json:"error,omitempty"`
	Mode          string          `json:"mode,omitempty"`
	Attempts      int             `json:"attempts"`
	Regenerations int             `json:"regenerations"`
	AreaFallback  bool            `json:"areaFallback"`
	Downgraded    bool            `json:"downgradedToFull,omitempty"`
	ClaudeMs      int64           `json:"claudeMs"`
	TotalMs       int64           `json:"totalMs"`
	FirstEventMs  int64           `json:"firstEventMs,omitempty"` // -stream: time to the app/summary event
	FirstEvent    string          `json:"firstEvent,omitempty"`   // -stream: its provisional summary
	OutputChars   int             `json:"outputChars,omitempty"`
	Usage         ai.Usage        `json:"usage"`
	Summary       string          `json:"summary,omitempty"`
	LastProblems  []string        `json:"lastProblems,omitempty"`
	LintRetried   bool            `json:"lintRetried,omitempty"`
	Omissions     []string        `json:"omissions,omitempty"` // #257: the server appended a note for these
	Metrics       *Metrics        `json:"metrics,omitempty"`
	ExpectMisses  []string        `json:"expectMisses,omitempty"`
	LogicPass     *ai.LogicMeta   `json:"logicPass,omitempty"`
	Patch         []dsl.PatchOp   `json:"patch,omitempty"`
	DSL           json.RawMessage `json:"dsl,omitempty"`
}

type Run struct {
	Date    string   `json:"date"`
	Model   string   `json:"model"`
	Effort  string   `json:"effort"`
	Tag     string   `json:"tag,omitempty"`
	Stream  bool     `json:"stream,omitempty"`
	Cases   []Record `json:"cases"`
	Summary Summary  `json:"summary"`
}

type Summary struct {
	N                 int     `json:"n"`
	OK                int     `json:"ok"`
	FirstPass         int     `json:"firstPass"` // ok && regenerations == 0 && !areaFallback
	Regenerated       int     `json:"regenerated"`
	AreaFallback      int     `json:"areaFallback"`
	Failed            int     `json:"failed"`
	ExpectMisses      int     `json:"expectMisses"`
	CreateP50Ms       int64   `json:"createP50Ms"`
	CreateP95Ms       int64   `json:"createP95Ms"`
	EditP50Ms         int64   `json:"editP50Ms"`
	CreateFirstP50Ms  int64   `json:"createFirstEventP50Ms,omitempty"`
	AvgOutTokens      float64 `json:"avgOutputTokens"`
	AvgCreateOut      float64 `json:"avgCreateOutputTokens"`
	AvgEditOut        float64 `json:"avgEditOutputTokens"`
	AvgCreateOutChars float64 `json:"avgCreateOutputChars,omitempty"`
	AvgAreaRatio      float64 `json:"avgAreaRatio"`
	TabbedCreates     int     `json:"tabbedCreates"`
	CalcCreates       int     `json:"calcCreates"` // creates whose app computes something (Metrics.HasCalc)
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
	stream := flag.Bool("stream", false, "use GenerateStream (SSE path) and record the time to the first app/summary event")
	repeat := flag.Int("repeat", 1, "run every selected case this many times (p50/p95 over all runs)")
	cmpBefore := flag.String("compare-before", "", "glob of raw run files = before; with -compare-after prints a before/after table and exits (no API calls)")
	cmpAfter := flag.String("compare-after", "", "glob of raw run files = after")
	envFile := flag.String("env-file", "", "KEY=VALUE file to load into the environment first (e.g. ../.env); values are never printed")
	logicPass := flag.Bool("logic-pass", false, "enable the 2nd calculations-only call after each create (same as AI_LOGIC_PASS=1)")
	chat := flag.Bool("chat", false, "run the clarify-phase cases (internal/ai/eval/chat_cases.json unless -cases is set) through Service.Chat instead of /generate")
	chatModel := flag.String("chat-model", "", "-chat: clarify model (empty = ai.DefaultChatModel, same as AI_CHAT_MODEL)")
	chatEffort := flag.String("chat-effort", "", "-chat: clarify effort (empty = ai.DefaultChatEffort)")
	chatThinking := flag.String("chat-thinking", "", "-chat: thinking.type adaptive|disabled|between_tools (empty = ai.DefaultChatThinking)")
	chatCaps := flag.String("capabilities", "", "-chat: comma-separated capabilities (answer,proposal) sent by cases without their own mode/capabilities (#185)")
	check := flag.Bool("check", false, "validate the cases file (ids, bases, expect keys and types) and exit (no API calls, #281)")
	flag.Parse()
	if *chat && *casesPath == "internal/ai/eval/cases.json" {
		*casesPath = "internal/ai/eval/chat_cases.json"
	}

	if *cmpBefore != "" || *cmpAfter != "" {
		if err := compare(*cmpBefore, *cmpAfter); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := checkCasesFile(*casesPath, *chat); err != nil {
		log.Fatal(err)
	}
	if *check {
		fmt.Printf("ok %s\n", *casesPath)
		return
	}

	if *envFile != "" {
		if err := loadEnvFile(*envFile); err != nil {
			log.Fatalf("env-file: %v", err)
		}
	}
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set")
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
		LogicPass:   *logicPass,
	}
	ctx := context.Background()
	if *chat {
		svc.ChatConfig = ai.ChatConfig{Model: *chatModel, Effort: *chatEffort, Thinking: *chatThinking}
		var caps []string
		for _, c := range strings.Split(*chatCaps, ",") {
			if c = strings.TrimSpace(c); c != "" {
				caps = append(caps, c)
			}
		}
		runChat(ctx, svc, *casesPath, *outDir, *tag, filter, caps)
		return
	}
	raw, err := os.ReadFile(*casesPath)
	if err != nil {
		log.Fatal(err)
	}
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		log.Fatal(err)
	}
	run := Run{Date: time.Now().Format("2006-01-02"), Model: *model, Effort: *effort, Tag: *tag, Stream: *stream}
	created := map[string]json.RawMessage{} // case id -> resulting DSL (create or edit)

	for rep := 1; rep <= max(*repeat, 1); rep++ {
		for _, c := range cases {
			if len(filter) > 0 && !filter[c.ID] {
				continue
			}
			rec := runCase(ctx, svc, store, c, *locale, *effort, *stream, created)
			if *repeat > 1 {
				rec.Repeat = rep
			}
			run.Cases = append(run.Cases, rec)
		}
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
	hasExpect := map[string]bool{}
	for _, c := range cases {
		hasExpect[c.ID] = len(c.Expect) > 0
	}
	printPassByGroup(passByGroup(run.Cases, hasExpect))
	fmt.Printf("\nwrote %s\n", path)
}

func runCase(ctx context.Context, svc *ai.Service, store *appconfigclient.Memory, c Case, forceLocale, effort string, stream bool, created map[string]json.RawMessage) Record {
	appID := fmt.Sprintf("eval-%s-%d", c.ID, time.Now().UnixNano())
	mode := ai.ModeCreate
	loc := ai.Locale(c.Locale)
	if forceLocale != "" {
		loc = ai.Locale(forceLocale)
	}
	if loc == "" {
		loc = ai.DetectLocale(c.Transcript)
	}
	if c.Kind == "edit" {
		mode = ai.ModeEdit
		base, ok := created[c.Base]
		if name, isExample := strings.CutPrefix(c.Base, "example:"); isExample {
			var err error
			if base, err = os.ReadFile(filepath.Join("internal/ai/examples", name+".json")); err != nil {
				log.Fatal(err)
			}
			ok = true
		}
		if !ok {
			seed := "internal/ai/examples/kakeibo.json"
			if loc == ai.LocaleEN {
				seed = "internal/ai/examples/expense_en.json"
			}
			var err error
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
	log.Printf("[%s] %s effort=%s locale=%s stream=%v %q", c.ID, c.Kind, effort, loc, stream, c.Transcript)
	rec := Record{ID: c.ID, Kind: c.Kind, Group: c.Group, Transcript: c.Transcript, Locale: string(loc)}
	started := time.Now()
	req := ai.Request{AppID: appID, Transcript: c.Transcript, Mode: mode, Locale: loc}
	var res *ai.Result
	var err error
	if stream {
		res, err = svc.GenerateStream(ctx, req, func(ev ai.Event) {
			if rec.FirstEventMs == 0 {
				rec.FirstEventMs = time.Since(started).Milliseconds()
				rec.FirstEvent = ev.Summary
				log.Printf("[%s] first event %s at %dms: %q", c.ID, ev.Type, rec.FirstEventMs, ev.Summary)
			}
		})
	} else {
		res, err = svc.Generate(ctx, req)
	}
	rec.TotalMs = time.Since(started).Milliseconds()
	if err != nil {
		rec.Error = err.Error()
		if ge, ok := err.(*ai.Error); ok {
			rec.Attempts = ge.Attempts
			rec.LastProblems = ge.Problems
		}
		log.Printf("[%s] FAIL %v", c.ID, err)
		return rec
	}
	rec.OK = true
	rec.Mode = res.Mode
	rec.Attempts = res.Meta.Attempts
	rec.Regenerations = res.Meta.Regenerations
	rec.AreaFallback = res.Meta.AreaFallback
	rec.Downgraded = res.Meta.DowngradedToFull
	rec.ClaudeMs = res.LatencyMs.Claude
	rec.OutputChars = res.Meta.OutputChars
	rec.Usage = res.Meta.Usage
	rec.Summary = res.Summary
	rec.LastProblems = res.Meta.LastProblems
	rec.LintRetried = res.Meta.LintRetried
	rec.Omissions = res.Meta.Omissions
	rec.LogicPass = res.Meta.LogicPass
	rec.Patch = res.Patch
	rec.DSL = res.DSL
	if app, perr := dsl.Parse(res.DSL); perr == nil {
		rec.Metrics = measure(app)
		rec.ExpectMisses = append(checkExpect(c.Expect, rec.Metrics, rec.Summary), checkNav(c.Expect, app, rec.Mode)...)
		rec.ExpectMisses = append(rec.ExpectMisses, checkEnum(c.Expect, app)...)
		rec.ExpectMisses = append(rec.ExpectMisses, checkKeep(c.Expect, app, rec.Summary)...)
		rec.ExpectMisses = append(rec.ExpectMisses, checkMarks(c.Expect, app)...) // #255: marks.go
		if v, ok := c.Expect["noOmission"].(bool); ok && v && len(rec.Omissions) > 0 {
			rec.ExpectMisses = append(rec.ExpectMisses, fmt.Sprintf("omissions=%v", rec.Omissions))
		}
	}
	created[c.ID] = res.DSL // edits too, so an edit can build on an earlier edit (#229)
	log.Printf("[%s] ok attempts=%d fallback=%v claude=%dms first=%dms out=%d chars=%d tabs=%d screens=%d area=%.2f misses=%v summary=%q",
		c.ID, rec.Attempts, rec.AreaFallback, rec.ClaudeMs, rec.FirstEventMs, rec.Usage.OutputTokens, rec.OutputChars,
		rec.Metrics.Tabs, rec.Metrics.Screens, rec.Metrics.AreaRatio, rec.ExpectMisses, rec.Summary)
	return rec
}

func measure(a *dsl.App) *Metrics {
	m := &Metrics{Tabs: len(a.App.Tabs), Screens: len(a.Screens), Entities: len(a.Entities)}
	m.PrefillRefs = prefillRefs(a)
	for i := range a.Entities {
		e := &a.Entities[i]
		m.Fields += len(e.Fields)
		m.Scopes = append(m.Scopes, e.ID+":"+string(e.EffectiveScope()))
		if e.MembersCanEdit() {
			m.EditMembers = append(m.EditMembers, e.ID)
		}
		for _, f := range e.Fields {
			if f.Computed != nil {
				m.Computed = append(m.Computed, e.ID+"."+f.ID)
			}
			if f.Type == dsl.FieldRef || f.Type == dsl.FieldRefs {
				m.RefFields++
			}
			if f.Health != "" {
				m.HealthField = true
			}
		}
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
				case dsl.CompSettlement:
					m.Settlement = true
				case dsl.CompCalendar:
					m.HasCalendar = true
				case dsl.CompGridTable:
					m.Grid = append(m.Grid, x.ColumnField+":"+x.RowField)
				case dsl.CompHealthStat:
					m.Health = append(m.Health, x.Metric+":"+x.Period)
				case dsl.CompStat:
					if x.Value != nil {
						m.StatValues++
					}
					if u := fieldUnit(a, x.Entity, x.Field); u != "" && x.Aggregate != "" && x.Aggregate != "count" &&
						(strings.HasSuffix(x.Label, "("+u+")") || strings.HasSuffix(x.Label, "（"+u+"）")) {
						m.StatUnitDup = append(m.StatUnitDup, x.Aggregate+":"+x.Label)
					}
				case dsl.CompProgressBar:
					if x.Entity != "" && x.Target != nil {
						m.ProgressTo = true
					}
				}
				if x.Action != nil && x.Action.Type == dsl.ActionScheduleNotif && x.Action.Notification != nil {
					m.Notifications = append(m.Notifications, "action:"+x.Action.Notification.Repeat+"@"+x.Action.Notification.Time)
				}
			}
		}
		m.Areas = append(m.Areas, s.ID+": "+strings.Join(areas, " "))
	}
	m.AuthorShown, m.MeFilter = authorUses(a)
	m.NameInputs = nameInputs(a)
	if m.TopLevel > 0 {
		m.AreaRatio = float64(m.WithArea) / float64(m.TopLevel)
	}
	return m
}

func fieldUnit(a *dsl.App, entity, field string) string {
	if e := a.EntityByID(entity); e != nil {
		if f := e.FieldByID(field); f != nil {
			return f.Unit
		}
	}
	return ""
}

func checkExpect(exp map[string]any, m *Metrics, summary string) []string {
	var miss []string
	for k, v := range exp {
		switch k {
		case "shared": // #281: shared with others, public or shared alike
			found := slices.ContainsFunc(m.Scopes, func(s string) bool {
				return strings.HasSuffix(s, ":public") || strings.HasSuffix(s, ":shared")
			})
			if found != v.(bool) {
				miss = append(miss, fmt.Sprintf("shared=%v", m.Scopes))
			}
		case "substitution": // a part missing from the catalog was requested: summary must say so
			if mentionsSubstitution(summary) != v.(bool) {
				miss = append(miss, fmt.Sprintf("substitution=%v", !v.(bool)))
			}
		case "health":
			want, _ := v.(string)
			if !slices.ContainsFunc(m.Health, func(h string) bool { return strings.HasPrefix(h, want+":") }) {
				miss = append(miss, fmt.Sprintf("health=%v", m.Health))
			}
		case "healthField":
			if m.HealthField != v.(bool) {
				miss = append(miss, fmt.Sprintf("healthField=%v", m.HealthField))
			}
		case "grid":
			if (len(m.Grid) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("grid=%v", m.Grid))
			}
		case "prefillRef": // #256
			if (len(m.PrefillRefs) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("prefillRef=%v", m.PrefillRefs))
			}
		case "calendar":
			if m.HasCalendar != v.(bool) {
				miss = append(miss, fmt.Sprintf("calendar=%v", m.HasCalendar))
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
		case "calc": // computed / stat.value / settlement / progressBar target: any calculation at all
			if m.HasCalc() != v.(bool) {
				miss = append(miss, fmt.Sprintf("calc=%v", m.HasCalc()))
			}
		case "computed":
			if (len(m.Computed) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("computed=%v", m.Computed))
			}
		case "statValue":
			if (m.StatValues > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("statValue=%d", m.StatValues))
			}
		case "settlement":
			if m.Settlement != v.(bool) {
				miss = append(miss, fmt.Sprintf("settlement=%v", m.Settlement))
			}
		case "ref":
			if (m.RefFields > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("ref=%d", m.RefFields))
			}
		case "noShared": // #77: 「この機能なしで作る」は shared / public の entity を作らない
			for _, s := range m.Scopes {
				if v.(bool) && (strings.HasSuffix(s, ":public") || strings.HasSuffix(s, ":shared")) {
					miss = append(miss, fmt.Sprintf("noShared: scope=%v", m.Scopes))
					break
				}
			}
		case "editMembers": // #252: true = some shared entity is edit:members, false = none
			if (len(m.EditMembers) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("editMembers=%v", m.EditMembers))
			}
		case "authorShown": // #250
			if (len(m.AuthorShown) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("authorShown=%v", m.AuthorShown))
			}
		case "nameInput": // #250: false = no typed-in name field on shared entities
			if (len(m.NameInputs) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("nameInput=%v", m.NameInputs))
			}
		case "meFilter": // #251
			if (len(m.MeFilter) > 0) != v.(bool) {
				miss = append(miss, fmt.Sprintf("meFilter=%v", m.MeFilter))
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

// checkNav checks the navigation expect keys (#114) against the result.
// checkEnum (#194): `enumField` "entity.field" is the choice field; optHas /
// optNot check its options (enum choices or ref initial records), fields =
// the entity's field count (no memo / extra field added), refPick = it became
// a ref to a master entity with a form screen and initial options.
// checkKeep (#221): keepEntity / keep list the fields that must survive the
// edit; growTo is the field count when every requested field was added. The
// edit passes when nothing in keep was removed and either the entity reached
// growTo or the summary says the cap blocked it (上限 / "limit").
func checkKeep(exp map[string]any, a *dsl.App, summary string) []string {
	eid, _ := exp["keepEntity"].(string)
	if eid == "" {
		return nil
	}
	e := a.EntityByID(eid)
	if e == nil {
		return []string{"keepEntity missing " + eid}
	}
	var miss []string
	if keep, ok := exp["keep"].([]any); ok {
		for _, k := range keep {
			if e.FieldByID(fmt.Sprint(k)) == nil {
				miss = append(miss, "removed "+fmt.Sprint(k))
			}
		}
	}
	if g, ok := exp["growTo"].(float64); ok && len(e.Fields) < int(g) &&
		!strings.Contains(summary, "上限") && !strings.Contains(strings.ToLower(summary), "limit") {
		miss = append(miss, fmt.Sprintf("fields=%d < %d and summary does not mention the cap", len(e.Fields), int(g)))
	}
	return miss
}

func checkEnum(exp map[string]any, a *dsl.App) []string {
	target, _ := exp["enumField"].(string)
	if target == "" {
		return nil
	}
	eid, fid, _ := strings.Cut(target, ".")
	e := a.EntityByID(eid)
	if e == nil || e.FieldByID(fid) == nil {
		return []string{"enumField missing " + target}
	}
	f := e.FieldByID(fid)
	var miss []string
	strs := func(v any) (out []string) {
		for _, s := range v.([]any) {
			out = append(out, fmt.Sprint(s))
		}
		return out
	}
	for k, v := range exp {
		switch k {
		case "optHas", "optNot":
			for _, w := range strs(v) {
				if slices.Contains(f.Options, w) != (k == "optHas") {
					miss = append(miss, fmt.Sprintf("%s %s (options=%v)", k, w, f.Options))
				}
			}
		case "fields":
			if n := int(v.(float64)); len(e.Fields) != n {
				miss = append(miss, fmt.Sprintf("fields=%d (want %d)", len(e.Fields), n))
			}
		case "refPick":
			form := false
			for _, s := range a.Screens {
				form = form || (f.RefEntity != "" && s.Entity == f.RefEntity)
			}
			if ok := f.Type == dsl.FieldRef && a.EntityByID(f.RefEntity) != nil && form && len(f.Options) > 0; ok != v.(bool) {
				miss = append(miss, fmt.Sprintf("refPick=%v (type=%s ref=%s form=%v options=%d)", ok, f.Type, f.RefEntity, form, len(f.Options)))
			}
		}
	}
	return miss
}

func checkNav(exp map[string]any, a *dsl.App, mode string) []string {
	var miss []string
	num := func(v any) int { f, _ := v.(float64); return int(f) }
	for k, v := range exp {
		switch k {
		case "patch":
			if (mode == "patch") != v.(bool) {
				miss = append(miss, "patch="+mode)
			}
		case "unreachable":
			if u := a.Unreachable(); len(u) != num(v) {
				miss = append(miss, fmt.Sprintf("unreachable=%v", u))
			}
		case "nav":
			for _, e := range v.([]any) {
				from, to, _ := strings.Cut(fmt.Sprint(e), ">")
				if !a.HasNav(from, to) {
					miss = append(miss, "nav!"+from+">"+to)
				}
			}
		case "tabs", "tabsMin":
			n := len(a.App.Tabs)
			if (k == "tabs" && n != num(v)) || (k == "tabsMin" && n < num(v)) {
				miss = append(miss, fmt.Sprintf("tabs=%d", n))
			}
		case "tabOrder":
			want := strings.Split(fmt.Sprint(v), ",")
			var got []string
			for _, t := range a.App.Tabs {
				got = append(got, t.Screen)
			}
			if len(got) < len(want) || strings.Join(got[:len(want)], ",") != strings.Join(want, ",") {
				miss = append(miss, "tabOrder="+strings.Join(got, ","))
			}
		case "tabHas":
			found := false
			for i, t := range a.App.Tabs {
				if i == 0 {
					continue
				}
				for _, s := range a.Screens {
					if s.ID != t.Screen {
						continue
					}
					for _, c := range s.Components {
						for _, x := range append([]dsl.Component{c}, c.Children...) {
							found = found || string(x.Type) == fmt.Sprint(v)
						}
					}
				}
			}
			if !found {
				miss = append(miss, fmt.Sprintf("tabHas!%v", v))
			}
		case "refs": // #231: some field is type refs (multi-select, e.g. expense.split_with)
			found := false
			for _, e := range a.Entities {
				for _, f := range e.Fields {
					found = found || f.Type == dsl.FieldRefs
				}
			}
			if found != v.(bool) {
				miss = append(miss, fmt.Sprintf("refs=%v", found))
			}
		case "deletable": // #231: each entity has a list with swipeToDelete or a deleteRecord button / toolbar
			for _, w := range v.([]any) {
				if !deletable(a, fmt.Sprint(w)) {
					miss = append(miss, fmt.Sprintf("deletable!%v", w))
				}
			}
		case "has", "lacks": // component types that must (#168) / must not (#212) appear on some screen
			for _, w := range v.([]any) {
				found := false
				for _, s := range a.Screens {
					for _, c := range s.Components {
						for _, x := range append([]dsl.Component{c}, c.Children...) {
							found = found || string(x.Type) == fmt.Sprint(w)
						}
					}
				}
				if found != (k == "has") {
					miss = append(miss, fmt.Sprintf("%s!%v", k, w))
				}
			}
		}
	}
	sort.Strings(miss)
	return miss
}

// deletable: a list of entity swipes to delete, or a deleteRecord button / toolbar targets it (#231).
func deletable(a *dsl.App, entity string) bool {
	del := func(act *dsl.Action) bool {
		return act != nil && act.Type == dsl.ActionDeleteRecord && act.Entity == entity
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			if del(&s.Toolbar[i].Action) {
				return true
			}
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if del(x.Action) || (x.Type == dsl.CompList && x.Entity == entity && x.SwipeToDelete) {
					return true
				}
			}
		}
	}
	return false
}

// mentionsSubstitution: the ja template says 代替, the en one says "replaced".
func mentionsSubstitution(summary string) bool {
	if strings.Contains(summary, "代替") || strings.Contains(summary, "できません") || strings.Contains(summary, "できない") {
		return true
	}
	l := strings.ToLower(summary)
	return strings.Contains(l, "replaced") || strings.Contains(l, "isn't possible") || strings.Contains(l, "can't")
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
	var createMs, editMs, firstMs []int64
	var outAll, outCreate, outEdit, nCreate, nEdit, charsCreate int
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
			if r.FirstEventMs > 0 {
				firstMs = append(firstMs, r.FirstEventMs)
			}
			outCreate += r.Usage.OutputTokens
			charsCreate += r.OutputChars
			nCreate++
			if r.Metrics != nil && r.Metrics.Tabs >= 2 {
				s.TabbedCreates++
			}
			if r.Metrics != nil && r.Metrics.HasCalc() {
				s.CalcCreates++
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
	s.CreateFirstP50Ms = percentile(firstMs, 0.5)
	if s.OK > 0 {
		s.AvgOutTokens = float64(outAll) / float64(s.OK)
	}
	if nCreate > 0 {
		s.AvgCreateOut = float64(outCreate) / float64(nCreate)
		s.AvgCreateOutChars = float64(charsCreate) / float64(nCreate)
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
	fmt.Printf("\n## %s %s effort=%s stream=%v\n\n", run.Date, run.Model, run.Effort, run.Stream)
	fmt.Println("| id | kind | loc | ok | att | fb | claude ms | first ms | in/out tok | tabs | scr | comp | area% | notif | misses | summary |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
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
		first := "-"
		if r.FirstEventMs > 0 {
			first = fmt.Sprint(r.FirstEventMs)
		}
		fmt.Printf("| %s | %s | %s | %s | %d | %s | %d | %s | %d/%d | %s | %s | %s | %s | %s | %s | %s |\n",
			r.ID, r.Kind, r.Locale, ok, r.Attempts, fb, r.ClaudeMs, first, r.Usage.InputTokens+r.Usage.CacheReadInputTokens+r.Usage.CacheCreationInputTokens,
			r.Usage.OutputTokens, tabs, scr, comp, area, notif, misses, strings.ReplaceAll(sum, "|", "/"))
	}
	s := run.Summary
	fmt.Printf("\nn=%d ok=%d firstPass=%d regenerated=%d areaFallback=%d failed=%d expectMisses=%d\n", s.N, s.OK, s.FirstPass, s.Regenerated, s.AreaFallback, s.Failed, s.ExpectMisses)
	fmt.Printf("create p50=%dms p95=%dms first-event p50=%dms | edit p50=%dms | avg out tokens all=%.0f create=%.0f edit=%.0f | avg create chars=%.0f | avg area ratio=%.2f | tabbed creates=%d | calc creates=%d\n",
		s.CreateP50Ms, s.CreateP95Ms, s.CreateFirstP50Ms, s.EditP50Ms, s.AvgOutTokens, s.AvgCreateOut, s.AvgEditOut, s.AvgCreateOutChars, s.AvgAreaRatio, s.TabbedCreates, s.CalcCreates)
	for _, r := range run.Cases {
		if r.Metrics == nil {
			continue
		}
		lp := "-"
		if r.LogicPass != nil {
			lp = fmt.Sprintf("applied=%v ops=%d att=%d %dms out=%d problems=%d", r.LogicPass.Applied, r.LogicPass.Ops, r.LogicPass.Attempts, r.LogicPass.ClaudeMs, r.LogicPass.Usage.OutputTokens, len(r.LogicPass.Problems))
		}
		fmt.Printf("logic %s: computed=%v statValue=%d settlement=%v ref=%d progressTarget=%v | pass2 %s\n",
			r.ID, r.Metrics.Computed, r.Metrics.StatValues, r.Metrics.Settlement, r.Metrics.RefFields, r.Metrics.ProgressTo, lp)
	}
}

// ---- compare ----------------------------------------------------------------

type side struct {
	name  string
	recs  map[string][]Record // case id -> runs
	order []string
	all   []Record
}

func loadSide(name, glob string) (*side, error) {
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no files match %q", name, glob)
	}
	s := &side{name: name, recs: map[string][]Record{}}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(raw, &run); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, r := range run.Cases {
			if _, seen := s.recs[r.ID]; !seen {
				s.order = append(s.order, r.ID)
			}
			s.recs[r.ID] = append(s.recs[r.ID], r)
			s.all = append(s.all, r)
		}
	}
	return s, nil
}

func medianOf(recs []Record, f func(Record) int64) int64 {
	var v []int64
	for _, r := range recs {
		if r.OK {
			v = append(v, f(r))
		}
	}
	return percentile(v, 0.5)
}

func okCount(recs []Record) (ok, first int) {
	for _, r := range recs {
		if r.OK {
			ok++
			if r.Regenerations == 0 && !r.AreaFallback {
				first++
			}
		}
	}
	return ok, first
}

func pct(a, b int64) string {
	if a == 0 {
		return "-"
	}
	return fmt.Sprintf("%+.0f%%", float64(b-a)/float64(a)*100)
}

// compare prints per-case medians and the aggregate before/after table.
func compare(beforeGlob, afterGlob string) error {
	before, err := loadSide("before", beforeGlob)
	if err != nil {
		return err
	}
	after, err := loadSide("after", afterGlob)
	if err != nil {
		return err
	}
	ms := func(r Record) int64 { return r.ClaudeMs }
	out := func(r Record) int64 { return int64(r.Usage.OutputTokens) }
	first := func(r Record) int64 { return r.FirstEventMs }
	fmt.Println("| id | kind | runs (before/after) | 一発通過 | Claude ms 中央値 before → after | 出力 tokens 中央値 before → after | first event ms (after) |")
	fmt.Println("|---|---|---|---|---|---|---|")
	ids := append([]string(nil), before.order...)
	for _, id := range after.order {
		if _, ok := before.recs[id]; !ok {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		b, a := before.recs[id], after.recs[id]
		kind := ""
		if len(b) > 0 {
			kind = b[0].Kind
		} else if len(a) > 0 {
			kind = a[0].Kind
		}
		bOK, bFirst := okCount(b)
		aOK, aFirst := okCount(a)
		fe := "-"
		if v := medianOf(a, first); v > 0 {
			fe = fmt.Sprint(v)
		}
		fmt.Printf("| %s | %s | %d/%d | %d/%d → %d/%d | %d → %d (%s) | %d → %d (%s) | %s |\n",
			id, kind, len(b), len(a), bFirst, bOK, aFirst, aOK,
			medianOf(b, ms), medianOf(a, ms), pct(medianOf(b, ms), medianOf(a, ms)),
			medianOf(b, out), medianOf(a, out), pct(medianOf(b, out), medianOf(a, out)), fe)
	}
	bs, as := summarize(before.all), summarize(after.all)
	fmt.Println()
	fmt.Println("| 指標 | before | after | 差 |")
	fmt.Println("|---|---|---|---|")
	fmt.Printf("| 一発通過 | %d/%d | %d/%d | |\n", bs.FirstPass, bs.N, as.FirstPass, as.N)
	fmt.Printf("| create p50 (Claude ms) | %d | %d | %s |\n", bs.CreateP50Ms, as.CreateP50Ms, pct(bs.CreateP50Ms, as.CreateP50Ms))
	fmt.Printf("| create p95 (Claude ms) | %d | %d | %s |\n", bs.CreateP95Ms, as.CreateP95Ms, pct(bs.CreateP95Ms, as.CreateP95Ms))
	fmt.Printf("| create first-event p50 (ms) | %d | %d | |\n", bs.CreateFirstP50Ms, as.CreateFirstP50Ms)
	fmt.Printf("| edit p50 (Claude ms) | %d | %d | %s |\n", bs.EditP50Ms, as.EditP50Ms, pct(bs.EditP50Ms, as.EditP50Ms))
	fmt.Printf("| 平均出力 tokens (create) | %.0f | %.0f | %s |\n", bs.AvgCreateOut, as.AvgCreateOut, pct(int64(bs.AvgCreateOut), int64(as.AvgCreateOut)))
	fmt.Printf("| 平均出力 tokens (edit) | %.0f | %.0f | %s |\n", bs.AvgEditOut, as.AvgEditOut, pct(int64(bs.AvgEditOut), int64(as.AvgEditOut)))
	fmt.Printf("| 平均出力 chars (create) | %.0f | %.0f | |\n", bs.AvgCreateOutChars, as.AvgCreateOutChars)
	fmt.Printf("| area 付与率 | %.2f | %.2f | |\n", bs.AvgAreaRatio, as.AvgAreaRatio)
	fmt.Printf("| expect miss | %d | %d | |\n", bs.ExpectMisses, as.ExpectMisses)
	return nil
}

// loadEnvFile sets KEY=VALUE lines (export prefix, quotes and # comments
// allowed) that are not already set. Values are never logged.
func loadEnvFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}
