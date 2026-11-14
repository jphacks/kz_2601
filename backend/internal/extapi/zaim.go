package extapi

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/jphacks/kz_2601/backend/internal/extapi/oauth1"
)

// Zaim (家計簿, Zaim REST API v2): free (one `runs` a call), OAuth 1.0a
// (OAuth1Client: request token → authorize → access token, HMAC-SHA1;
// oauth1 package). Zaim has no scopes: one consent covers every op.
//
//	money(limit?, startDate?, endDate?, mode?)                          the person's payments / income
//	addPayment(amount, categoryId?, genreId?, date?, place?, comment?)   one payment (write)
//
// An access token has no expiry and no refresh. Zaim has no revoke endpoint:
// disconnecting deletes the stored token (ErrNoRevoke) and the person can
// remove the app in Zaim's settings. Zaim's rate limit is not published:
// a 429 is upstream_unavailable and pauses the person's calls a minute.
//
// Env: EXT_ZAIM_ENABLED=1, EXT_ZAIM_CLIENT_ID (consumer key),
// EXT_ZAIM_CLIENT_SECRET (consumer secret; Secret Manager). Off by default;
// not in the review build (NeedsOAuth).
const Zaim = "zaim"

// zaimWire is where the provider talks to; tests point it at httptest.
type zaimWire struct {
	RequestURL string // request token (the runner)
	AuthURL    string // opened in the browser, not reached by the runner
	AccessURL  string // access token (the runner)
	APIBase    string
	// Client is nil in production: NewHTTPClient on the provider's Hosts.
	Client *http.Client
	// Now is nil in production (time.Now).
	Now func() time.Time
}

var zaimProdWire = zaimWire{
	RequestURL: "https://api.zaim.net/v2/auth/request",
	AuthURL:    "https://auth.zaim.net/users/auth",
	AccessURL:  "https://api.zaim.net/v2/auth/access",
	APIBase:    "https://api.zaim.net/v2",
}

// zaimHosts: the token endpoints and the API share one host; auth.zaim.net
// is the browser's only.
var zaimHosts = []string{"https://api.zaim.net"}

