package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Google Drive (Drive API v3): free (one `runs` a call), OAuth with PKCE,
// read only.
//
//	recent(limit?)     drive.file
//	search(q, limit?)  drive.file
//
// The scope is drive.file (not sensitive): only the files the person picked
// in the Google Picker or the app made, as googleSheets. drive.readonly and
// drive.metadata.readonly would show every file, but they are restricted
// scopes (a yearly CASA assessment), which docs/decisions/external-apis.md
// 「審査・規約」rules out; gdriveScope is the one place to change after
// that. File contents are never read; nothing is cached. The OAuth client is
// Google Calendar's unless EXT_GOOGLE_DRIVE_CLIENT_ID / _CLIENT_SECRET are
// set (ClientFrom: one consent screen).
//
// Env: EXT_GOOGLE_DRIVE_ENABLED=1. Off by default; not in the review build
// (NeedsOAuth).
const GoogleDrive = "googleDrive"

const gdriveScope = "https://www.googleapis.com/auth/drive.file"

var googleDriveWire = gcalWire{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	RevokeURL: "https://oauth2.googleapis.com/revoke",
	APIBase:   "https://www.googleapis.com/drive/v3",
}

var googleDriveHosts = []string{"https://oauth2.googleapis.com", "https://www.googleapis.com"}

func init() {
	register(Provider{ID: GoogleDrive, Billing: BillingFree, NeedsOAuth: true, NameJa: "Google ドライブ", NameEn: "Google Drive",
		Ops: []OpSpec{
			{Name: "recent", Scopes: []string{gdriveScope}, Args: []string{"limit"}, Check: checkGdriveRecent},
			{Name: "search", Scopes: []string{gdriveScope}, Args: []string{"q", "limit"}, Check: checkGdriveSearch},
		},
		Hosts:      googleDriveHosts,
		ClientFrom: GoogleCalendar,
		New: func(sec Secrets) (map[string]Operation, error) {
			d, err := newGoogleDrive(sec, googleDriveWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"recent": OperationFunc(d.recent), "search": OperationFunc(d.search)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			d, err := newGoogleDrive(sec, googleDriveWire)
			if err != nil {
				return nil, err
			}
			return d.g, nil
		},
	})
}

// googleDrive is the provider's ops; the OAuth client and do are Google
// Calendar's code on the Drive wire.
type googleDrive struct {
	g *googleCalendar
}

func newGoogleDrive(sec Secrets, w gcalWire) (*googleDrive, error) {
	g, err := newGoogleClient(GoogleDrive, sec, w, googleDriveHosts)
	if err != nil {
		return nil, err
	}
	return &googleDrive{g: g}, nil
}

// --- arguments ---

const (
	gdriveDefaultLimit = 20
	gdriveMaxLimit     = 50
	gdriveQueryMax     = 256
	gdriveNoName       = "(名前なし)"
	gdriveFields       = "files(id,name,mimeType,modifiedTime,size,webViewLink)"
)

func gdriveLimit(v string) (int, error) {
	if v == "" {
		return gdriveDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > gdriveMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrBadArgs, gdriveMaxLimit)
	}
	return x, nil
}

// gdriveQuery checks search's q (1-256 runes, one line).
func gdriveQuery(v string) (string, error) {
	q, err := gcalText("q", v, gdriveQueryMax, true)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(q, "\n\t") {
		return "", fmt.Errorf("%w: q must be one line", ErrBadArgs)
	}
	return q, nil
}

// gdriveQuote escapes a value for a Drive query string literal.
func gdriveQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func checkGdriveRecent(a map[string]string) error {
	_, err := gdriveLimit(a["limit"])
	return err
}

func checkGdriveSearch(a map[string]string) error {
	if _, err := gdriveQuery(a["q"]); err != nil {
		return err
	}
	_, err := gdriveLimit(a["limit"])
	return err
}

// gdriveCanRead: the granted scopes cover the op (unknown scopes pass;
// Google then answers 403 and the person connects again).
func gdriveCanRead(t *Token) bool {
	if t == nil || len(t.Scopes) == 0 {
		return true
	}
	ok := []string{gdriveScope, "https://www.googleapis.com/auth/drive", "https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/drive.metadata.readonly", "https://www.googleapis.com/auth/drive.metadata"}
	return slices.ContainsFunc(t.Scopes, func(s string) bool { return slices.Contains(ok, s) })
}

