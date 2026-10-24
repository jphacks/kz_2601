// smoke drives the three demo utterances (docs/03_demo_script.md) against a
// running ai (+ appconfig) and prints one Markdown table: step, ms, Claude ms,
// tokens, result. Exit status is non-zero when any step fails, so it can run
// after a staging deploy (non-blocking) and before the demo (make smoke-stg).
//
//	go run ./cmd/smoke -ai http://localhost:8081 -appconfig http://localhost:8080          # AUTH_MODE=dev
//	go run ./cmd/smoke -ai $AI_URL -appconfig $APPCONFIG_URL -firebase-api-key $KEY       # staging (firebase)
//	go run ./cmd/smoke ... -only kakeibo                                                   # one scenario
//	go run ./cmd/smoke ... -only kakeibo -cleanup                                          # production: archive smoke-* apps at the end
//
// Token resolution: -token > -firebase-api-key (anonymous sign-up via the
// identitytoolkit REST API) > $SMOKE_UID or "dev-smoke" (appconfig AUTH_MODE=dev).
// When the resolved token is rejected by appconfig (401) the run falls back to
// health + "ai rejects unauthenticated generate" checks only.
//
// -cleanup archives every app the run created (DELETE /v1/apps/{id}, status=archived)
// so production Firestore does not accumulate smoke-* apps. Needs -appconfig.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Demo utterances: the exact strings measured in internal/ai/eval/cases.json.
const (
	sayKakeibo  = "毎日の出費をメモできる家計簿アプリを作って。今月いくら使ったかと支出の一覧、カテゴリ別のグラフも見たい。レシートの写真も残せるようにして"
	sayBigger   = "今月の合計をもっと大きく表示して"
	sayUndo     = "元に戻して"
	sayTraining = "筋トレの記録アプリ。腕立てとスクワットの回数を毎日つけて、朝8時にリマインドして"
	sayWalk     = "散歩で見つけたいい場所を写真とひとことで残すメモアプリ。夜に見返したい"
)

type step struct {
	Name     string
	OK       bool
	Warn     bool
	Ms       int64
	ClaudeMs int64
	Tokens   string
	Note     string
}

type runner struct {
	ai, appconfig string
	token         string // full Authorization value
	tokenKind     string // flag|firebase|dev
	http          *http.Client
	store         *appconfigclient.HTTP // nil when -appconfig is empty
	created       []string              // appIds this run created (archived by -cleanup)
	steps         []step
}

func main() {
	var (
		aiURL   = flag.String("ai", "http://localhost:8081", "ai base URL")
		cfgURL  = flag.String("appconfig", "http://localhost:8080", "appconfig base URL (empty = skip appconfig cross-checks)")
		token   = flag.String("token", "", "bearer token (default: Firebase anonymous via -firebase-api-key, else $SMOKE_UID / dev-smoke)")
		apiKey  = flag.String("firebase-api-key", os.Getenv("FIREBASE_WEB_API_KEY"), "Firebase Web API key for anonymous sign-up")
		only    = flag.String("only", "", "run one scenario: kakeibo|training|walk")
		appID   = flag.String("app", "", "appId prefix (default smoke-<unix>)")
		timeout = flag.Duration("timeout", 90*time.Second, "per-request timeout")
		cleanup = flag.Bool("cleanup", false, "archive the apps this run created (DELETE /v1/apps/{id}) at the end")
	)
	flag.Parse()
	if *appID == "" {
		*appID = fmt.Sprintf("smoke-%d", time.Now().Unix())
	}
	r := &runner{
		ai:        strings.TrimRight(*aiURL, "/"),
		appconfig: strings.TrimRight(*cfgURL, "/"),
		http:      &http.Client{Timeout: *timeout},
	}
	if r.appconfig != "" {
		r.store = appconfigclient.NewHTTP(r.appconfig)
		r.store.Client = r.http
	}
	ctx := context.Background()

	fmt.Printf("### smoke: ai=%s appconfig=%s app=%s\n\n", r.ai, orDash(r.appconfig), *appID)
	r.health(ctx)
	r.resolveToken(ctx, *token, *apiKey)
	fallback := r.authProbe(ctx, *appID)
	if fallback {
		r.unauthenticatedGenerateIs401(ctx, *appID)
	} else {
		switch *only {
		case "", "kakeibo":
			r.kakeibo(ctx, *appID)
		}
		switch *only {
		case "", "training":
			r.training(ctx, *appID+"-training")
		}
		switch *only {
		case "", "walk":
			r.walk(ctx, *appID+"-walk")
		}
	}
	if *cleanup {
		r.cleanup(ctx)
	}
	failed := r.print(fallback)
	if failed > 0 {
		os.Exit(1)
	}
}