func init() {
	register(Provider{ID: Zaim, Billing: BillingFree, NeedsOAuth: true, NameJa: "Zaim", NameEn: "Zaim",
		Ops: []OpSpec{
			{Name: "money", Args: []string{"limit", "startDate", "endDate", "mode"}, Check: checkZaimMoney},
			{Name: "addPayment", Write: true, Args: []string{"amount", "categoryId", "genreId", "date", "place", "comment"}, Check: checkZaimAddPayment},
		},
		Hosts: zaimHosts,
		New: func(sec Secrets) (map[string]Operation, error) {
			z, err := newZaim(sec, zaimProdWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"money": OperationFunc(z.money), "addPayment": OperationFunc(z.addPayment)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) { return newZaim(sec, zaimProdWire) },
	})
}

// zaim is the provider's ops and OAuth 1.0a client.
type zaim struct {
	sec  Secrets
	wire zaimWire
	hc   *http.Client
	now  func() time.Time
}

func newZaim(sec Secrets, w zaimWire) (*zaim, error) {
	if sec.ClientID == "" || sec.ClientSecret == "" {
		return nil, errors.New("EXT_ZAIM_CLIENT_ID / _CLIENT_SECRET not set")
	}
	hc := w.Client
	if hc == nil {
		hc = NewHTTPClient(zaimHosts, DefaultTimeout)
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	return &zaim{sec: sec, wire: w, hc: hc, now: now}, nil
}

// --- OAuth 1.0a ---

var _ OAuth1Client = (*zaim)(nil)

// AuthURL is unused: Start calls Begin (the consent URL needs the request
// token first).
func (z *zaim) AuthURL(string, string, string, []string) string { return "" }

// Exchange is unused: Callback calls Exchange1.
func (z *zaim) Exchange(context.Context, string, string, string) (Token, error) {
	return Token{}, errors.New("zaim: OAuth 1.0a uses Exchange1")
}

// Begin gets a request token for redirectURI and returns the consent URL.
func (z *zaim) Begin(ctx context.Context, redirectURI string) (string, string, string, error) {
	v, err := z.tokenRequest(ctx, z.wire.RequestURL, "", "", oauth1.Param{Key: "oauth_callback", Value: redirectURI})
	if err != nil {
		return "", "", "", err
	}
	if v.Get("oauth_callback_confirmed") != "true" {
		return "", "", "", errors.New("zaim: callback not confirmed")
	}
	rt, secret := v.Get("oauth_token"), v.Get("oauth_token_secret")
	return z.wire.AuthURL + "?" + url.Values{"oauth_token": {rt}}.Encode(), rt, secret, nil
}

// Exchange1 turns the authorized request token and the verifier into the
// access token (Token.RefreshToken holds the token secret; no expiry).
func (z *zaim) Exchange1(ctx context.Context, requestToken, secret, verifier string) (Token, error) {
	if requestToken == "" || secret == "" || verifier == "" {
		return Token{}, ErrInvalidGrant
	}
	v, err := z.tokenRequest(ctx, z.wire.AccessURL, requestToken, secret, oauth1.Param{Key: "oauth_verifier", Value: verifier})
	if err != nil {
		return Token{}, err
	}
	return Token{AccessToken: v.Get("oauth_token"), RefreshToken: v.Get("oauth_token_secret")}, nil
}

// Refresh: an OAuth 1.0a token is not renewed (it has no expiry either).
func (z *zaim) Refresh(context.Context, Token) (Token, error) { return Token{}, ErrInvalidGrant }

// Revoke: Zaim has no revoke endpoint, so only the stored token is deleted.
func (z *zaim) Revoke(context.Context, Token) error { return ErrNoRevoke }

// tokenRequest posts a signed request to a token endpoint and reads its
// form-encoded answer (oauth_token, oauth_token_secret). A 401 is a refused
// request token / verifier (ErrInvalidGrant).
func (z *zaim) tokenRequest(ctx context.Context, endpoint, token, secret string, extra oauth1.Param) (url.Values, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := z.sign(req, "", token, secret, extra); err != nil {
		return nil, err
	}
	resp, err := z.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidGrant
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode >= 500 || resp.StatusCode == 429, Err: errors.New("token endpoint refused")}
	}
	v, err := url.ParseQuery(string(body))
	if err != nil || v.Get("oauth_token") == "" || v.Get("oauth_token_secret") == "" {
		return nil, errors.New("zaim: bad token answer")
	}
	return v, nil
}

// sign puts the OAuth 1.0a Authorization header on req (formBody is the
// form-encoded body when there is one; it is signed too).
func (z *zaim) sign(req *http.Request, formBody, token, secret string, extra ...oauth1.Param) error {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	h, err := oauth1.Header(req.Method, req.URL, formBody, z.sec.ClientID, z.sec.ClientSecret, token, secret, hex.EncodeToString(nonce), z.now().Unix(), extra...)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", h)
	return nil
}

// --- arguments ---

const (
	zaimDefaultLimit = 20
	zaimMaxLimit     = 100
	zaimMaxRange     = 366 * 24 * time.Hour
	zaimMaxAmount    = 100_000_000
	zaimTextMax      = 100 // Zaim's place / comment / name
)

var (
	zaimIDRe    = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
	zaimModes   = []string{"payment", "income", "transfer"}
	zaimModesJa = map[string]string{"payment": "支出", "income": "収入", "transfer": "振替"}
)

func zaimLimit(v string) (int, error) {
	if v == "" {
		return zaimDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > zaimMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrBadArgs, zaimMaxLimit)
	}
	return x, nil
}

// zaimDate reads YYYY-MM-DD (JST).
func zaimDate(name, v string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, v, gcalJST)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrBadArgs, name)
	}
	return t, nil
}

