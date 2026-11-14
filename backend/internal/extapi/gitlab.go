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
)

// GitLab(gitlab.com の API v4): 無料(1 回 = `runs` 1)・OAuth 2.0(PKCE S256)・scope は
// `read_api` だけ(読むだけ。書く op は無い)。届くのは https://gitlab.com だけ(自前の
// GitLab は今は対象外)。
//
//	myIssues(limit?, state?)         自分に割り当てられた Issue(更新の新しい順)
//	myMergeRequests(limit?, state?)  自分が作った MR(更新の新しい順)
//	pipelines(projectId, limit?)     プロジェクトのパイプライン(新しい順)
//
// アクセストークンは 2 時間・リフレッシュトークンは使うたびに入れ替わる。切る時は
// POST /oauth/revoke。gitlab.com の上限は認証ありで 1 人 1 分 2,000 回で、429 は
// provider を 1 分止める(limits.go)。
//
// Env: EXT_GITLAB_ENABLED=1, EXT_GITLAB_CLIENT_ID, EXT_GITLAB_CLIENT_SECRET
// (Secret Manager)。既定 off・審査用の版に入らない(NeedsOAuth)。
const GitLab = "gitlab"

const (
	gitlabScopeRead  = "read_api"
	gitlabUserAgent  = "TailorIt/1.0 (https://tailor-it.app)"
	gitlabDefaultLim = 20
	gitlabMaxLimit   = 50
)

// gitlabWire は話す先(テストは httptest に向ける)。
type gitlabWire struct {
	AuthURL   string // ブラウザで開く(runner からは届かない)
	TokenURL  string
	RevokeURL string
	APIBase   string
	// Client は本番では nil(Hosts の NewHTTPClient)。
	Client *http.Client
	// Now は本番では nil(time.Now)。
	Now func() time.Time
}

var gitlabWireDefault = gitlabWire{
	AuthURL:   "https://gitlab.com/oauth/authorize",
	TokenURL:  "https://gitlab.com/oauth/token",
	RevokeURL: "https://gitlab.com/oauth/revoke",
	APIBase:   "https://gitlab.com/api/v4",
}

var gitlabHosts = []string{"https://gitlab.com"}

func init() {
	register(Provider{ID: GitLab, Billing: BillingFree, NeedsOAuth: true, NameJa: "GitLab", NameEn: "GitLab",
		Ops: []OpSpec{
			{Name: "myIssues", Scopes: []string{gitlabScopeRead}, Args: []string{"limit", "state"}, Check: checkGitLabIssues},
			{Name: "myMergeRequests", Scopes: []string{gitlabScopeRead}, Args: []string{"limit", "state"}, Check: checkGitLabMRs},
			{Name: "pipelines", Scopes: []string{gitlabScopeRead}, Args: []string{"projectId", "limit"}, Check: checkGitLabPipelines},
		},
		Hosts: gitlabHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newGitLab(sec, gitlabWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"myIssues": OperationFunc(c.myIssues), "myMergeRequests": OperationFunc(c.myMergeRequests),
				"pipelines": OperationFunc(c.pipelines)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newGitLab(sec, gitlabWireDefault) },
	})
}

// gitlab は op と OAuth のクライアント。
type gitlab struct {
	sec  Secrets
	wire gitlabWire
	hc   *http.Client
	now  func() time.Time
}

