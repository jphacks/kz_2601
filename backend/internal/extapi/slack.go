package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Slack (#1679, Web API): free (one `runs` a call), OAuth v2 with a user
// token (xoxp-): the app acts as the person, so a status or a message is
// theirs. Only user scopes are asked for (user_scope); no bot token.
//
//	setStatus(text?, emoji?, minutes?)   users.profile:write   write
//	postMessage(channel, text)           chat:write            write
//	unread(channel)                      im:read
//
// unread reads one direct message's unread count (conversations.info gives
// unread counts for DMs only; counting a channel would mean reading its
// messages with conversations.history, Tier 1 for non-Marketplace apps).
// No op reads or keeps a message's text.
//
// No PKCE: Slack's PKCE turns the app into a public client for good, and the
// runner is a confidential client (Basic auth); the one-use state binds the
// callback. Slack adds new scopes to the person's token, so connecting again
// for a write keeps the reads. Tokens do not expire unless the app turns on
// token rotation (12 hours and a one-use refresh token); both are handled.
//
// Env: EXT_SLACK_ENABLED=1, EXT_SLACK_CLIENT_ID, EXT_SLACK_CLIENT_SECRET
// (Secret Manager). Off by default; not in the review build (NeedsOAuth).
// Slack's API terms: commercial distribution needs the Marketplace (or a
// separate agreement), data is kept only as long as a feature needs it and
// never used to train models, and a privacy policy must say what is read.
const Slack = "slack"

const (
	slackScopeStatus = "users.profile:write"
	slackScopePost   = "chat:write"
	slackScopeIM     = "im:read"
)

// slackWire is where the provider talks to; tests point it at httptest.
type slackWire struct {
	AuthURL string // opened in the browser, not reached by the runner
	APIBase string // https://slack.com/api: oauth.v2.access, auth.revoke and the methods
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var slackWireDefault = slackWire{
	AuthURL: "https://slack.com/oauth/v2/authorize",
	APIBase: "https://slack.com/api",
}

var slackHosts = []string{"https://slack.com"}

func init() {
	register(Provider{ID: Slack, Billing: BillingFree, NeedsOAuth: true, NameJa: "Slack", NameEn: "Slack",
		Ops: []OpSpec{
			{Name: "setStatus", Write: true, Scopes: []string{slackScopeStatus}, Args: []string{"text", "emoji", "minutes"}, Check: checkSlackStatus},
			{Name: "postMessage", Write: true, Scopes: []string{slackScopePost}, Args: []string{"channel", "text"}, Check: checkSlackPost},
			{Name: "unread", Scopes: []string{slackScopeIM}, Args: []string{"channel"}, Check: checkSlackUnread},
		},
		Hosts: slackHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			c, err := newSlack(sec, slackWireDefault)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"setStatus": OperationFunc(c.setStatus), "postMessage": OperationFunc(c.postMessage),
				"unread": OperationFunc(c.unread)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newSlack(sec, slackWireDefault) },
	})
}

// slack is the provider's ops and OAuth client.
type slack struct {
	sec  Secrets
	wire slackWire
	hc   *http.Client
	now  func() time.Time
}

func newSlack(sec Secrets, w slackWire) (*slack, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_SLACK_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(slackHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &slack{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth ---

// AuthURL asks for user scopes only (comma separated, user_scope); no
// code challenge (see the package comment above).
func (c *slack) AuthURL(state, _, redirectURI string, scopes []string) string {
	v := url.Values{
		"client_id":    {c.sec.ClientID},
		"redirect_uri": {redirectURI},
		"user_scope":   {strings.Join(scopes, ",")},
		"state":        {state},
	}
	return c.wire.AuthURL + "?" + v.Encode()
}

func (c *slack) Exchange(ctx context.Context, code, _, redirectURI string) (Token, error) {
	return c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI}}, Token{})
}

// Refresh: only a rotating token has a refresh token (one use; a new one
// comes back). A token without one does not expire.
func (c *slack) Refresh(ctx context.Context, t Token) (Token, error) {
	if t.RefreshToken == "" {
		return Token{}, ErrInvalidGrant
	}
	return c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}}, t)
}

