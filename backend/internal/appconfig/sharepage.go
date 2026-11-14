package appconfig

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// AppStoreURL is Tailor It on the App Store (#278).
const AppStoreURL = "https://apps.apple.com/app/id6828742045"

// maxSharePageBytes caps the rendered page; a bigger one falls back to the
// name and the call to action only.
const maxSharePageBytes = 512 << 10

// sharePageSrc is the unauthenticated web view of a share link (GET /s/{token}):
// the app's screens read-only (shareview.go, #362) plus "open in app" /
// App Store and the steps for someone without the app (#278). Records are
// never shown (#253). No script runs: the CSP allows only the nonce'd <style>.
// Fixed wording comes from sharePageText via {{t "key"}}.
const sharePageSrc = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow">
<meta name="color-scheme" content="light dark">
{{if .Found}}<title>{{.Name}} - Tailor It</title>
<meta property="og:type" content="website">
<meta property="og:site_name" content="Tailor It">
<meta property="og:title" content="{{.Name}}">
<meta property="og:description" content="{{if .Description}}{{.Description}}{{else}}{{t "ogDefault"}}{{end}}">
{{if .PageURL}}<meta property="og:url" content="{{.PageURL}}">
{{end}}<meta name="twitter:card" content="summary">
{{else}}<title>{{t "invalid"}} - Tailor It</title>
{{end}}<style nonce="{{.Nonce}}">
:root{--bg:#f6f5fb;--fg:#17142a;--muted:#5d5875;--card:#fff;--line:#dedbea;--accent:#6a4cff;--row:40px}
@media (prefers-color-scheme:dark){:root{--bg:#0f0c1d;--fg:#f3f1ff;--muted:#a9a3c6;--card:#1b1730;--line:#332d52;--accent:#9a85ff}}
*{box-sizing:border-box}
body{margin:0;overflow-wrap:anywhere;background:var(--bg);color:var(--fg);font:16px/1.6 -apple-system,"Hiragino Sans","Noto Sans JP",sans-serif}
main{max-width:480px;margin:0 auto;padding:16px}
a{color:var(--accent)}
:focus-visible{outline:3px solid var(--accent);outline-offset:2px}
header.app{text-align:center;padding:24px 0 8px}
header.app h1{font-size:24px;margin:4px 0}
.muted,.lead{color:var(--muted)}
.cta{display:flex;flex-direction:column;gap:10px;margin:16px 0 24px}
.btn{display:block;text-align:center;padding:14px 20px;border-radius:999px;font-weight:600;text-decoration:none;border:1px solid var(--line);color:var(--fg);background:var(--card)}
.btn.primary{background:var(--accent);border-color:var(--accent);color:#fff}
nav.screens ul{display:flex;gap:8px;overflow-x:auto;list-style:none;padding:0;margin:0 0 12px}
nav.screens a{display:block;white-space:nowrap;padding:6px 12px;border-radius:999px;background:var(--card);border:1px solid var(--line);text-decoration:none;color:var(--fg);font-size:14px}
section.screen{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:12px;margin:0 0 20px}
section.screen>h2{font-size:18px;margin:0 0 8px;display:flex;justify-content:space-between;gap:8px}
.tb{font-size:14px;color:var(--muted);font-weight:500}
.grid{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));grid-auto-rows:minmax(var(--row),auto);gap:6px}
.stack{display:flex;flex-direction:column;gap:8px;margin-top:8px}
.grid>*,.stack>*{min-width:0}
.row{display:flex;gap:8px;overflow-x:auto}.row>*{flex:1 1 0;min-width:120px}
.cgrid{display:grid;gap:8px}.cols2{grid-template-columns:repeat(2,1fr)}.cols3{grid-template-columns:repeat(3,1fr)}.cols4{grid-template-columns:repeat(4,1fr)}
.box{border:1px solid var(--line);border-radius:12px;padding:10px}
.box>h3,details>summary{font-size:15px;margin:0 0 6px;font-weight:600}
h3.t{font-size:20px;margin:0}h3.h{font-size:17px;margin:0}
p{margin:0}.caption{font-size:13px;color:var(--muted)}
.stat .v{font-size:24px;font-weight:700}
.meter{height:8px;border-radius:4px;background:var(--line)}
.badge{display:inline-block;padding:2px 10px;border-radius:999px;background:var(--line);font-size:13px}
.empty,.ph{display:flex;align-items:center;justify-content:center;text-align:center;min-height:var(--row);padding:8px;border:1px dashed var(--line);border-radius:10px;color:var(--muted);font-size:14px;height:100%}
.skel{font-size:13px;color:var(--muted);border-bottom:1px solid var(--line);padding:4px 0}
ul.rows{list-style:none;margin:0;padding:0}ul.rows li{display:flex;justify-content:space-between;gap:8px;border-bottom:1px solid var(--line);padding:6px 0}
label{display:block;font-size:13px;color:var(--muted)}
input,select,textarea,button{font:inherit;width:100%;color:var(--fg);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:8px}
input[type=checkbox]{width:auto}
button{border-radius:999px;font-weight:600}button.primary{background:var(--accent);color:#fff;border-color:var(--accent)}button.destructive{color:#d33}
:disabled{opacity:.75;cursor:not-allowed}
.unit{display:flex;gap:6px;align-items:center}
.stars{letter-spacing:2px;color:var(--muted)}
.img{min-height:96px}
figure{margin:0}
svg.chart{width:100%;height:96px;color:var(--accent);opacity:.5}
table{width:100%;border-collapse:collapse;font-size:14px}th{text-align:left;color:var(--muted);font-weight:600;border-bottom:1px solid var(--line);padding:4px}
.icon{display:inline-block;width:20px;height:20px;border-radius:50%;background:var(--line)}
hr{border:0;border-top:1px solid var(--line);width:100%}
section.install{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:12px 16px;margin:0 0 20px}
section.install>h2{font-size:16px;margin:0 0 6px}
section.install ol{margin:0;padding-left:1.4em}
footer{font-size:13px;color:var(--muted);text-align:center;padding:8px 0 32px}
{{if .Accent}}:root{--accent:{{.Accent}}}
{{end}}{{range .CSS}}.{{.Class}}{grid-{{.Prop}}:{{.From}}/{{.To}}}
{{end}}</style></head>
<body><main>
{{if .Found}}<header class="app"><p class="muted">{{t "shared"}}</p>
<h1>{{.Name}}</h1>
{{with .Description}}<p class="lead">{{.}}</p>{{end}}</header>
<div class="cta"><a class="btn primary" href="{{.ImportURL}}">{{t "open"}}</a>
<a class="btn" href="{{.StoreURL}}" rel="noopener">{{t "get"}}</a></div>
<section class="install" aria-labelledby="install-h"><h2 id="install-h">{{t "installH"}}</h2>
<p class="caption">{{t "installLead"}}</p>
<ol><li>{{t "install1"}}</li><li>{{t "install2"}}</li><li>{{t "install3"}}</li></ol></section>
<p class="caption">{{t "sample"}}</p>
{{if .Screens}}<nav class="screens" aria-label="{{t "screens"}}"><ul>{{range .Screens}}<li><a href="#{{.Anchor}}">{{if .Title}}{{.Title}}{{else}}{{t "screen"}}{{end}}</a></li>{{end}}</ul></nav>
{{range .Screens}}<section class="screen" id="{{.Anchor}}" aria-labelledby="{{.Anchor}}h">
<h2 id="{{.Anchor}}h">{{if .Title}}{{.Title}}{{else}}{{t "screen"}}{{end}}{{if .Toolbar}}<span class="tb">{{range $i, $b := .Toolbar}}{{if $i}} · {{end}}{{$b}}{{end}}</span>{{end}}</h2>
{{if .Grid}}<div class="grid">{{range .Grid}}{{template "node" .}}{{end}}</div>{{end}}
{{if .Stack}}<div class="stack">{{range .Stack}}{{template "node" .}}{{end}}</div>{{end}}
</section>
{{end}}{{if .Truncated}}<p class="caption">{{t "more"}}</p>{{end}}{{end}}
{{else}}<header class="app"><h1>{{t "invalid"}}</h1><p class="muted">{{t "invalidLead"}}</p></header>
<div class="cta"><a class="btn" href="{{.StoreURL}}" rel="noopener">{{t "see"}}</a></div>
{{end}}<footer>Tailor It</footer>
</main></body></html>
{{define "node"}}<div class="{{.Class}}">{{template "leaf" .}}</div>{{end}}
{{define "children"}}{{if .Grid}}<div class="grid">{{range .Children}}{{template "node" .}}{{end}}</div>{{else}}{{range .Children}}{{template "leaf" .}}{{end}}{{end}}{{end}}
{{define "leaf"}}{{- if eq .Kind "section" "card"}}<div class="box">{{with .Title}}<h3>{{.}}</h3>{{end}}<div class="stack">{{template "children" .}}</div></div>
{{- else if eq .Kind "stack"}}<div class="stack">{{template "children" .}}</div>
{{- else if eq .Kind "row"}}<div class="row">{{template "children" .}}</div>
{{- else if eq .Kind "grid"}}<div class="cgrid cols{{.Cols}}">{{template "children" .}}</div>
{{- else if eq .Kind "disclosure"}}<details class="box"{{if .Open}} open{{end}}><summary>{{if .Title}}{{.Title}}{{else}}{{t "details"}}{{end}}</summary><div class="stack">{{template "children" .}}</div></details>
{{- else if eq .Kind "heading"}}<h3 class="{{if eq .Style "title"}}t{{else}}h{{end}}">{{.Text}}</h3>
{{- else if eq .Kind "text"}}<p{{if eq .Style "caption"}} class="caption"{{end}}>{{.Text}}</p>
{{- else if eq .Kind "badge"}}<span class="badge">{{.Text}}</span>
{{- else if eq .Kind "divider"}}<hr>
{{- else if eq .Kind "spacer"}}<div aria-hidden="true"></div>
{{- else if eq .Kind "icon"}}<span class="icon" aria-hidden="true"></span>
{{- else if eq .Kind "empty"}}<div class="empty"><div>{{with .Title}}<strong>{{.}}</strong><br>{{end}}{{.Text}}</div></div>
{{- else if eq .Kind "stat"}}<div class="box stat"><div class="caption">{{.Label}}</div><div class="v" aria-label="{{t "noData"}}">—</div></div>
{{- else if eq .Kind "meter"}}<div class="box"><div class="caption">{{.Label}}</div><div class="meter" role="img" aria-label="{{t "noData"}}"></div></div>
{{- else if eq .Kind "list"}}<div class="box">{{with .Items}}<div class="skel" aria-hidden="true">{{range $i, $f := .}}{{if $i}} / {{end}}{{$f}}{{end}}</div>{{end}}{{if .Rows}}<ul class="rows">{{range .Rows}}<li><span>{{.Title}}{{with .Subtitle}}<br><span class="caption">{{.}}</span>{{end}}</span><span>{{.Trailing}}</span></li>{{end}}</ul>{{else}}<div class="empty">{{if .Text}}{{.Text}}{{else}}{{t "records"}}{{end}}</div>{{end}}</div>
{{- else if eq .Kind "table"}}<div class="box"><table><thead><tr>{{range .Items}}<th scope="col">{{.}}</th>{{end}}</tr></thead></table><div class="empty">{{t "records"}}</div></div>
{{- else if eq .Kind "chart"}}<figure class="box">{{with .Title}}<figcaption class="caption">{{.}}</figcaption>{{end}}{{if eq .Style "pie"}}<svg class="chart" viewBox="0 0 120 96" aria-hidden="true"><circle cx="60" cy="48" r="40" fill="none" stroke="currentColor" stroke-width="16" stroke-dasharray="120 131"/></svg>{{else}}<svg class="chart" viewBox="0 0 120 96" aria-hidden="true"><rect x="6" y="56" width="16" height="36" fill="currentColor"/><rect x="30" y="36" width="16" height="56" fill="currentColor"/><rect x="54" y="64" width="16" height="28" fill="currentColor"/><rect x="78" y="24" width="16" height="68" fill="currentColor"/><rect x="102" y="46" width="16" height="46" fill="currentColor"/></svg>{{end}}<p class="caption">{{t "chart"}}</p></figure>
{{- else if eq .Kind "image"}}<div class="ph img" role="img" aria-label="{{t "photo"}}">{{t "photo"}}</div>
{{- else if eq .Kind "input"}}<div>{{with .Label}}<label for="{{$.ID}}">{{.}}</label>{{end}}<div class="unit"><input id="{{.ID}}" type="{{.Style}}"{{with .Placeholder}} placeholder="{{.}}"{{end}} disabled>{{with .Unit}}<span>{{.}}</span>{{end}}</div></div>
{{- else if eq .Kind "textarea"}}<div>{{with .Label}}<label for="{{$.ID}}">{{.}}</label>{{end}}<textarea id="{{.ID}}" rows="3"{{with .Placeholder}} placeholder="{{.}}"{{end}} disabled></textarea></div>
{{- else if eq .Kind "select"}}<div>{{with .Label}}<label for="{{$.ID}}">{{.}}</label>{{end}}<select id="{{.ID}}" disabled>{{range .Options}}<option>{{.}}</option>{{end}}</select></div>
{{- else if eq .Kind "toggle"}}<label><input id="{{.ID}}" type="checkbox" disabled> {{.Label}}</label>
{{- else if eq .Kind "rating"}}<div>{{with .Label}}<span class="caption">{{.}}</span> {{end}}<span class="stars" role="img" aria-label="{{printf (t "rating") .Cols}}">{{.Text}}</span></div>
{{- else if eq .Kind "button"}}<button type="button" class="{{.Style}}" disabled>{{if .Label}}{{.Label}}{{else}}{{t "share"}}{{end}}</button>
{{- else if eq .Kind "link"}}{{if .Href}}<a href="{{.Href}}" rel="nofollow noopener noreferrer ugc" target="_blank">{{.Text}}</a>{{else}}<span>{{.Text}}</span>{{end}}
{{- else if eq .Kind "flashcards"}}<div class="ph">{{with .Items}}{{printf (t "cards") (index . 0) (index . 1)}}<br>{{end}}{{t "inApp"}}</div>
{{- else}}<div class="ph">{{with .Title}}{{.}}<br>{{end}}{{t "inApp"}}</div>
{{- end}}{{end}}`

// sharePageText is the page's fixed wording (#278). The keys must match in
// every language (TestSharePageTextKeys).
var sharePageText = map[string]map[string]string{
	"ja": {
		"ogDefault":   "Tailor It で作られたアプリが共有されました",
		"invalid":     "リンクが無効です",
		"invalidLead": "このシェアリンクは取り消されたか、存在しません。",
		"see":         "App Store で Tailor It を見る",
		"shared":      "アプリが共有されました",
		"open":        "アプリで開く",
		"get":         "App Store で入手",
		"installH":    "Tailor It をまだ入れていない場合",
		"installLead": "「アプリで開く」で開けない時は、Tailor It が入っていません。次の順で参加できます。",
		"install1":    "「App Store で入手」から Tailor It をインストールします。",
		"install2":    "Tailor It を開いて、サインインします。",
		"install3":    "このページに戻って、もう一度「アプリで開く」を押すと参加できます(届いたリンクを開き直しても同じです)。",
		"sample":      "この画面は見本です。記録されたデータは表示しません。",
		"screens":     "画面",
		"screen":      "画面",
		"more":        "続きはアプリで見られます。",
		"details":     "詳細",
		"noData":      "データなし",
		"records":     "記録はアプリで見られます",
		"chart":       "グラフはアプリで見られます",
		"photo":       "写真",
		"rating":      "評価 %d 段階",
		"inApp":       "アプリで見られます",
		"cards":       "カード(表: %s / 裏: %s)",
		"share":       "共有",
	},
	"en": {
		"ogDefault":   "An app made with Tailor It was shared with you",
		"invalid":     "This link is not valid",
		"invalidLead": "This share link was revoked or does not exist.",
		"see":         "See Tailor It on the App Store",
		"shared":      "An app was shared with you",
		"open":        "Open in the app",
		"get":         "Get it on the App Store",
		"installH":    "Don't have Tailor It yet?",
		"installLead": "If \"Open in the app\" does nothing, Tailor It is not installed. Join in three steps:",
		"install1":    "Install Tailor It from \"Get it on the App Store\".",
		"install2":    "Open Tailor It and sign in.",
		"install3":    "Come back to this page and tap \"Open in the app\" again to join (opening the link you received again works too).",
		"sample":      "This is a preview. Recorded data is not shown.",
		"screens":     "Screens",
		"screen":      "Screen",
		"more":        "See the rest in the app.",
		"details":     "Details",
		"noData":      "No data",
		"records":     "Records are shown in the app",
		"chart":       "Charts are shown in the app",
		"photo":       "Photo",
		"rating":      "Rating out of %d",
		"inApp":       "Shown in the app",
		"cards":       "Cards (front: %s / back: %s)",
		"share":       "Share",
	},
}

// sharePages holds the page template per language.
var sharePages = func() map[string]*template.Template {
	out := map[string]*template.Template{}
	for lang, text := range sharePageText {
		t := func(key string) string { return text[key] }
		out[lang] = template.Must(template.New("share").Funcs(template.FuncMap{"t": t}).Parse(sharePageSrc))
	}
	return out
}()

// shareLang picks ja or en from Accept-Language: the first ja / en tag in the
// sender's order (q-values are not ranked; browsers list by preference).
// No header keeps the original Japanese page; any other language gets en.
func shareLang(accept string) string {
	if strings.TrimSpace(accept) == "" {
		return "ja"
	}
	for _, part := range strings.Split(accept, ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		primary, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		if primary == "ja" || primary == "en" {
			return primary
		}
	}
	return "en"
}

// shareHidden reports whether the operator unpublished the shared app
// (#321). Hidden apps get the invalid-link page, like a revoked share. A
// failed lookup also hides it: the page must not show an unpublished app
// because the store was briefly unavailable (checkSharedApp fails too).
func shareHidden(ctx context.Context, s store.Store, sh *store.Share) bool {
	p, err := s.GetPublicApp(ctx, sh.AppID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return false
	case err != nil:
		slog.Warn("appconfig: share page moderation lookup failed", slog.Any("error", err))
		return true
	}
	return p.Hidden
}

func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]) // html/template would entity-escape + and /
}

// shareCSP: no scripts, no fetches, no frames; only the nonce'd style block.
func shareCSP(nonce string) string {
	return "default-src 'none'; style-src 'nonce-" + nonce + "'; img-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}

// SharePage serves GET /s/{token}. Mount it outside auth.
func SharePage(s store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := webPage{StoreURL: AppStoreURL}
		lang := shareLang(r.Header.Get("Accept-Language"))
		tok := r.PathValue("token")
		if validShareToken(tok) {
			if sh, err := s.GetShare(r.Context(), tok); err == nil && !sh.Revoked && !shareHidden(r.Context(), s, sh) {
				page = buildWebPage(sh.Name, sh.DSL, nil)         // nil RecordPeek: no records (#253)
				page.ImportURL = template.URL(ImportScheme + tok) // #nosec G203 -- tok matched validShareToken
				page.PageURL = webBase(r) + "/s/" + tok
			}
		}
		page.Lang, page.Nonce = lang, newNonce()
		body, err := renderSharePage(page)
		if err != nil {
			slog.Warn("appconfig: share page render failed", slog.Any("error", err))
			page.Screens, page.CSS, page.Truncated = nil, nil, false
			if body, err = renderSharePage(page); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", shareCSP(page.Nonce))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer") // the token is a capability
		h.Set("X-Robots-Tag", "noindex, nofollow")
		h.Set("Vary", "Accept-Language")
		if !page.Found {
			h.Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotFound)
		} else {
			// short and private: a revoke / unpublish shows within a minute
			h.Set("Cache-Control", "private, max-age=60")
		}
		_, _ = w.Write(body)
	}
}

var errPageTooBig = errors.New("share page exceeds size limit")

func renderSharePage(p webPage) ([]byte, error) {
	tmpl, ok := sharePages[p.Lang]
	if !ok {
		tmpl = sharePages["ja"]
		p.Lang = "ja"
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return nil, err
	}
	if buf.Len() > maxSharePageBytes {
		return nil, errPageTooBig
	}
	return buf.Bytes(), nil
}