// ---- scenarios --------------------------------------------------------------

func (r *runner) kakeibo(ctx context.Context, appID string) {
	r.track(appID)
	bctx := appconfigclient.WithBearer(ctx, r.token)
	if r.store != nil {
		t := time.Now()
		env, err := r.store.Get(bctx, appID)
		switch {
		case err != nil:
			r.fail("A-0 GET /dsl (自動作成)", t, err.Error())
		case env.Version != 1:
			r.fail("A-0 GET /dsl (自動作成)", t, fmt.Sprintf("fresh appId must be version 1, got %d", env.Version))
		default:
			r.pass("A-0 GET /dsl (自動作成)", t, "version 1 (initial)")
		}
	}

	created, ok := r.generate(ctx, "A-1 家計簿 create", appID, sayKakeibo, func(res *ai.Result, app *dsl.App) error {
		if res.Mode != "full" {
			return fmt.Errorf("mode %s, want full", res.Mode)
		}
		if n := overlaps(app); n != 0 {
			return fmt.Errorf("%d overlapping areas", n)
		}
		return nil
	}, func(res *ai.Result, app *dsl.App) string {
		return fmt.Sprintf("v%d screens=%d tabs=%d overlaps=0 %s", res.Version, len(app.Screens), len(app.App.Tabs), res.Meta.Model)
	})
	if !ok {
		return
	}
	edited, ok := r.generate(ctx, "A-2 「"+sayBigger+"」 edit", appID, sayBigger, func(res *ai.Result, app *dsl.App) error {
		if res.Version != created.Version+1 {
			return fmt.Errorf("version %d, want %d", res.Version, created.Version+1)
		}
		if n := overlaps(app); n != 0 {
			return fmt.Errorf("%d overlapping areas", n)
		}
		return nil
	}, func(res *ai.Result, _ *dsl.App) string {
		note := fmt.Sprintf("v%d mode=%s ops=%d", res.Version, res.Mode, len(res.Patch))
		if res.Mode != "patch" {
			note += " (patch -> full に降格)"
		}
		return note
	})
	if !ok {
		return
	}
	_, ok = r.generate(ctx, "A-3 「"+sayUndo+"」 undo", appID, sayUndo, func(res *ai.Result, _ *dsl.App) error {
		if res.Mode != "restore" || res.RestoredFrom != created.Version {
			return fmt.Errorf("mode=%s restoredFrom=%d, want restore from %d", res.Mode, res.RestoredFrom, created.Version)
		}
		if res.Version != edited.Version+1 {
			return fmt.Errorf("version %d, want %d", res.Version, edited.Version+1)
		}
		if !bytes.Equal(canon(res.DSL), canon(created.DSL)) {
			return errors.New("restored DSL differs from the create result")
		}
		return nil
	}, func(res *ai.Result, _ *dsl.App) string {
		return fmt.Sprintf("v%d == v%d の DSL / %s", res.Version, res.RestoredFrom, res.Summary)
	})
	if !ok || r.store == nil {
		return
	}
	t := time.Now()
	vs, err := r.store.Versions(bctx, appID)
	if err != nil {
		r.fail("A-4 appconfig GET /versions", t, err.Error())
		return
	}
	switch {
	case len(vs) == 0:
		r.fail("A-4 appconfig GET /versions", t, "no versions")
	case vs[0].Version != edited.Version+1:
		r.warn("A-4 appconfig GET /versions", t, fmt.Sprintf("appconfig latest=v%d but ai returned v%d: ai is on its in-memory store (APPCONFIG_BASE_URL unset?)", vs[0].Version, edited.Version+1))
	case vs[0].Source == nil || vs[0].Source.Kind != "restore":
		r.fail("A-4 appconfig GET /versions", t, "latest version is not source.kind=restore")
	default:
		r.pass("A-4 appconfig GET /versions", t, fmt.Sprintf("%d versions, latest v%d restore←v%d", len(vs), vs[0].Version, vs[0].Source.RestoredFrom))
	}
}

