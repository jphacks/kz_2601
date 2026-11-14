package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Backlog(Nulab・API v2): 無料(1 回 = `runs` 1)・OAuth 2.0。Backlog は**スペースごとに
// ホストが違う**(`https://<space>.backlog.com` / `.backlog.jp`)ので、つなぐ時に
// `space`(例 `acme.backlog.jp`)を受け取り(Provider.ConnectArgs / ConnectHost)、
// トークンと一緒にホストを持つ(Token.Host)。認可・トークン・API は全部そのホスト。
// OAuth のクライアント(client_id / secret)は Nulab のアプリ登録で 1 つ(どのスペースでも同じ)。
//
//	myIssues(limit?, statusId?)                                              自分が担当の課題
//	projects(limit?)                                                         参加しているプロジェクト
//	addIssue(projectId, summary, issueTypeId, priorityId?, description?, dueDate?)  課題を足す(書く・確認必須)
//
// Backlog の OAuth に scope と PKCE は無い(認可の URL に code_challenge を付けない。state で
// 守る)。アクセストークンは 1 時間・リフレッシュトークンは入れ替わる。取り消しの口は無い
// (ErrNoRevoke: 保存したトークンを消すだけ)。上限は利用者 1 人 1 分 150 回(API の種類ごと)で、
// 429 は provider を 1 分止める。
//
// Env: EXT_BACKLOG_ENABLED=1, EXT_BACKLOG_CLIENT_ID, EXT_BACKLOG_CLIENT_SECRET
// (Secret Manager)。既定 off・審査用の版に入らない(NeedsOAuth)。
const Backlog = "backlog"

const (
	backlogUserAgent    = "TailorIt/1.0 (https://tailor-it.app)"
	backlogDefaultLimit = 20
	backlogMaxLimit     = 100
	backlogMaxSummary   = 255
	backlogMaxDesc      = 100000
)

// backlogWire は話す先。Base はホスト(`https://acme.backlog.jp`)を実際の URL に写す
// (本番は同じもの・テストは httptest に向ける)。
type backlogWire struct {
	Base func(host string) string
	// Client は本番では nil(Hosts の NewHTTPClient)。
	Client *http.Client
	// Now は本番では nil(time.Now)。
	Now func() time.Time
}

var backlogWireDefault = backlogWire{Base: func(host string) string { return host }}

// backlogHosts: スペースごとのホストなので、1 ラベル + ドメインの形(AllowedURL)。
var backlogHosts = []string{"https://*.backlog.com", "https://*.backlog.jp"}

