package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendJSONMapsFailures(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == 0 {
			time.Sleep(300 * time.Millisecond)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"echo":"` + r.URL.Query().Get("appid") + `"}`))
	}))
	defer srv.Close()
	get := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"?appid=secret-key", nil)
		var out map[string]string
		return sendJSON(ctx, srv.Client(), req, &out)
	}
	for _, c := range []struct {
		status    int
		reason    string
		retryable bool
	}{
		{http.StatusOK, ReasonOK, false},
		{http.StatusServiceUnavailable, ReasonUpstream, true},
		{http.StatusTooManyRequests, ReasonUpstream, true},
		{http.StatusBadRequest, ReasonUpstream, false},
		{0, ReasonUpstreamTimeout, false},
	} {
		status = c.status
		err := get()
		if Reason(err) != c.reason {
			t.Fatalf("status %d: reason %s (%v)", c.status, Reason(err), err)
		}
		var pe *ProviderError
		if errors.As(err, &pe) && (pe.Billable || pe.Retryable != c.retryable) {
			t.Fatalf("status %d: %+v", c.status, pe)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Fatalf("status %d: the key is in the error: %v", c.status, err)
		}
	}
}

func TestTransportErrorDropsTheURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	req, _ := http.NewRequest(http.MethodGet, u+"?appid=secret-key", nil)
	var out any
	err := sendJSON(context.Background(), http.DefaultClient, req, &out)
	if Reason(err) != ReasonUpstream || strings.Contains(err.Error(), "secret-key") || strings.Contains(err.Error(), u) {
		t.Fatalf("%v", err)
	}
	// a host outside the list is egress, not the provider's fault
	hc := NewHTTPClient([]string{"https://allowed.example"}, time.Second)
	req, _ = http.NewRequest(http.MethodGet, "https://other.example/?appid=secret-key", nil)
	if err := sendJSON(context.Background(), hc, req, &out); !errors.Is(err, ErrEgress) || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("%v", err)
	}
}

func TestArgLatLng(t *testing.T) {
	lat, lng, ok, err := argLatLng(map[string]string{"lat": "36.5613", "lng": "136.6562"}, 2)
	if err != nil || !ok || lat != 36.56 || lng != 136.66 {
		t.Fatalf("%v %v %v %v", lat, lng, ok, err)
	}
	if _, _, ok, err := argLatLng(map[string]string{}, 2); ok || err != nil {
		t.Fatal("missing both is not an error")
	}
	for _, a := range []map[string]string{
		{"lat": "36.5"},
		{"lat": "91", "lng": "0"},
		{"lat": "0", "lng": "-180.01"},
		{"lat": "NaN", "lng": "0"},
		{"lat": "0x10", "lng": "0"},
		{"lat": "1e400", "lng": "0"},
		{"lat": "abc", "lng": "0"},
	} {
		if _, _, _, err := argLatLng(a, 2); !errors.Is(err, ErrUnknown) {
			t.Fatalf("%v accepted", a)
		}
	}
	if n, err := argInt(map[string]string{"n": "5"}, "n", 1, 1, 10); n != 5 || err != nil {
		t.Fatal(n, err)
	}
	if _, err := argInt(map[string]string{"n": "11"}, "n", 1, 1, 10); !errors.Is(err, ErrUnknown) {
		t.Fatal("over the range accepted")
	}
}