// zaimMoneyQuery turns money's arguments into the query (mapping=1 is
// Zaim's required marker).
func zaimMoneyQuery(a map[string]string) (url.Values, error) {
	n, err := zaimLimit(a["limit"])
	if err != nil {
		return nil, err
	}
	v := url.Values{"mapping": {"1"}, "limit": {strconv.Itoa(n)}, "order": {"date"}}
	var from, to time.Time
	if s := a["startDate"]; s != "" {
		if from, err = zaimDate("startDate", s); err != nil {
			return nil, err
		}
		v.Set("start_date", s)
	}
	if s := a["endDate"]; s != "" {
		if to, err = zaimDate("endDate", s); err != nil {
			return nil, err
		}
		v.Set("end_date", s)
	}
	if !from.IsZero() && !to.IsZero() && (to.Before(from) || to.Sub(from) > zaimMaxRange) {
		return nil, fmt.Errorf("%w: endDate must be on or after startDate and within 366 days", ErrBadArgs)
	}
	if m := a["mode"]; m != "" {
		if !slices.Contains(zaimModes, m) {
			return nil, fmt.Errorf("%w: mode must be payment, income or transfer", ErrBadArgs)
		}
		v.Set("mode", m)
	}
	return v, nil
}

func checkZaimMoney(a map[string]string) error {
	_, err := zaimMoneyQuery(a)
	return err
}

// zaimPayment is addPayment's checked arguments.
type zaimPayment struct {
	amount              int64
	categoryID, genreID string
	date                string // "" = today (JST) when sent
	place, comment      string
}

func zaimPaymentArgs(a map[string]string) (zaimPayment, error) {
	var p zaimPayment
	amt, err := strconv.ParseInt(a["amount"], 10, 64)
	if err != nil || amt < 1 || amt > zaimMaxAmount {
		return p, fmt.Errorf("%w: amount must be a whole number of yen, 1-%d", ErrBadArgs, zaimMaxAmount)
	}
	p.amount = amt
	for name, dst := range map[string]*string{"categoryId": &p.categoryID, "genreId": &p.genreID} {
		if v := a[name]; v != "" {
			if !zaimIDRe.MatchString(v) {
				return p, fmt.Errorf("%w: %s must be a Zaim id (digits)", ErrBadArgs, name)
			}
			*dst = v
		}
	}
	if p.categoryID == "" && p.genreID == "" {
		return p, fmt.Errorf("%w: categoryId or genreId is required", ErrBadArgs)
	}
	if v := a["date"]; v != "" {
		if _, err := zaimDate("date", v); err != nil {
			return p, err
		}
		p.date = v
	}
	if p.place, err = gcalText("place", a["place"], zaimTextMax, false); err != nil {
		return p, err
	}
	if p.comment, err = gcalText("comment", a["comment"], zaimTextMax, false); err != nil {
		return p, err
	}
	return p, nil
}

func checkZaimAddPayment(a map[string]string) error {
	_, err := zaimPaymentArgs(a)
	return err
}

// --- answers ---

type zaimMoney struct {
	ID         int64  `json:"id"`
	Mode       string `json:"mode"`
	Date       string `json:"date"`
	CategoryID int64  `json:"category_id"`
	GenreID    int64  `json:"genre_id"`
	Amount     int64  `json:"amount"`
	Name       string `json:"name"`
	Place      string `json:"place"`
	Comment    string `json:"comment"`
	Currency   string `json:"currency_code"`
}

type zaimCategory struct {
	ID     int64  `json:"id"`
	Mode   string `json:"mode"`
	Name   string `json:"name"`
	Sort   int    `json:"sort"`
	Active int    `json:"active"`
}

type zaimGenre struct {
	ID         int64  `json:"id"`
	CategoryID int64  `json:"category_id"`
	Name       string `json:"name"`
	Sort       int    `json:"sort"`
	Active     int    `json:"active"`
}

// zaimNames are the person's category and genre names by id.
type zaimNames struct {
	categories map[int64]string
	genres     map[int64]string
}

// label is "category/genre" (whichever is known).
func (n zaimNames) label(categoryID, genreID int64) string {
	c, g := n.categories[categoryID], n.genres[genreID]
	switch {
	case c != "" && g != "" && c != g:
		return c + "/" + g
	case g != "":
		return g
	}
	return c
}

