package appconfig

import (
	"html/template"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// sharePage is the unauthenticated web fallback for a share link (GET /s/{token}):
// it only shows the app name and a button that opens tailorit://import/<token>.
// The DSL itself is never exposed here.
var sharePage = template.Must(template.New("share").Parse(`<!doctype html>
<html lang="ja"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Name}} - Tailor It</title>
<style>
body{font-family:-apple-system,"Hiragino Sans",sans-serif;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0f0c1d;color:#fff}
main{max-width:360px;padding:32px 24px;text-align:center}
h1{font-size:24px;margin:8px 0 4px}
p{color:#c9c3e6;line-height:1.6}
a.btn{display:inline-block;margin-top:20px;padding:14px 28px;border-radius:999px;background:linear-gradient(135deg,#7b5cff,#ff5ca8);color:#fff;text-decoration:none;font-weight:600}
</style></head>
<body><main>
{{if .Found}}<p>アプリが共有されました</p>
<h1>{{.Name}}</h1>
<a class="btn" href="{{.URL}}">Tailor It で開く</a>
<p style="font-size:13px;margin-top:24px">開かない場合は Tailor It をインストールしてから、もう一度このリンクを開いてください。</p>
{{else}}<h1>リンクが無効です</h1><p>このシェアリンクは取り消されたか、存在しません。</p>{{end}}
</main></body></html>`))

// SharePage serves GET /s/{token}. Mount it outside auth.
func SharePage(s store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			Found bool
			Name  string
			URL   template.URL
		}{}
		tok := r.PathValue("token")
		if idRe.MatchString(tok) {
			if sh, err := s.GetShare(r.Context(), tok); err == nil && !sh.Revoked {
				data.Found, data.Name, data.URL = true, sh.Name, template.URL(ImportScheme+tok) // #nosec G203 -- tok matched idRe
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if !data.Found {
			w.WriteHeader(http.StatusNotFound)
		}
		_ = sharePage.Execute(w, data)
	}
}
