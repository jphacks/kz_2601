package extapi

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Dropbox (API v2): the person's files' names and metadata, read only.
// Free (one `runs` a call), per-user OAuth 2.0 with PKCE (S256) and a
// refresh token (token_access_type=offline). Only files.metadata.read is
// asked for: no file content is ever read.
//
//	recent(limit?)     Rows: files in the root folder, newest first
//	search(q, limit?)  Rows: files whose name matches q
//
// limit 1-50 (default 20). Rows are names, paths, modified times and
// sizes; there is never a link (shared links are not read or made). The
// access token lives 4 hours; the refresh token is not rotated. Revoking
// (/2/auth/token/revoke) ends the whole grant.
//
// Env: EXT_DROPBOX_ENABLED=1, EXT_DROPBOX_CLIENT_ID, EXT_DROPBOX_CLIENT_SECRET
// (the app key / secret, Secret Manager). Off by default; not in the review
// build (NeedsOAuth).
const Dropbox = "dropbox"

const dropboxScopeMetadata = "files.metadata.read"

// dropboxWire is where the provider talks to; tests point it at httptest.
type dropboxWire struct {
	AuthURL   string // opened in the browser, not reached by the runner
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var dropboxWireDefault = dropboxWire{
	AuthURL:   "https://www.dropbox.com/oauth2/authorize",
	TokenURL:  "https://api.dropboxapi.com/oauth2/token",
	RevokeURL: "https://api.dropboxapi.com/2/auth/token/revoke",
	APIBase:   "https://api.dropboxapi.com/2",
}

// dropboxHosts: the token, revoke and API endpoints share one host; the
// consent page (www.dropbox.com) is the browser's only.
var dropboxHosts = []string{"https://api.dropboxapi.com"}

func init() {
	register(Provider{ID: Dropbox, Billing: BillingFree, NeedsOAuth: true, NameJa: "Dropbox", NameEn: "Dropbox",
		Ops: []OpSpec{
			{Name: "recent", Scopes: []string{dropboxScopeMetadata}, Args: []string{"limit"}, Check: checkDropboxRecent},
			{Name: "search", Scopes: []string{dropboxScopeMetadata}, Args: []string{"q", "limit"}, Check: checkDropboxSearch},
		},
		Hosts: dropboxHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newDropbox(sec, dropboxWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"recent": OperationFunc(c.recent), "search": OperationFunc(c.search)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newDropbox(sec, dropboxWireDefault) },
	})
}

type dropbox struct {
	sec  Secrets
	wire dropboxWire
	hc   *http.Client
	now  func() time.Time
}

func newDropbox(sec Secrets, w dropboxWire) (*dropbox, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_DROPBOX_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(dropboxHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &dropbox{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: PKCE S256, offline access for a refresh token, the scopes space
// separated.
func (c *dropbox) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"token_access_type":     {"offline"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *dropbox) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh renews the access token (the refresh token is kept).
func (c *dropbox) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke ends the grant (access and refresh token) with the access token.
// An expired access token is renewed first (revoking with it would end
// nothing); a refused refresh is ErrInvalidGrant. 401 with a fresh token
// is a grant already gone.
func (c *dropbox) Revoke(ctx context.Context, t Token) error {
	if t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if err != nil {
			return err
		}
		t = nt
	}
	if t.AccessToken == "" {
		return ErrNoRevoke
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.RevokeURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	resp, err := c.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusUnauthorized {
		return nil
	}
	return statusError(resp.StatusCode)
}

func (c *dropbox) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		// only a refused code / refresh token is the person's to fix; another
		// 4xx (a bad client id or secret) is ours and keeps the connection
		if e.Error == "invalid_grant" {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, statusError(resp.StatusCode)
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("dropbox: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
	if t.RefreshToken == "" {
		t.RefreshToken = prev.RefreshToken
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

// --- args ---

const (
	dropboxDefaultLimit = 20
	dropboxMaxLimit     = 50
	dropboxQueryMax     = 200
	// dropboxListLimit is how many root entries are read to pick the newest.
	dropboxListLimit = 500
)

func checkDropboxRecent(a map[string]string) error {
	_, err := argInt(a, "limit", dropboxDefaultLimit, 1, dropboxMaxLimit)
	return err
}

// dropboxQuery checks q: one line, 1-200 characters.
func dropboxQuery(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", fmt.Errorf("%w: q is required", ErrBadArgs)
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > dropboxQueryMax:
		return "", fmt.Errorf("%w: q is over %d characters", ErrBadArgs, dropboxQueryMax)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return "", fmt.Errorf("%w: q must be one line", ErrBadArgs)
	}
	return v, nil
}

func checkDropboxSearch(a map[string]string) error {
	if _, err := dropboxQuery(a["q"]); err != nil {
		return err
	}
	_, err := argInt(a, "limit", dropboxDefaultLimit, 1, dropboxMaxLimit)
	return err
}

// --- ops ---

var dropboxJST = time.FixedZone("JST", 9*60*60)

// dropboxEntry is a file or folder's metadata (the fields shown).
type dropboxEntry struct {
	Tag            string `json:".tag"`
	Name           string `json:"name"`
	PathDisplay    string `json:"path_display"`
	ServerModified string `json:"server_modified"`
	Size           int64  `json:"size"`
}

func (e dropboxEntry) modified() time.Time {
	t, err := time.Parse(time.RFC3339, e.ServerModified)
	if err != nil {
		return time.Time{}
	}
	return t
}

// dropboxSize writes bytes as the device shows them (1.2 MB).
func dropboxSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 3; m /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + " " + []string{"KB", "MB", "GB", "TB"}[exp]
}

// row: title = name, subtitle = "path · modified (JST)", value = size
// ("フォルダ" for a folder), never a link.
func (e dropboxEntry) row() Row {
	r := Row{Title: e.Name, Subtitle: e.PathDisplay}
	if e.Tag == "folder" {
		r.Value = "フォルダ"
		return r
	}
	if t := e.modified(); !t.IsZero() {
		r.Subtitle += " · " + t.In(dropboxJST).Format("2006/1/2 15:04")
	}
	r.Value = dropboxSize(e.Size)
	return r
}

// recent lists the files of the root folder (not recursive), newest first
// by server_modified, at most limit. Folders are left out (they have no
// modified time); one page of the root is read.
func (c *dropbox) recent(ctx context.Context, in Input) (Output, error) {
	n, err := argInt(in.Args, "limit", dropboxDefaultLimit, 1, dropboxMaxLimit)
	if err != nil {
		return Output{}, err
	}
	b, err := json.Marshal(map[string]any{"path": "", "recursive": false, "include_deleted": false, "include_mounted_folders": true, "limit": dropboxListLimit})
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Entries []dropboxEntry `json:"entries"`
	}
	if err := c.do(ctx, in, "/files/list_folder", b, &r); err != nil {
		return Output{}, err
	}
	files := slices.DeleteFunc(r.Entries, func(e dropboxEntry) bool { return e.Tag != "file" })
	slices.SortStableFunc(files, func(a, b dropboxEntry) int { return cmp.Compare(b.modified().Unix(), a.modified().Unix()) })
	out := Output{Rows: make([]Row, 0, min(n, len(files)))}
	for _, e := range files {
		if len(out.Rows) == n {
			break
		}
		out.Rows = append(out.Rows, e.row())
	}
	return out, nil
}

// search finds files and folders whose name matches q (filename only),
// at most limit, as Dropbox ranks them.
func (c *dropbox) search(ctx context.Context, in Input) (Output, error) {
	q, err := dropboxQuery(in.Args["q"])
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", dropboxDefaultLimit, 1, dropboxMaxLimit)
	if err != nil {
		return Output{}, err
	}
	b, err := json.Marshal(map[string]any{"query": q, "options": map[string]any{"max_results": n, "file_status": "active", "filename_only": true}})
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Matches []struct {
			Metadata struct {
				Metadata dropboxEntry `json:"metadata"`
			} `json:"metadata"`
		} `json:"matches"`
	}
	if err := c.do(ctx, in, "/files/search_v2", b, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Matches))}
	for _, m := range r.Matches {
		if e := m.Metadata.Metadata; e.Tag == "file" || e.Tag == "folder" {
			out.Rows = append(out.Rows, e.row())
		}
	}
	return out, nil
}

// do posts a JSON body to an RPC endpoint with the person's token. 401 is
// a token Dropbox no longer takes (ErrInvalidGrant: the connection turns
// to error); 403 is a missing scope or no access (ErrNotConnected: connect
// again); 400 / 409 (Dropbox's endpoint-specific error) are bad input
// (bad_request); 429 / 5xx are retryable provider errors. The body is not
// kept (it echoes the request).
func (c *dropbox) do(ctx context.Context, in Input, path string, body []byte, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("dropbox: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: dropbox refused the request", ErrUnknown)}
	}
	return statusError(resp.StatusCode)
}