func (r *runner) training(ctx context.Context, appID string) {
	r.track(appID)
	r.generate(ctx, "B 筋トレ create(通知)", appID, sayTraining, func(_ *ai.Result, app *dsl.App) error {
		for _, n := range app.App.Notifications {
			if n.Repeat == "daily" && strings.HasPrefix(n.Time, "08:") {
				return nil
			}
		}
		return fmt.Errorf("no daily 08:xx notification (got %d)", len(app.App.Notifications))
	}, func(res *ai.Result, app *dsl.App) string {
		return fmt.Sprintf("v%d screens=%d notifications=%d overlaps=%d", res.Version, len(app.Screens), len(app.App.Notifications), overlaps(app))
	})
}

func (r *runner) walk(ctx context.Context, appID string) {
	r.track(appID)
	r.generate(ctx, "C 散歩 create(写真)", appID, sayWalk, func(_ *ai.Result, app *dsl.App) error {
		if src := photoSource(app); src == "" {
			return errors.New("no photoField")
		} else if src != "camera" && src != "both" {
			return fmt.Errorf("photoField source=%s, want camera", src)
		}
		return nil
	}, func(res *ai.Result, app *dsl.App) string {
		return fmt.Sprintf("v%d screens=%d photoField{source:%s} overlaps=%d", res.Version, len(app.Screens), photoSource(app), overlaps(app))
	})
}

// ---- plumbing ---------------------------------------------------------------

func (r *runner) health(ctx context.Context) {
	for _, u := range []string{r.ai, r.appconfig} {
		if u == "" {
			continue
		}
		t := time.Now()
		status, _, err := r.do(ctx, http.MethodGet, u+"/health", "", nil)
		if err != nil || status != 200 {
			r.fail("health "+u, t, fmt.Sprintf("status=%d err=%v", status, err))
			continue
		}
		r.pass("health "+u, t, "200")
	}
}

func (r *runner) resolveToken(ctx context.Context, flagToken, apiKey string) {
	switch {
	case flagToken != "":
		r.token, r.tokenKind = "Bearer "+strings.TrimPrefix(flagToken, "Bearer "), "flag"
		return
	case apiKey != "":
		t := time.Now()
		id, uid, err := firebaseAnonymous(ctx, r.http, apiKey)
		if err == nil {
			r.token, r.tokenKind = "Bearer "+id, "firebase"
			r.pass("Firebase 匿名サインアップ", t, "uid="+uid)
			return
		}
		r.warn("Firebase 匿名サインアップ", t, err.Error()+" -> dev token にフォールバック")
	}
	uid := os.Getenv("SMOKE_UID")
	if uid == "" {
		uid = "dev-smoke"
	}
	r.token, r.tokenKind = "Bearer "+uid, "dev"
}

// authProbe reports whether the run must fall back (token rejected by appconfig).
func (r *runner) authProbe(ctx context.Context, appID string) bool {
	if r.store == nil {
		return false
	}
	t := time.Now()
	probeID := appID + "-probe"
	status, _, err := r.do(ctx, http.MethodGet, r.appconfig+"/v1/apps/"+probeID+"/dsl", r.token, nil)
	switch {
	case err != nil:
		r.fail("auth probe ("+r.tokenKind+" token)", t, err.Error())
		return true
	case status == http.StatusUnauthorized:
		r.warn("auth probe ("+r.tokenKind+" token)", t, "appconfig returned 401 (AUTH_MODE=firebase?) -> health/401 checks only")
		return true
	case status != 200:
		r.fail("auth probe ("+r.tokenKind+" token)", t, fmt.Sprintf("status=%d", status))
		return true
	}
	r.pass("auth probe ("+r.tokenKind+" token)", t, "200")
	r.track(probeID) // GET /dsl auto-created it
	return false
}

