package appconfig

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
)

// SupportEmail receives account deletion requests (docs/legal/privacy-policy.md).
const SupportEmail = "support@tailor-it.app"

// AccountDeletePath is the web page for deleting an account outside the app
// (Google Play's account deletion URL). It sits on the share page's domain.
const AccountDeletePath = "/account/delete"

// accountDeletePageSrc explains how to delete an account: in the app, or by
// email when the app is gone. No sign-in here: the web has no Firebase Auth,
// and the API accepts only our app (App Check, #318). No script runs.
const accountDeletePageSrc = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>{{t "title"}} - Tailor It</title>
<style nonce="{{.Nonce}}">
:root{--bg:#f6f5fb;--fg:#17142a;--muted:#5d5875;--card:#fff;--line:#dedbea;--accent:#6a4cff}
@media (prefers-color-scheme:dark){:root{--bg:#0f0c1d;--fg:#f3f1ff;--muted:#a9a3c6;--card:#1b1730;--line:#332d52;--accent:#9a85ff}}
*{box-sizing:border-box}
body{margin:0;overflow-wrap:anywhere;background:var(--bg);color:var(--fg);font:16px/1.6 -apple-system,"Hiragino Sans","Noto Sans JP",sans-serif}
main{max-width:560px;margin:0 auto;padding:16px}
a{color:var(--accent)}
:focus-visible{outline:3px solid var(--accent);outline-offset:2px}
header{text-align:center;padding:24px 0 8px}
header h1{font-size:24px;margin:4px 0}
.muted{color:var(--muted)}
section{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:12px 16px;margin:0 0 16px}
section>h2{font-size:17px;margin:0 0 6px}
ul,ol{margin:0;padding-left:1.4em}
p{margin:0 0 6px}
.btn{display:block;text-align:center;padding:14px 20px;margin:10px 0 0;border-radius:999px;font-weight:600;text-decoration:none;background:var(--accent);color:#fff}
footer{font-size:13px;color:var(--muted);text-align:center;padding:8px 0 32px}
</style></head>
<body><main>
<header><p class="muted">Tailor It</p><h1>{{t "title"}}</h1><p class="muted">{{t "lead"}}</p></header>
<section aria-labelledby="app-h"><h2 id="app-h">{{t "appH"}}</h2>
<ol><li>{{t "app1"}}</li><li>{{t "app2"}}</li><li>{{t "app3"}}</li></ol></section>
<section aria-labelledby="mail-h"><h2 id="mail-h">{{t "mailH"}}</h2>
<p>{{t "mailLead"}}</p>
<ul><li>{{t "mail1"}}</li><li>{{t "mail2"}}</li></ul>
<a class="btn" href="{{.MailTo}}">{{.Email}}</a></section>
<section aria-labelledby="gone-h"><h2 id="gone-h">{{t "goneH"}}</h2>
<ul><li>{{t "gone1"}}</li><li>{{t "gone2"}}</li><li>{{t "gone3"}}</li><li>{{t "gone4"}}</li></ul></section>
<section aria-labelledby="kept-h"><h2 id="kept-h">{{t "keptH"}}</h2>
<ul><li>{{t "kept1"}}</li><li>{{t "kept2"}}</li><li>{{t "kept3"}}</li></ul></section>
<section aria-labelledby="undo-h"><h2 id="undo-h">{{t "undoH"}}</h2>
<p>{{t "undo"}}</p></section>
<footer>Tailor It · {{.Email}}</footer>
</main></body></html>`

// accountDeleteText is the page's wording. It follows the privacy policy's
// chapter 4 (docs/legal/privacy-policy.md); change both together. The keys
// must match in every language (TestAccountDeleteTextKeys).
var accountDeleteText = map[string]map[string]string{
	"ja": {
		"title":    "アカウントの削除",
		"lead":     "Tailor It のアカウントと、そのデータを削除する方法です。",
		"appH":     "アプリから削除する",
		"app1":     "Tailor It を開いて、サインインします。",
		"app2":     "設定の「アカウントを削除」を押します。",
		"app3":     "確認の画面で削除します。その場でアカウントが使えなくなります。",
		"mailH":    "アプリを使えない場合は、メールで依頼する",
		"mailLead": "アプリを消した・端末が手元にない時は、次のアドレスへ削除を依頼してください。",
		"mail1":    "件名を「アカウント削除の依頼」にして、サインインに使ったアカウントのメールアドレスから送ってください。",
		"mail2":    "本人であることを確かめるため、返信でお尋ねすることがあります。確かめられたら削除します。",
		"mailSubj": "アカウント削除の依頼",
		"goneH":    "削除されるもの",
		"gone1":    "作ったアプリ・記録・共有リンク・アカウントの情報。削除の時点で使えなくなり、30 日以内に消去します。",
		"gone2":    "通知のための端末のトークン。削除の時点で消し、通知と自動の動きを止めます。",
		"gone3":    "購入の記録・通知の受け取りの同意・送ったフィードバック。30 日以内に消去します。",
		"gone4":    "サインインのアカウント(Firebase)。",
		"keptH":    "削除されないもの",
		"kept1":    "ほかの人と共有しているアプリへの投稿は残ります。",
		"kept2":    "消去したデータも、障害に備えたバックアップに最大 14 日間残り、その後自動的に消えます。",
		"kept3":    "Tailor It Pro の購読は自動では解約されません。購入したストアの購読の管理から解約してください。",
		"undoH":    "元に戻せません",
		"undo":     "削除したアカウントとデータは元に戻せません。同じ方法でサインインしても、新しいアカウントとして始まります。",
	},
	"en": {
		"title":    "Delete your account",
		"lead":     "How to delete your Tailor It account and its data.",
		"appH":     "Delete in the app",
		"app1":     "Open Tailor It and sign in.",
		"app2":     "In Settings, tap \"Delete Account\".",
		"app3":     "Confirm. The account stops working immediately.",
		"mailH":    "Can't use the app? Ask by email",
		"mailLead": "If you removed the app or no longer have the device, ask us to delete your account at this address.",
		"mail1":    "Use the subject \"Account deletion request\" and send it from the email address of the account you signed in with.",
		"mail2":    "We may reply to confirm it is you, then delete the account.",
		"mailSubj": "Account deletion request",
		"goneH":    "What is deleted",
		"gone1":    "Your apps, records, share links and account information. They become unavailable immediately and are erased within 30 days.",
		"gone2":    "Device tokens for notifications. Deleted immediately; notifications and your apps' automations stop.",
		"gone3":    "Purchase records, notification consents and feedback you sent. Erased within 30 days.",
		"gone4":    "Your sign-in account (Firebase).",
		"keptH":    "What is not deleted",
		"kept1":    "Posts in apps shared with others remain.",
		"kept2":    "Erased data may remain in disaster-recovery backups for up to 14 more days, then expires automatically.",
		"kept3":    "A Tailor It Pro subscription is not cancelled automatically. Cancel it in the subscription settings of the store you bought it from.",
		"undoH":    "This cannot be undone",
		"undo":     "A deleted account and its data cannot be restored. Signing in again the same way starts a new account.",
	},
}

var accountDeletePages = func() map[string]*template.Template {
	out := map[string]*template.Template{}
	for lang, text := range accountDeleteText {
		t := func(key string) string { return text[key] }
		out[lang] = template.Must(template.New("accountDelete").Funcs(template.FuncMap{"t": t}).Parse(accountDeletePageSrc))
	}
	return out
}()

type accountDeletePage struct {
	Lang, Nonce, Email string
	MailTo             template.URL
}

// AccountDeletePage serves GET /account/delete. Mount it outside auth.
func AccountDeletePage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := shareLang(r.Header.Get("Accept-Language"))
		p := accountDeletePage{
			Lang:   lang,
			Nonce:  newNonce(),
			Email:  SupportEmail,
			MailTo: template.URL("mailto:" + SupportEmail + "?subject=" + url.PathEscape(accountDeleteText[lang]["mailSubj"])), // #nosec G203 -- fixed text
		}
		var buf bytes.Buffer
		if err := accountDeletePages[lang].Execute(&buf, p); err != nil {
			slog.Warn("appconfig: account delete page render failed", slog.Any("error", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", shareCSP(p.Nonce))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Vary", "Accept-Language")
		h.Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(buf.Bytes())
	}
}

// MountAccountDeletePage adds GET /account/delete to the public mux.
func MountAccountDeletePage(mux *http.ServeMux) {
	mux.Handle("GET "+AccountDeletePath, AccountDeletePage())
}