var (
	backlogSpaceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}\.backlog\.(com|jp)$`)
	backlogIDRe    = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)
)

func init() {
	register(Provider{ID: Backlog, Billing: BillingFree, NeedsOAuth: true, NameJa: "Backlog", NameEn: "Backlog",
		Ops: []OpSpec{
			{Name: "myIssues", Args: []string{"limit", "statusId"}, Check: checkBacklogMyIssues},
			{Name: "projects", Args: []string{"limit"}, Check: checkBacklogProjects},
			{Name: "addIssue", Write: true, Args: []string{"projectId", "summary", "issueTypeId", "priorityId", "description", "dueDate"}, Check: checkBacklogAdd},
		},
		Hosts:       backlogHosts,
		ConnectArgs: []string{"space"},
		ConnectHost: backlogHost,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newBacklog(sec, backlogWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"myIssues": OperationFunc(c.myIssues), "projects": OperationFunc(c.projects), "addIssue": OperationFunc(c.addIssue)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newBacklog(sec, backlogWireDefault) },
	})
}

// backlogHost は `space`(`acme.backlog.jp` / `acme.backlog.com`)を https のホストにする。
func backlogHost(args map[string]string) (string, error) {
	sp := strings.ToLower(strings.TrimSpace(args["space"]))
	sp = strings.TrimPrefix(strings.TrimPrefix(sp, "https://"), "http://")
	if !backlogSpaceRe.MatchString(sp) {
		return "", errors.New("space must be like acme.backlog.jp or acme.backlog.com")
	}
	return "https://" + sp, nil
}

// backlog は op と OAuth のクライアント。
type backlog struct {
	sec  Secrets
	wire backlogWire
	hc   *http.Client
	now  func() time.Time
}

func newBacklog(sec Secrets, w backlogWire) (*backlog, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_BACKLOG_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(backlogHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	if w.Base == nil {
		w.Base = backlogWireDefault.Base
	}
	return &backlog{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL はホストなしでは作れない(Start は AuthURLOn を使う)。
func (c *backlog) AuthURL(string, string, string, []string) string { return "" }

// Exchange はホストなしでは引き換えられない(callback は ExchangeOn を使う)。
func (c *backlog) Exchange(context.Context, string, string, string) (Token, error) {
	return Token{}, errors.New("backlog: space is required")
}

// AuthURLOn は本人のスペースの同意ページ(scope・PKCE は無い)。
func (c *backlog) AuthURLOn(host, state, _, redirectURI string, _ []string) string {
	v := url.Values{"response_type": {"code"}, "client_id": {c.sec.ClientID}, "redirect_uri": {redirectURI}, "state": {state}}
	return c.wire.Base(host) + "/OAuth2AccessRequest.action?" + v.Encode()
}

func (c *backlog) ExchangeOn(ctx context.Context, host, code, _, redirectURI string) (Token, error) {
	t, err := c.token(ctx, host, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}})
	if err != nil {
		return Token{}, err
	}
	t.Host = host
	return t, nil
}

// Refresh はリフレッシュトークンを引き換える(入れ替わる)。ホストの無いトークンはつなぎ直し。
func (c *backlog) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" || t.Host == "" {
		return Token{}, ErrInvalidGrant
	}
	nt, err := c.token(ctx, t.Host, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}})
	if err != nil {
		return Token{}, err
	}
	nt.Host = t.Host
	return nt, nil
}

// Revoke: Backlog に取り消しの口は無い。
func (c *backlog) Revoke(context.Context, Token) error { return ErrNoRevoke }

func (c *backlog) token(ctx context.Context, host string, form url.Values) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.Base(host)+"/api/v2/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", backlogUserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return Token{}, ctx.Err()
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("reading the token answer failed")}
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "invalid_grant" || resp.StatusCode == http.StatusUnauthorized {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("backlog: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	return t, nil
}

// --- ops ---

// backlogID は数字の id(省略可の時は空を通す)。
func backlogID(a map[string]string, name string, required bool) (string, error) {
	v := strings.TrimSpace(a[name])
	if v == "" {
		if required {
			return "", fmt.Errorf("%w: %s is required", ErrBadArgs, name)
		}
		return "", nil
	}
	if !backlogIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: %s must be a Backlog id (digits)", ErrBadArgs, name)
	}
	return v, nil
}

func checkBacklogMyIssues(a map[string]string) error {
	if _, err := backlogID(a, "statusId", false); err != nil {
		return err
	}
	_, err := argInt(a, "limit", backlogDefaultLimit, 1, backlogMaxLimit)
	return err
}

func checkBacklogProjects(a map[string]string) error {
	_, err := argInt(a, "limit", backlogDefaultLimit, 1, backlogMaxLimit)
	return err
}

// backlogText は文字数と(1 行なら)改行の検査。
func backlogText(a map[string]string, name string, max int, oneLine bool) (string, error) {
	t := strings.TrimSpace(a[name])
	if len([]rune(t)) > max {
		return "", fmt.Errorf("%w: %s takes at most %d characters", ErrBadArgs, name, max)
	}
	if strings.IndexFunc(t, func(r rune) bool { return unicode.IsControl(r) && (oneLine || (r != '\n' && r != '\t')) }) >= 0 {
		return "", fmt.Errorf("%w: %s has control characters", ErrBadArgs, name)
	}
	return t, nil
}

type backlogAddArgs struct {
	projectID, summary, issueTypeID, priorityID, description, dueDate string
}

func backlogAddParse(a map[string]string) (backlogAddArgs, error) {
	var out backlogAddArgs
	var err error
	if out.projectID, err = backlogID(a, "projectId", true); err != nil {
		return out, err
	}
	if out.issueTypeID, err = backlogID(a, "issueTypeId", true); err != nil {
		return out, err
	}
	if out.priorityID, err = backlogID(a, "priorityId", false); err != nil {
		return out, err
	}
	if out.summary, err = backlogText(a, "summary", backlogMaxSummary, true); err != nil {
		return out, err
	}
	if out.summary == "" {
		return out, fmt.Errorf("%w: summary is required", ErrBadArgs)
	}
	if out.description, err = backlogText(a, "description", backlogMaxDesc, false); err != nil {
		return out, err
	}
	if d := strings.TrimSpace(a["dueDate"]); d != "" {
		if _, perr := time.Parse(time.DateOnly, d); perr != nil || !dateRe.MatchString(d) {
			return out, fmt.Errorf("%w: dueDate must be YYYY-MM-DD", ErrBadArgs)
		}
		out.dueDate = d
	}
	return out, nil
}

func checkBacklogAdd(a map[string]string) error {
	_, err := backlogAddParse(a)
	return err
}

// backlogIssue は課題の JSON のうち行にする分。
type backlogIssue struct {
	ID       int64  `json:"id"`
	IssueKey string `json:"issueKey"`
	Summary  string `json:"summary"`
	DueDate  string `json:"dueDate"`
	Status   *struct {
		Name string `json:"name"`
	} `json:"status"`
}

// row: 見出し = 件名・補足 = 「プロジェクトのキー · 期限 · 状態」・値 = 課題のキー・リンク = /view/KEY。
func (i backlogIssue) row(host string) Row {
	project, _, _ := strings.Cut(i.IssueKey, "-")
	due, _, _ := strings.Cut(i.DueDate, "T")
	parts := []string{project, due}
	if i.Status != nil {
		parts = append(parts, i.Status.Name)
	}
	var sub []string
	for _, p := range parts {
		if p != "" {
			sub = append(sub, p)
		}
	}
	r := Row{Title: i.Summary, Subtitle: strings.Join(sub, " · "), Value: i.IssueKey}
	if i.IssueKey != "" && host != "" {
		r.URL = host + "/view/" + url.PathEscape(i.IssueKey)
	}
	return r
}

// myIssues は自分が担当の課題(更新の新しい順)。statusId で状態を絞る。
func (c *backlog) myIssues(ctx context.Context, in Input) (Output, error) {
	status, err := backlogID(in.Args, "statusId", false)
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", backlogDefaultLimit, 1, backlogMaxLimit)
	if err != nil {
		return Output{}, err
	}
	var me struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/api/v2/users/myself", nil, &me); err != nil {
		return Output{}, err
	}
	v := url.Values{"assigneeId[]": {strconv.FormatInt(me.ID, 10)}, "count": {strconv.Itoa(n)}, "sort": {"updated"}, "order": {"desc"}}
	if status != "" {
		v.Set("statusId[]", status)
	}
	var items []backlogIssue
	if err := c.do(ctx, in, http.MethodGet, "/api/v2/issues?"+v.Encode(), nil, &items); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(items))}
	for _, it := range items {
		out.Rows = append(out.Rows, it.row(in.Token.Host))
	}
	return out, nil
}

// projects は参加しているプロジェクト(見出し = 名前・補足 = キー・値 = id・リンク)。
func (c *backlog) projects(ctx context.Context, in Input) (Output, error) {
	n, err := argInt(in.Args, "limit", backlogDefaultLimit, 1, backlogMaxLimit)
	if err != nil {
		return Output{}, err
	}
	var items []struct {
		ID         int64  `json:"id"`
		ProjectKey string `json:"projectKey"`
		Name       string `json:"name"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/api/v2/projects?archived=false", nil, &items); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, min(n, len(items)))}
	for _, it := range items {
		if len(out.Rows) == n {
			break
		}
		r := Row{Title: it.Name, Subtitle: it.ProjectKey, Value: strconv.FormatInt(it.ID, 10)}
		if it.ProjectKey != "" {
			r.URL = in.Token.Host + "/projects/" + url.PathEscape(it.ProjectKey)
		}
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}

