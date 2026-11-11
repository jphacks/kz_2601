package extapi

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// The fake providers (決定 5): fixed answers for stg and tests, so the whole
// path (press → appconfig → runner → answer, counting, connecting) runs
// without a real key. Never enabled in production (New skips them).
//
//	fakeFree   free, no OAuth   op listItems(q)                  EXT_FAKE_FREE_ENABLED=1
//	fakePaid   paid, no OAuth   op echo(text, costUsd)           EXT_FAKE_PAID_ENABLED=1, COIN_RESERVE_EXT_FAKE_PAID
//	fakeOauth  free, OAuth      ops listEvents, createEvent(title) EXT_FAKE_OAUTH_ENABLED=1
//
// Every op takes fail=5xx | 4xx | timeout to fail the way a provider does
// (4xx is billable: the provider charged it).
const (
	FakeFree  = "fakeFree"
	FakePaid  = "fakePaid"
	FakeOAuth = "fakeOauth"
)

func init() {
	register(Provider{ID: FakeFree, Billing: BillingFree, Fake: true,
		Ops: []OpSpec{{Name: "listItems", Args: []string{"q", "fail"}}},
		New: func(Secrets) (map[string]Operation, error) {
			return map[string]Operation{"listItems": fakeOp(func(in Input) Output {
				q := in.Args["q"]
				return Output{Rows: []Row{{Title: "fake 1", Subtitle: q, Value: "1"}, {Title: "fake 2", Subtitle: q, Value: "2"}}}
			})}, nil
		}})
	register(Provider{ID: FakePaid, Billing: BillingPaid, Fake: true,
		Ops: []OpSpec{{Name: "echo", Args: []string{"text", "costUsd", "fail"}}},
		New: func(Secrets) (map[string]Operation, error) {
			return map[string]Operation{"echo": fakeOp(func(in Input) Output {
				out := Output{Item: &Row{Title: in.Args["text"]}}
				if v, err := strconv.ParseFloat(in.Args["costUsd"], 64); err == nil && v >= 0 {
					out.Usage = Usage{USD: v, Priced: true}
				}
				return out
			})}, nil
		}})
	register(Provider{ID: FakeOAuth, Billing: BillingFree, Fake: true, NeedsOAuth: true, NameJa: "テスト用サービス", NameEn: "Test service",
		Ops: []OpSpec{
			{Name: "listEvents", Scopes: []string{"fake.read"}, Args: []string{"fail"}},
			{Name: "createEvent", Write: true, Scopes: []string{"fake.write"}, Args: []string{"title", "fail"}},
		},
		New: func(Secrets) (map[string]Operation, error) {
			need := func(in Input) bool { return in.Token != nil && strings.HasPrefix(in.Token.AccessToken, "fake-access-") }
			return map[string]Operation{
				"listEvents": fakeOp(func(in Input) Output {
					if !need(in) {
						return Output{}
					}
					return Output{Rows: []Row{{Title: "fake event", Value: "2026-11-11T10:00:00+09:00"}}}
				}),
				"createEvent": fakeOp(func(in Input) Output {
					return Output{ResultID: "fake-event-" + in.RequestID, Item: &Row{Title: in.Args["title"]}}
				}),
			}, nil
		},
		NewOAuth: func(Secrets) (OAuthClient, error) { return fakeOAuthClient{}, nil },
	})
}

// fakeOp answers with f, or fails as Args["fail"] says.
func fakeOp(f func(Input) Output) Operation {
	return OperationFunc(func(ctx context.Context, in Input) (Output, error) {
		switch in.Args["fail"] {
		case "5xx":
			return Output{}, &ProviderError{Status: 503, Retryable: true, Err: errors.New("fake unavailable")}
		case "4xx":
			return Output{}, &ProviderError{Status: 400, Billable: true, Err: errors.New("fake bad request")}
		case "timeout":
			<-ctx.Done()
			return Output{}, ctx.Err()
		case "grant":
			return Output{}, ErrInvalidGrant
		}
		return f(in), nil
	})
}

// fakeOAuthClient consents at once: its AuthURL is the runner's own
// callback with a code, so stg can connect without a real provider.
type fakeOAuthClient struct{}

func (fakeOAuthClient) AuthURL(state, _, redirectURI string, _ []string) string {
	return redirectURI + "?" + url.Values{"state": {state}, "code": {"fake-code"}}.Encode()
}

func (fakeOAuthClient) Exchange(_ context.Context, code, verifier, _ string) (Token, error) {
	if code != "fake-code" || verifier == "" {
		return Token{}, ErrInvalidGrant
	}
	return Token{AccessToken: "fake-access-1", RefreshToken: "fake-refresh-1"}, nil
}

func (fakeOAuthClient) Refresh(_ context.Context, t Token) (Token, error) {
	if t.RefreshToken != "fake-refresh-1" {
		return Token{}, ErrInvalidGrant
	}
	return Token{AccessToken: "fake-access-2", Scopes: t.Scopes}, nil
}

func (fakeOAuthClient) Revoke(context.Context, Token) error { return nil }