func newGitLab(sec Secrets, w gitlabWire) (*gitlab, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_GITLAB_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(gitlabHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &gitlab{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL: scope は空白区切り・PKCE S256。
func (c *gitlab) AuthURL(state, codeChallenge, redirectURI string, scopes []string) string {
	if len(scopes) == 0 {
		scopes = []string{gitlabScopeRead}
	}
	v := url.Values{
		"client_id":             {c.sec.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *gitlab) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh はリフレッシュトークンを引き換える(入れ替わる)。
func (c *gitlab) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke は POST /oauth/revoke(リフレッシュトークンがあればそれ、無ければアクセストークン)。
// 400(取り消し済み)・401 は済んだものとする。
func (c *gitlab) Revoke(ctx context.Context, t Token) error {
	tok := t.RefreshToken
	if tok == "" {
		tok = t.AccessToken
	}
	if tok == "" {
		return nil
	}
	form := url.Values{"token": {tok}, "client_id": {c.sec.ClientID}, "client_secret": {c.sec.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", gitlabUserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = ReadBody(resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300, resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnauthorized:
		return nil
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: errors.New("revoke refused")}
}

func (c *gitlab) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	form.Set("client_id", c.sec.ClientID)
	form.Set("client_secret", c.sec.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", gitlabUserAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "invalid_grant" {
			return Token{}, ErrInvalidGrant
		}
		return Token{}, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.AccessToken == "" {
		return Token{}, errors.New("gitlab: bad token answer")
	}
	t := Token{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, Scopes: strings.Fields(r.Scope)}
	if r.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(r.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

// --- ops ---

var (
	gitlabIssueStates = []string{"opened", "closed", "all"}
	gitlabMRStates    = []string{"opened", "merged", "closed", "locked", "all"}
	// gitlabProjectRe は数字の id か `group/project` のパス。
	gitlabProjectRe = regexp.MustCompile(`^([0-9]{1,12}|[A-Za-z0-9_.-]{1,100}(/[A-Za-z0-9_.-]{1,100}){1,5})$`)
)

// gitlabState は state(既定 opened)。
func gitlabState(v string, allowed []string) (string, error) {
	if v == "" {
		return "opened", nil
	}
	for _, s := range allowed {
		if s == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: state must be one of %s", ErrBadArgs, strings.Join(allowed, ", "))
}

func checkGitLabIssues(a map[string]string) error {
	if _, err := gitlabState(a["state"], gitlabIssueStates); err != nil {
		return err
	}
	_, err := argInt(a, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	return err
}

func checkGitLabMRs(a map[string]string) error {
	if _, err := gitlabState(a["state"], gitlabMRStates); err != nil {
		return err
	}
	_, err := argInt(a, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	return err
}

func checkGitLabPipelines(a map[string]string) error {
	if !gitlabProjectRe.MatchString(strings.TrimSpace(a["projectId"])) || strings.Contains(a["projectId"], "..") {
		return fmt.Errorf("%w: projectId must be a project id (digits) or a path like group/project", ErrBadArgs)
	}
	_, err := argInt(a, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	return err
}

// gitlabIssue は Issue / MR の JSON のうち行にする分。
type gitlabIssue struct {
	Title      string `json:"title"`
	State      string `json:"state"`
	DueDate    string `json:"due_date"`
	UpdatedAt  string `json:"updated_at"`
	WebURL     string `json:"web_url"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
}

// row: 見出し = 題名・補足 = 「group/proj#12 · 状態」・値 = value・リンク。
func (i gitlabIssue) row(value string) Row {
	r := Row{Title: i.Title, Subtitle: strings.TrimPrefix(i.References.Full+" · "+i.State, " · "), Value: value}
	r.Subtitle = strings.TrimSuffix(r.Subtitle, " · ")
	if strings.HasPrefix(i.WebURL, "https://gitlab.com/") {
		r.URL = i.WebURL
	}
	return r
}

// myIssues は自分に割り当てられた Issue(値 = 期限)。
func (c *gitlab) myIssues(ctx context.Context, in Input) (Output, error) {
	state, err := gitlabState(in.Args["state"], gitlabIssueStates)
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"scope": {"assigned_to_me"}, "state": {state}, "order_by": {"updated_at"}, "sort": {"desc"}, "per_page": {strconv.Itoa(n)}}
	var items []gitlabIssue
	if err := c.get(ctx, in, "/issues?"+v.Encode(), &items); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(items))}
	for _, it := range items {
		out.Rows = append(out.Rows, it.row(it.DueDate))
	}
	return out, nil
}

// myMergeRequests は自分が作った MR(値 = 更新日時)。
func (c *gitlab) myMergeRequests(ctx context.Context, in Input) (Output, error) {
	state, err := gitlabState(in.Args["state"], gitlabMRStates)
	if err != nil {
		return Output{}, err
	}
	n, err := argInt(in.Args, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"scope": {"created_by_me"}, "state": {state}, "order_by": {"updated_at"}, "sort": {"desc"}, "per_page": {strconv.Itoa(n)}}
	var items []gitlabIssue
	if err := c.get(ctx, in, "/merge_requests?"+v.Encode(), &items); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(items))}
	for _, it := range items {
		out.Rows = append(out.Rows, it.row(it.UpdatedAt))
	}
	return out, nil
}

// pipelines はプロジェクトのパイプライン(見出し = ref・補足 = 状態 · 作成日時・値 = id・リンク)。
// 無いプロジェクト(404)は 400。
func (c *gitlab) pipelines(ctx context.Context, in Input) (Output, error) {
	if err := checkGitLabPipelines(in.Args); err != nil {
		return Output{}, err
	}
	n, _ := argInt(in.Args, "limit", gitlabDefaultLim, 1, gitlabMaxLimit)
	id := url.PathEscape(strings.TrimSpace(in.Args["projectId"])) // `/` は %2F になる
	v := url.Values{"order_by": {"id"}, "sort": {"desc"}, "per_page": {strconv.Itoa(n)}}
	var items []struct {
		ID        int64  `json:"id"`
		Ref       string `json:"ref"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		WebURL    string `json:"web_url"`
	}
	if err := c.get(ctx, in, "/projects/"+id+"/pipelines?"+v.Encode(), &items); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(items))}
	for _, it := range items {
		r := Row{Title: it.Ref, Subtitle: strings.TrimSuffix(it.Status+" · "+it.CreatedAt, " · "), Value: strconv.FormatInt(it.ID, 10)}
		if strings.HasPrefix(it.WebURL, "https://gitlab.com/") {
			r.URL = it.WebURL
		}
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}

// --- the wire ---

// get は本人のトークンで API v4 を呼ぶ。401 はもう使えないトークン(ErrInvalidGrant・接続は
// error)、403 は scope / 権限不足(ErrNotConnected・つなぎ直し)、404 は 400、429 はやり直し可の
// 429(provider を止める)、5xx はやり直し可。
func (c *gitlab) get(ctx context.Context, in Input, path string, out any) error {
	if in.Token == nil || in.Token.AccessToken == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.wire.APIBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", gitlabUserAgent)
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
			return &ProviderError{Status: resp.StatusCode, Err: errors.New("gitlab answered an unexpected shape")}
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusTooManyRequests:
		return &ProviderError{Status: http.StatusTooManyRequests, Retryable: true, Err: errors.New("gitlab rate limited")}
	case resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: gitlab has no such project", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500, Err: fmt.Errorf("gitlab status %d", resp.StatusCode)}
}