// track remembers an appId so -cleanup can archive it.
func (r *runner) track(appID string) {
	r.created = append(r.created, appID)
}

// cleanup archives every tracked app via DELETE /v1/apps/{id} (204). 404 = already
// gone (WARN); anything else is a FAIL so leaked smoke-* apps show up in the table.
func (r *runner) cleanup(ctx context.Context) {
	if r.appconfig == "" {
		return
	}
	for _, id := range r.created {
		name := "cleanup DELETE /v1/apps/" + id
		t := time.Now()
		status, data, err := r.do(ctx, http.MethodDelete, r.appconfig+"/v1/apps/"+id, r.token, nil)
		switch {
		case err != nil:
			r.fail(name, t, err.Error())
		case status == http.StatusNoContent:
			r.pass(name, t, "archived")
		case status == http.StatusNotFound:
			r.warn(name, t, "404 (already gone)")
		default:
			r.fail(name, t, fmt.Sprintf("status=%d %s", status, trunc(string(data), 120)))
		}
	}
}

func (r *runner) unauthenticatedGenerateIs401(ctx context.Context, appID string) {
	t := time.Now()
	body, _ := json.Marshal(ai.Request{AppID: appID + "-noauth", Transcript: "家計簿", Mode: ai.ModeUndo})
	status, _, err := r.do(ctx, http.MethodPost, r.ai+"/v1/generate", "", body)
	switch {
	case err != nil:
		r.fail("ai POST /v1/generate (no token) is 401", t, err.Error())
	case status == http.StatusUnauthorized:
		r.pass("ai POST /v1/generate (no token) is 401", t, "ai -> appconfig auth wired")
	default:
		r.warn("ai POST /v1/generate (no token) is 401", t, fmt.Sprintf("status=%d: ai accepts anonymous calls (in-memory store?)", status))
	}
}

// generate posts one utterance, parses the DSL, runs check and records a step.
func (r *runner) generate(ctx context.Context, name, appID, text string,
	check func(*ai.Result, *dsl.App) error, note func(*ai.Result, *dsl.App) string) (*ai.Result, bool) {
	t := time.Now()
	body, _ := json.Marshal(ai.Request{AppID: appID, Transcript: text})
	status, data, err := r.do(ctx, http.MethodPost, r.ai+"/v1/generate", r.token, body)
	if err != nil {
		r.fail(name, t, err.Error())
		return nil, false
	}
	if status != 200 {
		r.fail(name, t, fmt.Sprintf("status=%d %s", status, trunc(string(data), 200)))
		return nil, false
	}
	var res ai.Result
	if err := json.Unmarshal(data, &res); err != nil {
		r.fail(name, t, "bad JSON: "+err.Error())
		return nil, false
	}
	app, err := dsl.Parse(res.DSL)
	if err != nil {
		r.fail(name, t, "dsl.Parse: "+trunc(err.Error(), 200))
		return nil, false
	}
	s := step{Name: name, Ms: time.Since(t).Milliseconds(), ClaudeMs: res.LatencyMs.Claude}
	if u := res.Meta.Usage; u.InputTokens+u.OutputTokens > 0 {
		s.Tokens = fmt.Sprintf("%d/%d", u.InputTokens, u.OutputTokens)
		if u.CacheReadInputTokens > 0 {
			s.Tokens += fmt.Sprintf(" (cache %d)", u.CacheReadInputTokens)
		}
	}
	if err := check(&res, app); err != nil {
		s.Note = err.Error()
		r.steps = append(r.steps, s)
		return &res, false
	}
	s.OK = true
	s.Note = note(&res, app)
	r.steps = append(r.steps, s)
	return &res, true
}

