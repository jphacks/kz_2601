package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestNotificationWhen2592(t *testing.T) {
	cases := []struct {
		name string
		n    dsl.Notification
		want string
	}{
		{"morning", dsl.Notification{Time: "08:00", Repeat: "daily"}, "毎朝8時"},
		{"morning with minutes", dsl.Notification{Time: "07:30", Repeat: "daily"}, "毎朝7時30分"},
		{"noon", dsl.Notification{Time: "12:00", Repeat: "daily"}, "毎日12時"},
		{"evening", dsl.Notification{Time: "21:00", Repeat: "daily"}, "毎晩9時"},
		{"midnight", dsl.Notification{Time: "00:00", Repeat: "daily"}, "毎朝0時"},
		{"weekly with time", dsl.Notification{Time: "19:05", Repeat: "weekly"}, "毎週7時5分"},
		{"once", dsl.Notification{Time: "10:00"}, "10時"},
		{"weekly without time", dsl.Notification{Repeat: "weekly"}, "毎週"},
		{"daily without time", dsl.Notification{Repeat: "daily"}, ""},
		{"relative time is not a clock", dsl.Notification{Time: "+1h", Repeat: "daily"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := notificationWhen(c.n); got != c.want {
				t.Errorf("notificationWhen(%+v) = %q, want %q", c.n, got, c.want)
			}
		})
	}
}

func TestMapStoreErr2592(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{
			"not found keeps status and code",
			&appconfigclient.Error{Status: http.StatusNotFound, Code: httpx.CodeNotFound, Message: "app not found"},
			http.StatusNotFound, httpx.CodeNotFound, "app not found",
		},
		{
			"conflict keeps status and code",
			fmt.Errorf("save: %w", &appconfigclient.Error{Status: http.StatusConflict, Code: httpx.CodeVersionConflict, Message: "stale"}),
			http.StatusConflict, httpx.CodeVersionConflict, "stale",
		},
		{
			"empty code becomes appconfig_error",
			&appconfigclient.Error{Status: http.StatusInternalServerError, Message: "boom"},
			http.StatusInternalServerError, httpx.CodeAppconfigError, "boom",
		},
		{
			"other errors are 502",
			errors.New("dial tcp: refused"),
			http.StatusBadGateway, httpx.CodeAppconfigError, "dial tcp: refused",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ge *Error
			if !errors.As(mapStoreErr(c.err), &ge) {
				t.Fatalf("mapStoreErr did not return *Error")
			}
			if ge.Status != c.wantStatus || ge.Code != c.wantCode || ge.Message != c.wantMsg {
				t.Errorf("got %d %q %q, want %d %q %q", ge.Status, ge.Code, ge.Message, c.wantStatus, c.wantCode, c.wantMsg)
			}
		})
	}
}

func TestMapStoreErrDetails2592(t *testing.T) {
	d := map[string]any{"features": []string{"x"}}
	var ge *Error
	if !errors.As(mapStoreErr(&appconfigclient.Error{Status: http.StatusPaymentRequired, Code: "pro_required", Details: d}), &ge) {
		t.Fatal("not *Error")
	}
	if ge.Details == nil || ge.Details["features"] == nil {
		t.Errorf("details not carried: %+v", ge.Details)
	}
}

func TestMapClaudeErr2592(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"429", &APIError{Status: 429, Type: "rate_limit_error"}, http.StatusTooManyRequests, httpx.CodeRateLimited, "Claude rate limit"},
		{"529 overloaded", &APIError{Status: 529, Type: "overloaded_error"}, http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "Claude unavailable: overloaded_error"},
		{"500", &APIError{Status: 500, Type: "api_error"}, http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "Claude unavailable: api_error"},
		{"400", &APIError{Status: 400, Type: "invalid_request_error", Message: "bad"}, http.StatusInternalServerError, httpx.CodeInternal, "Claude request rejected: bad"},
		{"401", &APIError{Status: 401, Type: "authentication_error", Message: "nope"}, http.StatusInternalServerError, httpx.CodeInternal, "Claude request rejected: nope"},
		{"403", &APIError{Status: 403, Type: "permission_error", Message: "no"}, http.StatusInternalServerError, httpx.CodeInternal, "Claude request rejected: no"},
		{"timeout", fmt.Errorf("call: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, httpx.CodeUpstreamTimeout, "Claude call timed out"},
		{"canceled", context.Canceled, http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "context canceled"},
		{"other", errors.New("eof"), http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "eof"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ge *Error
			if !errors.As(mapClaudeErr(c.err), &ge) {
				t.Fatalf("mapClaudeErr did not return *Error")
			}
			if ge.Status != c.wantStatus || ge.Code != c.wantCode || ge.Message != c.wantMsg {
				t.Errorf("got %d %q %q, want %d %q %q", ge.Status, ge.Code, ge.Message, c.wantStatus, c.wantCode, c.wantMsg)
			}
		})
	}
}

func TestMapClaudeErrPassesThroughError2592(t *testing.T) {
	in := &Error{Status: http.StatusUnprocessableEntity, Code: "x"}
	if got := mapClaudeErr(fmt.Errorf("wrap: %w", in)); got != in {
		t.Errorf("mapClaudeErr = %v, want the same *Error", got)
	}
}

func TestErrType2592(t *testing.T) {
	cases := []struct {
		name string
		e    *wireError
		want string
	}{
		{"nil", nil, "error"},
		{"empty type", &wireError{Message: "m"}, "error"},
		{"overloaded", &wireError{Type: "overloaded_error"}, "overloaded_error"},
	}
	for _, c := range cases {
		if got := errType(c.e); got != c.want {
			t.Errorf("%s: errType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestErrorKind2592(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"429", &APIError{Status: 429, Type: "rate_limit_error"}, "429_rate_limit_error"},
		{"529", fmt.Errorf("w: %w", &APIError{Status: 529, Type: "overloaded_error"}), "529_overloaded_error"},
		{"500", &APIError{Status: 500, Type: "api_error"}, "500_api_error"},
		{"timeout", fmt.Errorf("w: %w", context.DeadlineExceeded), "deadline_exceeded"},
		{"canceled", context.Canceled, "canceled"},
		{"other", errors.New("x"), "error"},
	}
	for _, c := range cases {
		if got := errorKind(c.err); got != c.want {
			t.Errorf("%s: errorKind = %q, want %q", c.name, got, c.want)
		}
	}
}