// Revoke revokes the user token (auth.revoke). An expired rotating token is
// renewed first; a token Slack already refuses is done.
func (c *slack) Revoke(ctx context.Context, t Token) error {
	if t.expired(c.now()) && t.RefreshToken != "" {
		nt, err := c.Refresh(ctx, t)
		if errors.Is(err, ErrInvalidGrant) {
			return nil
		}
		if err != nil {
			return err
		}
		t = nt
	}
	if t.AccessToken == "" {
		return nil
	}
	err := c.do(ctx, &t, http.MethodPost, "/auth.revoke", nil, nil)
	if errors.Is(err, ErrInvalidGrant) {
		return nil
	}
	return err
}

func (c *slack) token(ctx context.Context, form url.Values, prev Token) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wire.APIBase+"/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.SetBasicAuth(c.sec.ClientID, c.sec.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return Token{}, err
	}
	type userToken struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		userToken
		AuthedUser *userToken `json:"authed_user"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &r) != nil || !r.OK {
		switch r.Error {
		case "invalid_code", "invalid_refresh_token", "invalid_grant", "token_revoked", "token_expired", "invalid_auth", "account_inactive":
			return Token{}, ErrInvalidGrant
		}
		status := resp.StatusCode
		if status == http.StatusOK {
			status = http.StatusBadRequest
		}
		return Token{}, &ProviderError{Status: status, Retryable: status == 429 || status >= 500,
			Err: fmt.Errorf("token endpoint refused: %s", slackCode(r.Error))}
	}
	// the code exchange puts the user token in authed_user; a refresh of a
	// user token answers it at the top level (token_type user)
	u := r.AuthedUser
	if u == nil || u.AccessToken == "" {
		if r.TokenType != "user" {
			return Token{}, errors.New("slack: no user token in the answer")
		}
		u = &r.userToken
	}
	if u.AccessToken == "" {
		return Token{}, errors.New("slack: bad token answer")
	}
	t := Token{AccessToken: u.AccessToken, RefreshToken: u.RefreshToken, Scopes: slackSplitScopes(u.Scope)}
	if u.ExpiresIn > 0 && u.RefreshToken != "" {
		t.Expiry = c.now().Add(time.Duration(u.ExpiresIn) * time.Second).UTC()
	}
	if len(t.Scopes) == 0 {
		t.Scopes = prev.Scopes
	}
	return t, nil
}

func slackSplitScopes(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// slackHas: the granted scopes cover scope (unknown scopes pass; Slack then
// answers missing_scope and the person connects again).
func slackHas(t *Token, scope string) bool {
	return t == nil || len(t.Scopes) == 0 || slices.Contains(t.Scopes, scope)
}

// --- ops ---

const (
	slackMaxStatus  = 100  // Slack's status text
	slackMaxText    = 4000 // Slack's advice for one message
	slackMaxMinutes = 7 * 24 * 60
)

var (
	// a conversation id: C (channel), G (private / group DM), D (DM)
	slackChannelRe = regexp.MustCompile(`^[CGD][A-Z0-9]{8,20}$`)
	slackDMRe      = regexp.MustCompile(`^D[A-Z0-9]{8,20}$`)
	// :name: with an optional skin tone (no spaces or colons inside)
	slackEmojiRe = regexp.MustCompile(`^:[^:\s]{1,64}:(:skin-tone-[2-6]:)?$`)
	slackCodeRe  = regexp.MustCompile(`^[a-z_]{1,64}$`)
)

// slackText checks a free-text argument: length (runes) and control chars
// (line breaks only when lines).
func slackText(name, v string, max int, lines bool) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > max:
		return "", fmt.Errorf("%w: %s is over %d characters", ErrUnknown, name, max)
	case strings.ContainsFunc(v, func(r rune) bool { return r == 0x7f || r < 0x20 && (!lines || r != '\n' && r != '\t') }):
		return "", fmt.Errorf("%w: %s has control characters", ErrUnknown, name)
	}
	return v, nil
}

type slackStatus struct {
	text, emoji string
	minutes     int
}

// slackStatusArgs: text and emoji both empty clears the status (then no
// minutes); minutes 1 to a week (none = until changed).
func slackStatusArgs(a map[string]string) (slackStatus, error) {
	text, err := slackText("text", a["text"], slackMaxStatus, false)
	if err != nil {
		return slackStatus{}, err
	}
	emoji := strings.TrimSpace(a["emoji"])
	if emoji != "" && !slackEmojiRe.MatchString(emoji) {
		return slackStatus{}, fmt.Errorf("%w: emoji must be an emoji name in colons (:tomato:)", ErrUnknown)
	}
	s := slackStatus{text: text, emoji: emoji}
	if v := a["minutes"]; v != "" {
		if s.minutes, err = strconv.Atoi(v); err != nil || s.minutes < 1 || s.minutes > slackMaxMinutes {
			return slackStatus{}, fmt.Errorf("%w: minutes must be 1-%d", ErrUnknown, slackMaxMinutes)
		}
		if text == "" && emoji == "" {
			return slackStatus{}, fmt.Errorf("%w: minutes needs a text or an emoji", ErrUnknown)
		}
	}
	return s, nil
}

func checkSlackStatus(a map[string]string) error {
	_, err := slackStatusArgs(a)
	return err
}

// setStatus sets (or, with no text and no emoji, clears) the person's
// status; minutes makes it go away by itself. Setting it again is the same.
func (c *slack) setStatus(ctx context.Context, in Input) (Output, error) {
	if !slackHas(in.Token, slackScopeStatus) {
		return Output{}, ErrNotConnected
	}
	s, err := slackStatusArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	profile := map[string]any{"status_text": s.text, "status_emoji": s.emoji, "status_expiration": 0}
	row := Row{Title: s.text, Subtitle: s.emoji}
	if s.minutes > 0 {
		until := c.now().Add(time.Duration(s.minutes) * time.Minute).UTC()
		profile["status_expiration"] = until.Unix()
		row.Value = until.Format(time.RFC3339)
	}
	if err := c.do(ctx, in.Token, http.MethodPost, "/users.profile.set", map[string]any{"profile": profile}, nil); err != nil {
		return Output{}, err
	}
	return Output{Item: &row}, nil
}

func slackChannel(v string, re *regexp.Regexp, what string) (string, error) {
	if !re.MatchString(v) {
		return "", fmt.Errorf("%w: channel must be %s", ErrUnknown, what)
	}
	return v, nil
}

func slackPostArgs(a map[string]string) (channel, text string, err error) {
	if channel, err = slackChannel(a["channel"], slackChannelRe, "a Slack conversation id (C…, G… or D…)"); err != nil {
		return "", "", err
	}
	if text, err = slackText("text", a["text"], slackMaxText, true); err != nil {
		return "", "", err
	}
	if text == "" {
		return "", "", fmt.Errorf("%w: text is required", ErrUnknown)
	}
	// no @channel / @here / @everyone / user-group mentions from a generated app
	if strings.Contains(text, "<!") {
		return "", "", fmt.Errorf("%w: text cannot have special mentions (<!…>)", ErrUnknown)
	}
	return channel, text, nil
}

func checkSlackPost(a map[string]string) error {
	_, _, err := slackPostArgs(a)
	return err
}

// postMessage posts text as the person to a conversation they are in
// (parse none: no @channel or names turn into mentions). Slack has no
// idempotency key; a write op is never retried. resultId is the message ts.
func (c *slack) postMessage(ctx context.Context, in Input) (Output, error) {
	if !slackHas(in.Token, slackScopePost) {
		return Output{}, ErrNotConnected
	}
	channel, text, err := slackPostArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	body := map[string]any{"channel": channel, "text": text, "parse": "none", "link_names": false}
	if err := c.do(ctx, in.Token, http.MethodPost, "/chat.postMessage", body, &r); err != nil {
		return Output{}, err
	}
	title, _, _ := strings.Cut(text, "\n")
	if utf8.RuneCountInString(title) > 120 {
		title = string([]rune(title)[:120]) + "…"
	}
	return Output{ResultID: r.TS, Item: &Row{Title: title, Subtitle: r.Channel, Value: r.TS}}, nil
}

func checkSlackUnread(a map[string]string) error {
	_, err := slackChannel(a["channel"], slackDMRe, "a direct message id (D…)")
	return err
}

// unread is one direct message's unread count (Slack's display count).
func (c *slack) unread(ctx context.Context, in Input) (Output, error) {
	channel, err := slackChannel(in.Args["channel"], slackDMRe, "a direct message id (D…)")
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Channel struct {
			ID     string `json:"id"`
			Unread *int   `json:"unread_count_display"`
		} `json:"channel"`
	}
	if err := c.do(ctx, in.Token, http.MethodGet, "/conversations.info?"+url.Values{"channel": {channel}}.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	if r.Channel.Unread == nil {
		return Output{}, &ProviderError{Status: http.StatusBadRequest, Err: fmt.Errorf("%w: slack gives no unread count for this conversation", ErrUnknown)}
	}
	return Output{Item: &Row{Title: "未読", Subtitle: r.Channel.ID, Value: strconv.Itoa(*r.Channel.Unread)}}, nil
}

// slackCode keeps only an error code's safe form (it goes into logs).
func slackCode(s string) string {
	if slackCodeRe.MatchString(s) {
		return s
	}
	return "unknown"
}

// do calls a Web API method with the person's token. Slack answers most
// errors as 200 with ok:false and an error code:
//
//	invalid_auth, not_authed, token_revoked, token_expired, account_inactive (or HTTP 401)  → ErrInvalidGrant (connection error)
//	missing_scope, not_allowed_token_type, no_permission, access_denied ... (or HTTP 403)   → ErrNotConnected (connect again)
//	ratelimited / HTTP 429                                                                  → retryable 429 (upstream_unavailable)
//	fatal_error, internal_error, service_unavailable, request_timeout / HTTP 5xx            → retryable 503 (upstream_unavailable)
//	anything else (channel_not_found, not_in_channel, too_long ...)                         → 400 bad_request
//
// A nil body is a form POST / GET; a nil out reads only ok.
func (c *slack) do(ctx context.Context, tok *Token, method, path string, body any, out any) error {
	if tok == nil || tok.AccessToken == "" {
		return ErrNotConnected
	}
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.wire.APIBase+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusTooManyRequests:
		return &ProviderError{Status: 429, Retryable: true, Err: errors.New("slack ratelimited")}
	case resp.StatusCode >= 500:
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("slack unavailable")}
	case resp.StatusCode != http.StatusOK:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: slack status %d", ErrUnknown, resp.StatusCode)}
	}
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("slack: bad answer: %w", err)
	}
	if !r.OK {
		return slackError(r.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("slack: bad answer: %w", err)
	}
	return nil
}

// slackError maps an ok:false error code (see do).
func slackError(code string) error {
	switch code {
	case "invalid_auth", "not_authed", "token_revoked", "token_expired", "account_inactive":
		return ErrInvalidGrant
	case "missing_scope", "not_allowed_token_type", "no_permission", "access_denied", "ekm_access_denied",
		"team_access_not_granted", "org_login_required", "enterprise_is_restricted":
		return ErrNotConnected
	case "ratelimited", "rate_limited":
		return &ProviderError{Status: 429, Retryable: true, Err: errors.New("slack ratelimited")}
	case "fatal_error", "internal_error", "service_unavailable", "request_timeout":
		return &ProviderError{Status: http.StatusServiceUnavailable, Retryable: true, Err: fmt.Errorf("slack %s", code)}
	}
	return &ProviderError{Status: http.StatusBadRequest, Err: fmt.Errorf("%w: slack %s", ErrUnknown, slackCode(code))}
}