// addIssue は課題を 1 つ足す。priorityId が無い時はスペースの優先度から「中」(無ければ真ん中)を
// 使う(Backlog では必須)。答えは resultId(課題のキー)と 1 件。
func (c *backlog) addIssue(ctx context.Context, in Input) (Output, error) {
	a, err := backlogAddParse(in.Args)
	if err != nil {
		return Output{}, err
	}
	if a.priorityID == "" {
		p, err := c.defaultPriority(ctx, in)
		if err != nil {
			return Output{}, err
		}
		a.priorityID = p
	}
	form := url.Values{"projectId": {a.projectID}, "summary": {a.summary}, "issueTypeId": {a.issueTypeID}, "priorityId": {a.priorityID}}
	if a.description != "" {
		form.Set("description", a.description)
	}
	if a.dueDate != "" {
		form.Set("dueDate", a.dueDate)
	}
	var is backlogIssue
	if err := c.do(ctx, in, http.MethodPost, "/api/v2/issues", form, &is); err != nil {
		return Output{}, err
	}
	if is.IssueKey == "" {
		return Output{}, &ProviderError{Status: http.StatusBadGateway, Err: errors.New("backlog: no issue key in the answer")}
	}
	row := is.row(in.Token.Host)
	return Output{ResultID: is.IssueKey, Item: &row}, nil
}

// defaultPriority は「中」(名前が 中 / Normal)、無ければ一覧の真ん中。
func (c *backlog) defaultPriority(ctx context.Context, in Input) (string, error) {
	var ps []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, in, http.MethodGet, "/api/v2/priorities", nil, &ps); err != nil {
		return "", err
	}
	if len(ps) == 0 {
		return "", &ProviderError{Status: http.StatusBadGateway, Err: errors.New("backlog: no priorities")}
	}
	for _, p := range ps {
		if p.Name == "中" || strings.EqualFold(p.Name, "normal") {
			return strconv.FormatInt(p.ID, 10), nil
		}
	}
	return strconv.FormatInt(ps[len(ps)/2].ID, 10), nil
}

// --- the wire ---

// do は本人のトークンでスペースの API を呼ぶ。401 はもう使えないトークン(ErrInvalidGrant・
// 接続は error)、403 は権限不足(ErrNotConnected・つなぎ直し)、404 / 400 は 400(無い id・
// 受けない入力)、429 はやり直し可の 429(provider を止める)、5xx はやり直し可。
func (c *backlog) do(ctx context.Context, in Input, method, path string, form url.Values, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" || in.Token.Host == "" {
		return ErrNotConnected
	}
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wire.Base(in.Token.Host)+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", backlogUserAgent)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
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
			return &ProviderError{Status: resp.StatusCode, Err: errors.New("backlog answered an unexpected shape")}
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusTooManyRequests:
		return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("backlog rate limited")}
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusBadRequest:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: backlog refused the input", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: fmt.Errorf("backlog status %d", resp.StatusCode)}
}