// zaimAmount is "1,200 円" (another currency: "12.50 USD" as Zaim gives it).
func zaimAmount(amount int64, currency string) string {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	s := strconv.FormatInt(amount, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	if currency == "" || currency == "JPY" {
		return s + " 円"
	}
	return s + " " + currency
}

// row: title = the item (name), or the category/genre when it has none;
// subtitle = date, place and the mode when it is not a payment; value = the
// amount. Zaim has no page for one record, so no link.
func (m zaimMoney) row(names zaimNames) Row {
	title := strings.TrimSpace(m.Name)
	if title == "" {
		title = names.label(m.CategoryID, m.GenreID)
	}
	if title == "" {
		title = zaimModesJa[m.Mode]
	}
	if title == "" {
		title = "(品目なし)"
	}
	parts := []string{m.Date}
	if p := strings.TrimSpace(m.Place); p != "" {
		parts = append(parts, p)
	}
	if m.Mode != "payment" && zaimModesJa[m.Mode] != "" {
		parts = append(parts, zaimModesJa[m.Mode])
	}
	return Row{Title: title, Subtitle: strings.Join(parts, "・"), Value: zaimAmount(m.Amount, m.Currency)}
}

// --- ops ---

// money lists the person's records (default: the latest 20, every mode),
// newest first. Category and genre names are read only when a record has
// no item name.
func (z *zaim) money(ctx context.Context, in Input) (Output, error) {
	v, err := zaimMoneyQuery(in.Args)
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Money []zaimMoney `json:"money"`
	}
	if err := z.do(ctx, in.Token, http.MethodGet, "/home/money?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	var names zaimNames
	if slices.ContainsFunc(r.Money, func(m zaimMoney) bool { return strings.TrimSpace(m.Name) == "" }) {
		if names, err = z.names(ctx, in.Token); err != nil {
			return Output{}, err
		}
	}
	slices.SortStableFunc(r.Money, func(a, b zaimMoney) int {
		if c := strings.Compare(b.Date, a.Date); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	out := Output{Rows: make([]Row, 0, len(r.Money)), Cells: make([][]string, 0, len(r.Money))}
	for _, m := range r.Money {
		out.Rows = append(out.Rows, m.row(names))
		out.Cells = append(out.Cells, []string{m.Date, zaimModesJa[m.Mode], names.categories[m.CategoryID], names.genres[m.GenreID],
			m.Name, m.Place, strconv.FormatInt(m.Amount, 10), m.Comment})
	}
	return out, nil
}

// names reads the person's categories and genres (two calls).
func (z *zaim) names(ctx context.Context, t *Token) (zaimNames, error) {
	cs, gs, err := z.lists(ctx, t)
	if err != nil {
		return zaimNames{}, err
	}
	n := zaimNames{categories: map[int64]string{}, genres: map[int64]string{}}
	for _, c := range cs {
		n.categories[c.ID] = c.Name
	}
	for _, g := range gs {
		n.genres[g.ID] = g.Name
	}
	return n, nil
}

func (z *zaim) lists(ctx context.Context, t *Token) ([]zaimCategory, []zaimGenre, error) {
	var cr struct {
		Categories []zaimCategory `json:"categories"`
	}
	if err := z.do(ctx, t, http.MethodGet, "/home/category?mapping=1", nil, &cr); err != nil {
		return nil, nil, err
	}
	var gr struct {
		Genres []zaimGenre `json:"genres"`
	}
	if err := z.do(ctx, t, http.MethodGet, "/home/genre?mapping=1", nil, &gr); err != nil {
		return nil, nil, err
	}
	return cr.Categories, gr.Genres, nil
}

// addPayment adds one payment. Zaim needs both a category and a genre: a
// genreId alone takes its category; a categoryId alone takes the
// category's first active genre; neither is 400 before any call. An id the
// person does not have (or an inactive one: Zaim's active is 1 / -1) is 400
// after the lookup. Zaim has no idempotency
// key, so the service's never-retried write is the only guard against a
// second record. date defaults to today (JST).
func (z *zaim) addPayment(ctx context.Context, in Input) (Output, error) {
	p, err := zaimPaymentArgs(in.Args)
	if err != nil {
		return Output{}, err
	}
	cs, gs, err := z.lists(ctx, in.Token)
	if err != nil {
		return Output{}, err
	}
	category, genre, err := zaimPickGenre(cs, gs, p.categoryID, p.genreID)
	if err != nil {
		return Output{}, err
	}
	if p.date == "" {
		p.date = z.now().In(gcalJST).Format(time.DateOnly)
	}
	form := url.Values{
		"mapping":     {"1"},
		"category_id": {strconv.FormatInt(category.ID, 10)},
		"genre_id":    {strconv.FormatInt(genre.ID, 10)},
		"amount":      {strconv.FormatInt(p.amount, 10)},
		"date":        {p.date},
	}
	if p.place != "" {
		form.Set("place", p.place)
	}
	if p.comment != "" {
		form.Set("comment", p.comment)
	}
	var r struct {
		Money struct {
			ID int64 `json:"id"`
		} `json:"money"`
	}
	if err := z.do(ctx, in.Token, http.MethodPost, "/home/money/payment", form, &r); err != nil {
		return Output{}, err
	}
	m := zaimMoney{ID: r.Money.ID, Mode: "payment", Date: p.date, CategoryID: category.ID, GenreID: genre.ID, Amount: p.amount, Place: p.place, Comment: p.comment}
	row := m.row(zaimNames{categories: map[int64]string{category.ID: category.Name}, genres: map[int64]string{genre.ID: genre.Name}})
	return Output{ResultID: strconv.FormatInt(r.Money.ID, 10), Item: &row}, nil
}

// zaimPickGenre finds the payment category and genre of the ids (see
// addPayment). Not found is bad_request after the lookup.
func zaimPickGenre(cs []zaimCategory, gs []zaimGenre, categoryID, genreID string) (zaimCategory, zaimGenre, error) {
	if genreID != "" {
		id, _ := strconv.ParseInt(genreID, 10, 64)
		i := slices.IndexFunc(gs, func(g zaimGenre) bool { return g.ID == id && g.Active == 1 })
		if i < 0 {
			return zaimCategory{}, zaimGenre{}, afterCall(fmt.Errorf("%w: zaim has no such genreId", ErrBadArgs))
		}
		g := gs[i]
		if categoryID != "" && strconv.FormatInt(g.CategoryID, 10) != categoryID {
			return zaimCategory{}, zaimGenre{}, afterCall(fmt.Errorf("%w: genreId is not in categoryId", ErrBadArgs))
		}
		j := slices.IndexFunc(cs, func(c zaimCategory) bool { return c.ID == g.CategoryID })
		if j < 0 || cs[j].Mode != "payment" || cs[j].Active != 1 {
			return zaimCategory{}, zaimGenre{}, afterCall(fmt.Errorf("%w: genreId is not a payment genre", ErrBadArgs))
		}
		return cs[j], g, nil
	}
	id, _ := strconv.ParseInt(categoryID, 10, 64)
	j := slices.IndexFunc(cs, func(c zaimCategory) bool { return c.ID == id && c.Active == 1 && c.Mode == "payment" })
	if j < 0 {
		return zaimCategory{}, zaimGenre{}, afterCall(fmt.Errorf("%w: zaim has no such payment categoryId", ErrBadArgs))
	}
	var best *zaimGenre
	for i := range gs {
		g := &gs[i]
		if g.CategoryID != id || g.Active != 1 {
			continue
		}
		if best == nil || g.Sort < best.Sort {
			best = g
		}
	}
	if best == nil {
		return zaimCategory{}, zaimGenre{}, afterCall(fmt.Errorf("%w: categoryId has no genre", ErrBadArgs))
	}
	return cs[j], *best, nil
}

// do calls the API with the person's token (every request signed) and
// decodes the answer. 401 is a token Zaim no longer takes (ErrInvalidGrant:
// the connection turns to error); 403 is ErrNotConnected (connect again);
// 429 / 5xx are retryable provider errors; 400 / 404 are bad_request after
// the call.
func (z *zaim) do(ctx context.Context, t *Token, method, path string, form url.Values, out any) error {
	if t == nil || t.AccessToken == "" || t.RefreshToken == "" {
		return ErrNotConnected
	}
	body := ""
	if form != nil {
		body = form.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, z.wire.APIBase+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if err := z.sign(req, body, t.AccessToken, t.RefreshToken); err != nil {
		return err
	}
	resp, err := z.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := ReadBody(resp.Body)
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("zaim: bad answer: %w", err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrInvalidGrant
	case resp.StatusCode == http.StatusForbidden:
		return ErrNotConnected
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound:
		return &ProviderError{Status: resp.StatusCode, Err: fmt.Errorf("%w: zaim refused the request", ErrUnknown)}
	}
	return &ProviderError{Status: resp.StatusCode, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500, Err: errors.New("zaim")}
}
