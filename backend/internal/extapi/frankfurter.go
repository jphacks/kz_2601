package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Frankfurter (#1499): exchange rates of the European Central Bank
// (https://frankfurter.dev). Free, no key, no OAuth: one call is one `runs`.
// Only api.frankfurter.app is reached; the answer becomes rows (currency,
// rate). ECB rates change once a working day, so an answer is cached
// (latest 1 hour, a past date 1 day).
//
//	latest(from?, to?, amount?)          today's rates    EXT_FRANKFURTER_ENABLED=1
//	historical(date, from?, to?, amount?) the rates of a day (from 1999-01-04)
//
// from / to are ISO 4217 codes in capitals (from defaults to EUR at the
// provider, to to every currency it has); to is at most 10 codes joined by
// commas; amount is a positive decimal.
const Frankfurter = "frankfurter"

var frankfurterHosts = []string{"https://api.frankfurter.app"}

const (
	frankfurterMaxBody = 64 << 10
	frankfurterMaxTo   = 10
	frankfurterMaxRows = 60
)

// frankfurterFirstDay is the first day of the ECB reference rates.
var frankfurterFirstDay = time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)

var (
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	amountRe   = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,6})?$`)
	dateRe     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

func init() {
	register(Provider{ID: Frankfurter, Billing: BillingFree, Hosts: frankfurterHosts,
		Ops: []OpSpec{
			{Name: "latest", Args: []string{"from", "to", "amount"}, Check: checkFXArgs, CacheTTL: time.Hour},
			{Name: "historical", Args: []string{"date", "from", "to", "amount"}, Check: checkFXHistorical, CacheTTL: 24 * time.Hour},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newFrankfurter(frankfurterHosts[0], NewHTTPClient(frankfurterHosts, DefaultTimeout)), nil
		}})
}

// checkFXArgs checks from / to / amount.
func checkFXArgs(a map[string]string) error {
	if v, ok := a["from"]; ok && !currencyRe.MatchString(v) {
		return errors.New("from must be a currency code like USD")
	}
	if v, ok := a["to"]; ok {
		codes := strings.Split(v, ",")
		if len(codes) > frankfurterMaxTo {
			return fmt.Errorf("to takes at most %d currencies", frankfurterMaxTo)
		}
		for i, c := range codes {
			if !currencyRe.MatchString(c) || slices.Contains(codes[:i], c) {
				return errors.New("to must be currency codes like JPY,EUR (no repeats)")
			}
		}
	}
	if v, ok := a["amount"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if !amountRe.MatchString(v) || err != nil || f <= 0 {
			return errors.New("amount must be a positive number")
		}
	}
	return nil
}

// checkFXHistorical also checks date (YYYY-MM-DD, from 1999-01-04, not after today).
func checkFXHistorical(a map[string]string) error {
	d, ok := a["date"]
	if !ok {
		return errors.New("date is required")
	}
	t, err := time.Parse(time.DateOnly, d)
	if !dateRe.MatchString(d) || err != nil {
		return errors.New("date must be YYYY-MM-DD")
	}
	if t.Before(frankfurterFirstDay) || t.After(time.Now().UTC().Add(24*time.Hour)) {
		return errors.New("date must be between 1999-01-04 and today")
	}
	return checkFXArgs(a)
}

// newFrankfurter builds the ops against base (tests pass an httptest server).
func newFrankfurter(base string, hc *http.Client) map[string]Operation {
	f := &frankfurter{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{
		"latest":     OperationFunc(func(ctx context.Context, in Input) (Output, error) { return f.rates(ctx, "latest", in.Args) }),
		"historical": OperationFunc(func(ctx context.Context, in Input) (Output, error) { return f.rates(ctx, in.Args["date"], in.Args) }),
	}
}

type frankfurter struct {
	base string
	hc   *http.Client
}

// fxAnswer is the provider's JSON (only what becomes rows).
type fxAnswer struct {
	Amount json.Number            `json:"amount"`
	Base   string                 `json:"base"`
	Date   string                 `json:"date"`
	Rates  map[string]json.Number `json:"rates"`
}

func (f *frankfurter) rates(ctx context.Context, path string, args map[string]string) (Output, error) {
	q := url.Values{}
	for _, k := range []string{"from", "to", "amount"} {
		if v, ok := args[k]; ok {
			q.Set(k, v)
		}
	}
	u := f.base + "/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Output{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return Output{}, &ProviderError{Retryable: true, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnprocessableEntity || resp.StatusCode == http.StatusBadRequest:
		// a currency or a day the provider does not have
		return Output{}, fmt.Errorf("%w: frankfurter %d", ErrUnknown, resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return Output{}, &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("frankfurter unavailable")}
	case resp.StatusCode != http.StatusOK:
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("frankfurter refused")}
	}
	body, err := ReadBody(io.LimitReader(resp.Body, frankfurterMaxBody+1))
	if err == nil && len(body) > frankfurterMaxBody {
		err = fmt.Errorf("body over %d bytes", frankfurterMaxBody)
	}
	if err != nil {
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: err}
	}
	var a fxAnswer
	if err := json.Unmarshal(body, &a); err != nil || !currencyRe.MatchString(a.Base) || !dateRe.MatchString(a.Date) {
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("frankfurter answered an unexpected shape")}
	}
	return Output{Rows: fxRows(a, args["to"])}, nil
}

// fxRows is one row per currency: title the code, value the rate for
// amount, subtitle "date・amount base". The order of to, else by code.
func fxRows(a fxAnswer, to string) []Row {
	amount := string(a.Amount)
	if f, err := a.Amount.Float64(); err == nil {
		amount = strconv.FormatFloat(f, 'f', -1, 64)
	}
	sub := a.Date + "・" + amount + " " + a.Base
	var codes []string
	if to != "" {
		codes = strings.Split(to, ",")
	} else {
		for c := range a.Rates {
			codes = append(codes, c)
		}
		slices.Sort(codes)
	}
	rows := []Row{}
	for _, c := range codes {
		v, ok := a.Rates[c]
		if !ok || !currencyRe.MatchString(c) {
			continue
		}
		if _, err := v.Float64(); err != nil {
			continue
		}
		rows = append(rows, Row{Title: c, Subtitle: sub, Value: v.String()})
		if len(rows) == frankfurterMaxRows {
			break
		}
	}
	return rows
}