func (r *runner) do(ctx context.Context, method, url, auth string, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, nil
}

// firebaseAnonymous signs up an anonymous user (identitytoolkit accounts:signUp)
// and returns its ID token and uid. Needs "Anonymous" enabled in Firebase Auth.
func firebaseAnonymous(ctx context.Context, c *http.Client, apiKey string) (idToken, uid string, err error) {
	url := "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"returnSecureToken":true}`))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		IDToken string `json:"idToken"`
		LocalID string `json:"localId"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &out)
	if resp.StatusCode != 200 || out.IDToken == "" {
		msg := out.Error.Message
		if msg == "" {
			msg = trunc(string(data), 120)
		}
		return "", "", fmt.Errorf("identitytoolkit signUp: %d %s", resp.StatusCode, msg)
	}
	return out.IDToken, out.LocalID, nil
}

// ---- DSL metrics ------------------------------------------------------------

// overlaps counts overlapping area pairs among screen-direct components.
func overlaps(app *dsl.App) int {
	n := 0
	for _, s := range app.Screens {
		var areas []dsl.Area
		for _, c := range s.Components {
			if c.Area == "" {
				continue
			}
			a, err := dsl.ParseArea(c.Area)
			if err != nil {
				n++
				continue
			}
			for _, p := range areas {
				if a.Overlaps(p) {
					n++
				}
			}
			areas = append(areas, a)
		}
	}
	return n
}

// photoSource returns the source of the first photoField ("" when none).
func photoSource(app *dsl.App) string {
	var walk func([]dsl.Component) string
	walk = func(cs []dsl.Component) string {
		for _, c := range cs {
			if c.Type == dsl.CompPhotoField {
				if c.Source == "" {
					return "default"
				}
				return c.Source
			}
			if s := walk(c.Children); s != "" {
				return s
			}
		}
		return ""
	}
	for _, s := range app.Screens {
		if src := walk(s.Components); src != "" {
			return src
		}
	}
	return ""
}

func canon(raw json.RawMessage) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, _ := json.Marshal(v)
	return out
}

// ---- reporting --------------------------------------------------------------

func (r *runner) pass(name string, t time.Time, note string) {
	r.steps = append(r.steps, step{Name: name, OK: true, Ms: time.Since(t).Milliseconds(), Note: note})
}

func (r *runner) warn(name string, t time.Time, note string) {
	r.steps = append(r.steps, step{Name: name, OK: true, Warn: true, Ms: time.Since(t).Milliseconds(), Note: note})
}

func (r *runner) fail(name string, t time.Time, note string) {
	r.steps = append(r.steps, step{Name: name, Ms: time.Since(t).Milliseconds(), Note: note})
}

// print writes the Markdown table and returns the number of failed steps.
func (r *runner) print(fallback bool) int {
	if fallback {
		fmt.Println("**FALLBACK**: appconfig rejected the smoke token, so only health / auth wiring was checked.")
		fmt.Println()
	}
	fmt.Println("| step | result | ms | claude ms | tokens in/out | note |")
	fmt.Println("|---|---|---:|---:|---|---|")
	failed, warned := 0, 0
	var totalMs, claudeMs int64
	for _, s := range r.steps {
		res := "PASS"
		switch {
		case !s.OK:
			res, failed = "**FAIL**", failed+1
		case s.Warn:
			res, warned = "WARN", warned+1
		}
		totalMs += s.Ms
		claudeMs += s.ClaudeMs
		fmt.Printf("| %s | %s | %d | %s | %s | %s |\n", s.Name, res, s.Ms, orDash(nonZero(s.ClaudeMs)), orDash(s.Tokens), cell(s.Note))
	}
	fmt.Println()
	fmt.Printf("%d steps, %d failed, %d warnings, total %d ms (claude %d ms)\n", len(r.steps), failed, warned, totalMs, claudeMs)
	return failed
}

func nonZero(v int64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprint(v)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