// --- answers ---

type gdriveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	ModifiedTime string `json:"modifiedTime"`
	Size         string `json:"size"`
	WebViewLink  string `json:"webViewLink"`
}

// gdriveKinds are the Japanese names of Google's own file types.
var gdriveKinds = map[string]string{
	"application/vnd.google-apps.document":     "ドキュメント",
	"application/vnd.google-apps.spreadsheet":  "スプレッドシート",
	"application/vnd.google-apps.presentation": "スライド",
	"application/vnd.google-apps.folder":       "フォルダ",
	"application/vnd.google-apps.form":         "フォーム",
	"application/pdf":                          "PDF",
}

// gdriveKind is the file type's short name from its MIME type.
func gdriveKind(mime string) string {
	if k, ok := gdriveKinds[mime]; ok {
		return k
	}
	major, sub, _ := strings.Cut(mime, "/")
	switch major {
	case "image":
		return "画像"
	case "video":
		return "動画"
	case "audio":
		return "音声"
	case "text":
		return "テキスト"
	}
	return sub
}

// gdriveSize humanizes a size in bytes (Google's own files have none).
func gdriveSize(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return ""
	}
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64) + " GB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatInt(n/(1<<10), 10) + " KB"
	}
	return strconv.FormatInt(n, 10) + " B"
}

// row: the name, "modified (JST)・kind", the size, the file on Drive.
func (f gdriveFile) row() Row {
	r := Row{Title: strings.TrimSpace(f.Name), Value: gdriveSize(f.Size)}
	if r.Title == "" {
		r.Title = gdriveNoName
	}
	var parts []string
	if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
		parts = append(parts, t.In(gcalJST).Format("2006-01-02 15:04"))
	}
	if k := gdriveKind(f.MimeType); k != "" {
		parts = append(parts, k)
	}
	r.Subtitle = strings.Join(parts, "・")
	if u, err := url.Parse(f.WebViewLink); err == nil && u.Scheme == "https" && (u.Host == "drive.google.com" || u.Host == "docs.google.com") {
		r.URL = f.WebViewLink
	}
	return r
}

// --- ops ---

// recent lists the files the app can see, newest change first, one page of
// limit (default 20); trashed files are left out.
func (d *googleDrive) recent(ctx context.Context, in Input) (Output, error) {
	n, err := gdriveLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	return d.list(ctx, in.Token, "trashed = false", n)
}

// search lists the files whose name or text has q, newest change first.
func (d *googleDrive) search(ctx context.Context, in Input) (Output, error) {
	q, err := gdriveQuery(in.Args["q"])
	if err != nil {
		return Output{}, err
	}
	n, err := gdriveLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	lit := gdriveQuote(q)
	return d.list(ctx, in.Token, "(name contains "+lit+" or fullText contains "+lit+") and trashed = false", n)
}

func (d *googleDrive) list(ctx context.Context, tok *Token, query string, n int) (Output, error) {
	if !gdriveCanRead(tok) {
		return Output{}, ErrNotConnected
	}
	v := url.Values{
		"q":        {query},
		"orderBy":  {"modifiedTime desc"},
		"pageSize": {strconv.Itoa(n)},
		"spaces":   {"drive"},
		"fields":   {gdriveFields},
	}
	var r struct {
		Files []gdriveFile `json:"files"`
	}
	if err := d.do(ctx, tok, "/files?"+v.Encode(), &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Files))}
	for _, f := range r.Files {
		out.Rows = append(out.Rows, f.row())
	}
	return out, nil
}

// do is Google Calendar's do (401 → ErrInvalidGrant, a missing scope →
// ErrNotConnected, 429 / 5xx / quota 403 → retryable, an API not enabled →
// upstream_unavailable) plus: 400 and 404 are a query Drive refused
// (bad_request: connecting again would not help).
func (d *googleDrive) do(ctx context.Context, tok *Token, path string, out any) error {
	err := d.g.do(ctx, tok, http.MethodGet, path, nil, "", out)
	var pe *ProviderError
	if errors.As(err, &pe) && !pe.Retryable && (pe.Status == http.StatusBadRequest || pe.Status == http.StatusNotFound) {
		return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googleDrive refused the query", ErrUnknown)}
	}
	return err
}
